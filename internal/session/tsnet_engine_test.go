package session

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/logging"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"tailscale.com/envknob"
)

func tsnetCfg(t *testing.T) provider.ResolvedSessionConfig {
	return provider.ResolvedSessionConfig{
		NetworkID:    domain.NewNetworkID(),
		Provider:     domain.ProviderHeadscale,
		ControlURL:   "https://hs.example.com",
		NodeHostname: "h",
		StateDir:     t.TempDir(),
	}
}

func TestTsnetEngineRefusesUnpreparedEnv(t *testing.T) {
	t.Setenv("TS_NO_LOGS_NO_SUPPORT", "")
	if _, err := newTsnetEngine(tsnetCfg(t), "", slog.Default()); !errors.Is(err, ErrUnpreparedEnv) {
		t.Fatalf("got %v want ErrUnpreparedEnv", err)
	}
	if err := PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	if !envknob.NoLogsNoSupport() {
		t.Fatal("log upload not disabled after PrepareProcessEnv")
	}
	if _, err := newTsnetEngine(tsnetCfg(t), "", slog.Default()); err != nil {
		t.Fatal(err)
	}
}

func TestAmbientAuthKeysNeverReachTsnet(t *testing.T) {
	const a, b = "FLAVOR_TEST_SECRET_A", "FLAVOR_TEST_SECRET_B"
	t.Setenv("TS_AUTHKEY", a)
	t.Setenv("TS_AUTH_KEY", b)
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelDebug)

	if _, err := newTsnetEngine(tsnetCfg(t), "", log); !errors.Is(err, ErrUnpreparedEnv) {
		t.Fatalf("ambient key accepted: %v", err)
	}
	if err := PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"TS_AUTHKEY", "TS_AUTH_KEY"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Fatalf("%s still set", k)
		}
	}
	e, err := newTsnetEngine(tsnetCfg(t), "", log)
	if err != nil {
		t.Fatal(err)
	}
	if e.(*tsnetEngine).srv.AuthKey != "" {
		t.Fatal("tsnet server received an auth key")
	}
	for _, secret := range []string{a, b} {
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("ambient secret %s leaked into logs", secret)
		}
	}
}

func TestTsnetLogAdapterDropsAuthURL(t *testing.T) {
	if err := PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	const token = "FLAVOR_CANARY_TOKEN_5e7d"
	canary := "https://login.example.com/a/" + token + "?token=" + token
	var buf bytes.Buffer
	e, err := newTsnetEngine(tsnetCfg(t), "", logging.New(&buf, slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	srv := e.(*tsnetEngine).srv
	if srv.UserLogf == nil || srv.Logf == nil {
		t.Fatal("tsnet loggers must be set explicitly")
	}
	srv.UserLogf("To start this tsnet server, restart with TS_AUTHKEY set, or go to: %s", canary)
	srv.UserLogf("tsnet running state path %s", canary)
	srv.Logf("Received auth URL: %.20v...", canary)
	srv.Logf("%s", canary)
	out := buf.String()
	if strings.Contains(out, token) || strings.Contains(out, "login.example.com") {
		t.Fatalf("auth url leaked: %s", out)
	}
	if !strings.Contains(out, "network_id=") {
		t.Fatalf("expected adapter to log with session attrs: %s", out)
	}
}

func TestTsnetLogAdapterFormatsAndRedacts(t *testing.T) {
	var buf bytes.Buffer
	logf := tsnetUserLogf(logging.New(&buf, slog.LevelDebug))
	secrets := []string{
		"https://hs.example.com/register/FLAVOR_CANARY_REG_1a2b",
		"tskey-auth-kFLAVORCANARY-0123456789abcdef",
		"hskey-auth-FLAVOR_CANARY_hs_3c4d",
		"0123456789abcdef0123456789abcdef0123456789abcdef",
		"privkey:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"nlpriv:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"FLAVORCANARYBASE64TOKENaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for _, s := range secrets {
		logf("To start this tsnet server, restart with TS_AUTHKEY set, or go to: %s", s)
		logf("value %v here", s)
	}
	logf("tsnet starting with hostname %q, varRoot %q", "mac-test", "/tmp/flavor/networks/01M4ENPT42GAJC1W1ATBD995YP")
	logf("AuthLoop: state is %v; done", "Running")
	out := buf.String()
	for _, s := range secrets {
		if strings.Contains(out, s) || strings.Contains(out, "CANARY") {
			t.Fatalf("secret %q leaked: %s", s, out)
		}
	}
	for _, want := range []string{`\"mac-test\"`, "01M4ENPT42GAJC1W1ATBD995YP", "state is Running; done", logging.Redacted} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

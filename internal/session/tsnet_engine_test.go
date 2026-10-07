package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"tailscale.com/envknob"
)

func TestTsnetEngineRefusesUnpreparedEnv(t *testing.T) {
	t.Setenv("TS_NO_LOGS_NO_SUPPORT", "")
	if _, err := newTsnetEngine(testCfg(t), "", slog.Default()); !errors.Is(err, ErrUnpreparedEnv) {
		t.Fatalf("got %v want ErrUnpreparedEnv", err)
	}
	if err := PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	if !envknob.NoLogsNoSupport() {
		t.Fatal("log upload not disabled after PrepareProcessEnv")
	}
	if _, err := newTsnetEngine(testCfg(t), "", slog.Default()); err != nil {
		t.Fatal(err)
	}
}

func TestAmbientAuthKeysNeverReachTsnet(t *testing.T) {
	const a, b = "LATTICE_TEST_SECRET_A", "LATTICE_TEST_SECRET_B"
	t.Setenv("TS_AUTHKEY", a)
	t.Setenv("TS_AUTH_KEY", b)
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelDebug)

	if _, err := newTsnetEngine(testCfg(t), "", log); !errors.Is(err, ErrUnpreparedEnv) {
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
	e, err := newTsnetEngine(testCfg(t), "", log)
	if err != nil {
		t.Fatal(err)
	}
	if e.(*tsnetEngine).srv.AuthKey != "" {
		t.Fatal("tsnet server received an auth key")
	}

	bus := events.NewBus(256, 64)
	defer bus.Close()
	s, err := newSession(testCfg(t), bus, log, newFakeFactory(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	_ = s.Stop(context.Background())
	var dump strings.Builder
	dump.WriteString(buf.String())
	for _, p := range history(t, bus) {
		fmt.Fprintf(&dump, "%+v", p)
	}
	for _, secret := range []string{a, b} {
		if strings.Contains(dump.String(), secret) {
			t.Fatalf("ambient secret %s leaked", secret)
		}
	}
}

func TestTsnetLogAdapterDropsAuthURL(t *testing.T) {
	if err := PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	const token = "LATTICE_CANARY_TOKEN_5e7d"
	canary := "https://login.example.com/a/" + token + "?token=" + token
	var buf bytes.Buffer
	e, err := newTsnetEngine(testCfg(t), "", logging.New(&buf, slog.LevelDebug))
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

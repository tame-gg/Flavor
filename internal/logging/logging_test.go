package logging_test

import (
	"log/slog"
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

func TestSanitizeURL(t *testing.T) {
	got := logging.SanitizeURL("https://user:pass@example.com/login?token=abc#foo")
	if got != "https://example.com/login" {
		t.Fatalf("got %q", got)
	}
	if logging.SanitizeURL("not a url") != logging.Redacted {
		t.Fatal("expected redacted for garbage")
	}
}

func TestLoggerDoesNotEmitCanarySecret(t *testing.T) {
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	var b strings.Builder
	logger := logging.New(&b, slog.LevelDebug)
	logger.Error("enrollment failed",
		"network_id", "01TEST",
		"credential", secret.New(canary),
		"auth_url", logging.SanitizeURL("https://login.example/auth?token="+canary),
	)
	out := b.String()
	if strings.Contains(out, canary) {
		t.Fatalf("canary leaked: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected redaction markers: %s", out)
	}
	if !strings.Contains(out, "https://login.example/auth") {
		t.Fatalf("safe url host/path should remain: %s", out)
	}
}

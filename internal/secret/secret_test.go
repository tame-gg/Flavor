package secret_test

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

func TestSecretRedactsFormatting(t *testing.T) {
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	s := secret.New(canary)
	if s.String() != "[REDACTED]" {
		t.Fatalf("String=%q", s.String())
	}
	if s.GoString() != "[REDACTED]" {
		t.Fatalf("GoString=%q", s.GoString())
	}
	verbV := fmt.Sprintf("%v", s)
	if verbV != "[REDACTED]" {
		t.Fatalf("verb v formatting leaked: %s", verbV)
	}
	verbHashV := fmt.Sprintf("%#v", s)
	if verbHashV != "[REDACTED]" {
		t.Fatalf("verb hash-v formatting leaked: %s", verbHashV)
	}
	if s.Reveal() != canary {
		t.Fatal("Reveal must return plaintext")
	}
}

func TestSecretSlogDoesNotLeak(t *testing.T) {
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	var b strings.Builder
	logger := slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Info("probe", "credential", secret.New(canary))
	out := b.String()
	if strings.Contains(out, canary) {
		t.Fatalf("canary leaked into slog: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected redacted marker: %s", out)
	}
}

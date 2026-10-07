package domain_test

import (
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

func TestNetworkLabel(t *testing.T) {
	for in, want := range map[string]string{
		"LunarLabs":            "lunarlabs",
		"Home Lab":             "home-lab",
		"  Customer A / Prod ": "customer-a-prod",
		"Ünïcode Café":         "n-code-caf",
		"!!!":                  "",
	} {
		if got := domain.NetworkLabel(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
	if got := domain.NetworkLabel(strings.Repeat("a", 80)); len(got) != 63 {
		t.Fatalf("label not bounded: %d", len(got))
	}
}

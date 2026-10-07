package domain_test

import (
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

func TestNewNetworkIDUniqueAndParseable(t *testing.T) {
	a := domain.NewNetworkID()
	b := domain.NewNetworkID()
	if a == b {
		t.Fatal("expected unique NetworkIDs")
	}
	parsed, err := domain.ParseNetworkID(string(a))
	if err != nil {
		t.Fatal(err)
	}
	if parsed != a {
		t.Fatalf("got %q want %q", parsed, a)
	}
}

func TestParseNetworkIDRejectsTraversal(t *testing.T) {
	cases := []string{
		"",
		".",
		"..",
		"../escape",
		"foo/bar",
		"foo\\bar",
		"has space",
		"has\x00null",
		strings.Repeat("a", 65),
	}
	for _, c := range cases {
		if _, err := domain.ParseNetworkID(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestParseNodeIDRejectsEmpty(t *testing.T) {
	if _, err := domain.ParseNodeID(""); err == nil {
		t.Fatal("expected error")
	}
	id, err := domain.ParseNodeID("n123")
	if err != nil || id != "n123" {
		t.Fatalf("got %q %v", id, err)
	}
}

func TestControlPlaneIDTailscaleVsHeadscale(t *testing.T) {
	ts := domain.ControlPlaneIDFor(domain.ProviderTailscale, "")
	hsA := domain.ControlPlaneIDFor(domain.ProviderHeadscale, "https://headscale.example.com")
	hsB := domain.ControlPlaneIDFor(domain.ProviderHeadscale, "https://other.example.com")
	if ts == "" || hsA == "" {
		t.Fatal("empty control plane id")
	}
	if ts == hsA || hsA == hsB {
		t.Fatal("control plane ids must distinguish planes")
	}
}

package domain_test

import (
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

func TestParseProvider(t *testing.T) {
	p, err := domain.ParseProvider("tailscale")
	if err != nil || p != domain.ProviderTailscale {
		t.Fatalf("got %v %v", p, err)
	}
	p, err = domain.ParseProvider("headscale")
	if err != nil || p != domain.ProviderHeadscale {
		t.Fatalf("got %v %v", p, err)
	}
	if _, err := domain.ParseProvider("wireguard"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateNetworkInvariants(t *testing.T) {
	now := time.Now().UTC()
	n := domain.Network{
		ID:           domain.NewNetworkID(),
		DisplayName:  "LunarLabs",
		Provider:     domain.ProviderHeadscale,
		ControlURL:   "https://headscale.example.com",
		AutoConnect:  true,
		NodeHostname: "luna-desktop",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	n.ControlURL = ""
	if err := n.Validate(); err == nil {
		t.Fatal("headscale requires control url")
	}
	n.Provider = domain.ProviderTailscale
	n.ControlURL = "https://should-be-empty.example"
	if err := n.Validate(); err == nil {
		t.Fatal("tailscale requires empty control url")
	}
	n.ControlURL = ""
	n.NodeHostname = ""
	if err := n.Validate(); err == nil {
		t.Fatal("node hostname required")
	}
}

func TestDisplayNameAndHostnameIndependent(t *testing.T) {
	now := time.Now().UTC()
	n := domain.Network{
		ID:           domain.NewNetworkID(),
		DisplayName:  "LunarLabs Production",
		Provider:     domain.ProviderTailscale,
		ControlURL:   "",
		NodeHostname: "luna-desktop",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	if n.DisplayName == n.NodeHostname {
		t.Fatal("display name and hostname should remain distinct values in this fixture")
	}
	n.DisplayName = "Work"
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	if n.NodeHostname != "luna-desktop" {
		t.Fatal("rename must not alter node hostname in model")
	}
}

func TestDisplayNamesMayCollide(t *testing.T) {
	now := time.Now().UTC()
	a := domain.Network{
		ID: domain.NewNetworkID(), DisplayName: "Home", Provider: domain.ProviderTailscale,
		NodeHostname: "host-a", CreatedAt: now, UpdatedAt: now,
	}
	b := domain.Network{
		ID: domain.NewNetworkID(), DisplayName: "Home", Provider: domain.ProviderHeadscale,
		ControlURL: "https://hs.example", NodeHostname: "host-b", CreatedAt: now, UpdatedAt: now,
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.DisplayName != b.DisplayName {
		t.Fatal("fixture expects colliding display names")
	}
	if a.ID == b.ID {
		t.Fatal("ids must differ")
	}
}

func TestNormalizeControlURL(t *testing.T) {
	got, err := domain.NormalizeControlURL(domain.ProviderHeadscale, "https://headscale.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://headscale.example.com" {
		t.Fatalf("got %q", got)
	}
	got, err = domain.NormalizeControlURL(domain.ProviderTailscale, "https://ignored.example")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("tailscale control url must be empty, got %q", got)
	}
	if _, err := domain.NormalizeControlURL(domain.ProviderHeadscale, "ftp://bad"); err == nil {
		t.Fatal("expected scheme error")
	}
	if _, err := domain.NormalizeControlURL(domain.ProviderHeadscale, ""); err == nil {
		t.Fatal("expected empty url error")
	}
}

func TestConnectionStateParse(t *testing.T) {
	s, err := domain.ParseConnectionState("connected")
	if err != nil || s != domain.StateConnected {
		t.Fatalf("got %v %v", s, err)
	}
	if _, err := domain.ParseConnectionState("running"); err == nil {
		t.Fatal("running is not a lattice connection state")
	}
}

package provider_test

import (
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

func TestResolveTailscale(t *testing.T) {
	n := domain.Network{
		ID: domain.NewNetworkID(), DisplayName: "Personal", Provider: domain.ProviderTailscale,
		ControlURL: "", NodeHostname: "luna-desktop",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	cfg, err := provider.Resolve(n, "/tmp/state")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlURL != "" {
		t.Fatalf("control=%q", cfg.ControlURL)
	}
	if cfg.NodeHostname != "luna-desktop" || cfg.StateDir != "/tmp/state" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.ControlPlane != domain.ControlPlaneIDFor(domain.ProviderTailscale, "") {
		t.Fatal("control plane")
	}
}

func TestResolveHeadscale(t *testing.T) {
	n := domain.Network{
		ID: domain.NewNetworkID(), DisplayName: "Lab", Provider: domain.ProviderHeadscale,
		ControlURL: "https://hs.example.com/", NodeHostname: "lab-host",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	cfg, err := provider.Resolve(n, "/data/tsnet")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlURL != "https://hs.example.com" {
		t.Fatalf("control=%q", cfg.ControlURL)
	}
	if cfg.Provider != domain.ProviderHeadscale {
		t.Fatal("provider")
	}
}

package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/events"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
)

func TestIntegrationHeadscale(t *testing.T) {
	if os.Getenv("FLAVOR_INTEGRATION") != "1" {
		t.Skip("set FLAVOR_INTEGRATION=1 to run")
	}
	url := os.Getenv("FLAVOR_HEADSCALE_URL")
	key := os.Getenv("FLAVOR_HEADSCALE_AUTHKEY")
	if url == "" || key == "" {
		t.Skip("FLAVOR_HEADSCALE_URL and FLAVOR_HEADSCALE_AUTHKEY required")
	}
	if err := session.PrepareProcessEnv(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id := domain.NewNetworkID()
	stateDir := filepath.Join(dir, "tsnet")
	n := domain.Network{
		ID: id, DisplayName: "Integration", Provider: domain.ProviderHeadscale,
		ControlURL: url, NodeHostname: "flavor-itest",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	cfg, err := provider.Resolve(n, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(64, 8)
	defer bus.Close()
	s, err := session.NewSession(cfg, bus, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.Start(ctx, &session.EnrollmentInput{
		Method: session.EnrollmentAuthKey, Credential: secret.New(key),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		st := s.State()
		if st == domain.StateConnected || st == domain.StateDegraded {
			break
		}
		if st == domain.StateError {
			t.Fatalf("error state: %+v", s.Diagnostics(ctx))
		}
		time.Sleep(500 * time.Millisecond)
	}
	if s.State() != domain.StateConnected && s.State() != domain.StateDegraded {
		t.Fatalf("state=%s diag=%+v", s.State(), s.Diagnostics(ctx))
	}
	ln := s.LocalNode()
	if ln.NodeID == "" || len(ln.Addresses) == 0 {
		t.Fatalf("local node incomplete: %+v", ln)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	s2, err := session.NewSession(cfg, bus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if s2.State() == domain.StateConnected || s2.State() == domain.StateDegraded {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if s2.State() != domain.StateConnected && s2.State() != domain.StateDegraded {
		t.Fatalf("restart without auth key failed: state=%s", s2.State())
	}
	_ = s2.Stop(ctx)
}

package session_test

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

func cfgFor(id domain.NetworkID, name string) provider.ResolvedSessionConfig {
	return provider.ResolvedSessionConfig{
		NetworkID:    id,
		Provider:     domain.ProviderHeadscale,
		ControlURL:   "https://hs.example.com",
		NodeHostname: name,
		StateDir:     "/tmp/lattice-test/" + string(id) + "/tsnet",
		ControlPlane: domain.ControlPlaneIDFor(domain.ProviderHeadscale, "https://hs.example.com"),
	}
}

func TestSessionBoundToNetworkID(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(32, 8)
	defer bus.Close()
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, (&sessiontest.Sequence{}).Factory)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID() != id {
		t.Fatal(s.ID())
	}
	if s.Provider() != domain.ProviderHeadscale {
		t.Fatal(s.Provider())
	}
}

func TestStartStopLifecycle(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(64, 8)
	defer bus.Close()
	seq := &sessiontest.Sequence{}
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, seq.Factory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, nil); !errors.Is(err, session.ErrAlreadyActive) {
		t.Fatalf("got %v", err)
	}
	if n := seq.Count(); n != 1 {
		t.Fatalf("engines=%d", n)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.State() == domain.StateConnected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.State() != domain.StateConnected {
		t.Fatalf("state=%s", s.State())
	}
	ln := s.LocalNode()
	if ln.NodeID == "" || len(ln.Addresses) == 0 {
		t.Fatalf("%+v", ln)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if s.State() != domain.StateDisconnected {
		t.Fatal(s.State())
	}
	if len(s.Devices()) != 0 {
		t.Fatal("devices not cleared")
	}
}

func TestPartialStartCleanup(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(32, 8)
	defer bus.Close()
	eng := sessiontest.NewEngine()
	eng.SetStartErr(errors.New("boom"))
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
	if s.State() != domain.StateError {
		t.Fatal(s.State())
	}
	eng.SetStartErr(nil)
	eng.SetStatus(sessiontest.StatusSelf("node-self", "100.64.0.1"))
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestAuthPromptAndMalformedURL(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(64, 8)
	defer bus.Close()
	sub, err := bus.Subscribe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusNeedsLogin(""))
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eng.PushBrowse("javascript:alert(1)")
	time.Sleep(50 * time.Millisecond)
	if s.AuthPrompt() != nil {
		t.Fatal("malformed accepted")
	}
	eng.PushBrowse("https://login.example.com/a/xyz?token=secret")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.AuthPrompt() != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	p := s.AuthPrompt()
	if p == nil || p.FlowID == "" || p.URL == "" {
		t.Fatalf("%+v", p)
	}
	d := s.Diagnostics(context.Background())
	if d.HasAuthPrompt != true {
		t.Fatal(d)
	}
	found := false
	for {
		select {
		case ev := <-sub.Events:
			if ar, ok := ev.Payload.(events.AuthenticationRequired); ok {
				found = true
				if ar.AuthURL == "" || ar.FlowID == "" {
					t.Fatalf("%+v", ar)
				}
			}
		default:
			goto done
		}
	}
done:
	if !found {
		t.Fatal("missing AuthenticationRequired")
	}
}

func TestEnrollmentClearedAndNotInEvents(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(64, 8)
	defer bus.Close()
	sub, err := bus.Subscribe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	eng := sessiontest.NewEngine()
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	cred := secret.New("tskey-auth-SUPERSECRET")
	if err := s.Start(context.Background(), &session.EnrollmentInput{
		Method: session.EnrollmentAuthKey, Credential: cred,
	}); err != nil {
		t.Fatal(err)
	}
	if !eng.AuthKeyCleared() {
		t.Fatal("auth key retained")
	}
	time.Sleep(50 * time.Millisecond)
	for {
		select {
		case ev := <-sub.Events:
			raw := ev.Payload
			if s, ok := any(raw).(interface{ String() string }); ok {
				if containsSecret(s.String(), "SUPERSECRET") {
					t.Fatal("secret in event string")
				}
			}
		default:
			return
		}
	}
}

func containsSecret(s, needle string) bool {
	return len(needle) > 0 && (len(s) >= len(needle)) && (stringIndex(s, needle) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestDeviceDiffAndSnapshotIsolation(t *testing.T) {
	id := domain.NewNetworkID()
	bus := events.NewBus(64, 8)
	defer bus.Close()
	sub, err := bus.Subscribe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusWithPeer("peer-a", "100.64.0.2"))
	s, err := session.NewSessionWithFactory(cfgFor(id, "host-a"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, domain.StateConnected)
	devs := s.Devices()
	if len(devs) < 2 {
		t.Fatalf("%d", len(devs))
	}
	devs[0].Hostname = "mutated"
	devs[0].Addresses[0] = netip.MustParseAddr("100.64.0.9")
	again := s.Devices()
	for _, d := range again {
		if d.Hostname == "mutated" {
			t.Fatal("internal mutated")
		}
	}
	eng.SetStatus(sessiontest.StatusWithPeer("peer-a", "100.64.0.2"))
	eng.PushNetMap()
	time.Sleep(50 * time.Millisecond)
	updated := 0
	for {
		select {
		case ev := <-sub.Events:
			if _, ok := ev.Payload.(events.PeerUpdated); ok {
				updated++
			}
		default:
			goto done
		}
	}
done:
	if updated != 0 {
		t.Fatalf("unexpected updates=%d", updated)
	}
	eng.SetStatus(sessiontest.StatusWithPeer("peer-a", "100.64.0.3"))
	eng.PushNetMap()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, d := range s.Devices() {
			if d.ID.NodeID == "peer-a" && len(d.Addresses) == 1 && d.Addresses[0].String() == "100.64.0.3" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("peer not updated")
}

func TestDuplicateIPDistinctIdentity(t *testing.T) {
	bus := events.NewBus(64, 8)
	defer bus.Close()
	idA := domain.NewNetworkID()
	idB := domain.NewNetworkID()
	engA := sessiontest.NewEngine()
	engB := sessiontest.NewEngine()
	engA.SetStatus(sessiontest.StatusSelf("alpha", "100.64.0.1"))
	engB.SetStatus(sessiontest.StatusSelf("beta", "100.64.0.1"))
	a, err := session.NewSessionWithFactory(cfgFor(idA, "a"), bus, nil, sessiontest.Shared(engA))
	if err != nil {
		t.Fatal(err)
	}
	b, err := session.NewSessionWithFactory(cfgFor(idB, "b"), bus, nil, sessiontest.Shared(engB))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	waitState(t, a, domain.StateConnected)
	waitState(t, b, domain.StateConnected)
	da, db := a.Devices(), b.Devices()
	if len(da) == 0 || len(db) == 0 {
		t.Fatal("missing devices")
	}
	var ia, ib domain.DeviceIdentity
	for _, d := range da {
		if d.ID.NodeID == "alpha" {
			ia = d.ID
		}
	}
	for _, d := range db {
		if d.ID.NodeID == "beta" {
			ib = d.ID
		}
	}
	if ia.Key() == "" || ib.Key() == "" || ia.Key() == ib.Key() {
		t.Fatalf("%s vs %s", ia.Key(), ib.Key())
	}
}

func TestManagerOnePerNetwork(t *testing.T) {
	bus := events.NewBus(32, 8)
	defer bus.Close()
	m := session.NewManagerWithFactory(bus, nil, (&sessiontest.Sequence{}).Factory)
	id := domain.NewNetworkID()
	c := cfgFor(id, "host")
	s1, err := m.GetOrCreate(c)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := m.GetOrCreate(c)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 {
		t.Fatal("duplicate session objects")
	}
	id2 := domain.NewNetworkID()
	s3, err := m.GetOrCreate(cfgFor(id2, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if s3 == s1 {
		t.Fatal("shared across networks")
	}
}

func TestManagerLockDoesNotSerializeWork(t *testing.T) {
	bus := events.NewBus(32, 8)
	defer bus.Close()
	idA, idB := domain.NewNetworkID(), domain.NewNetworkID()
	seq := &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		if cfg.NetworkID == idA {
			e.SetSlowStart(200 * time.Millisecond)
		}
	}}
	m := session.NewManagerWithFactory(bus, nil, seq.Factory)
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = m.Start(context.Background(), cfgFor(idA, "a"), nil)
	}()
	go func() {
		defer wg.Done()
		_, _ = m.Start(context.Background(), cfgFor(idB, "b"), nil)
	}()
	wg.Wait()
	if time.Since(start) > 350*time.Millisecond {
		t.Fatalf("serialized: %s", time.Since(start))
	}
}

func TestConcurrentStart(t *testing.T) {
	bus := events.NewBus(32, 8)
	defer bus.Close()
	eng := sessiontest.NewEngine()
	eng.SetSlowStart(100 * time.Millisecond)
	s, err := session.NewSessionWithFactory(cfgFor(domain.NewNetworkID(), "h"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Start(context.Background(), nil)
		}()
	}
	wg.Wait()
	close(errs)
	var ok, busy int
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, session.ErrBusy), errors.Is(err, session.ErrAlreadyActive):
			busy++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || busy != 1 {
		t.Fatalf("ok=%d busy=%d", ok, busy)
	}
}

func TestApprovalMapping(t *testing.T) {
	bus := events.NewBus(64, 8)
	defer bus.Close()
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusNeedsMachineAuth())
	s, err := session.NewSessionWithFactory(cfgFor(domain.NewNetworkID(), "h"), bus, nil, sessiontest.Shared(eng))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, domain.StateAwaitingApproval)
}

func TestStateDirFromConfigNotCaller(t *testing.T) {
	id := domain.NewNetworkID()
	cfg := cfgFor(id, "h")
	cfg.StateDir = "/trusted/path/tsnet"
	bus := events.NewBus(8, 4)
	defer bus.Close()
	s, err := session.NewSessionWithFactory(cfg, bus, nil, (&sessiontest.Sequence{}).Factory)
	if err != nil {
		t.Fatal(err)
	}
	d := s.Diagnostics(context.Background())
	if d.NetworkID != id {
		t.Fatal(d.NetworkID)
	}
}

func waitState(t *testing.T, s *session.Session, want domain.NetworkConnectionState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.State() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state=%s want=%s", s.State(), want)
}

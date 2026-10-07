package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

func testCfg(t *testing.T) provider.ResolvedSessionConfig {
	t.Helper()
	return provider.ResolvedSessionConfig{
		NetworkID:    domain.NewNetworkID(),
		Provider:     domain.ProviderHeadscale,
		ControlURL:   "https://hs.example.com",
		NodeHostname: "h",
		StateDir:     t.TempDir(),
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func history(t *testing.T, bus *events.Bus) []events.Payload {
	t.Helper()
	sub, err := bus.Subscribe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	var out []events.Payload
	for {
		select {
		case ev := <-sub.Events:
			out = append(out, ev.Payload)
		default:
			return out
		}
	}
}

func count[T events.Payload](ps []events.Payload) int {
	n := 0
	for _, p := range ps {
		if _, ok := p.(T); ok {
			n++
		}
	}
	return n
}

type sequenceFactory struct {
	mu      sync.Mutex
	tracker liveTracker
	made    []*fakeEngine
	prepare func(i int, f *fakeEngine)
}

func (sf *sequenceFactory) factory(cfg provider.ResolvedSessionConfig, authKey string, _ *slog.Logger) (engine, error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	f := NewFakeEngine()
	f.authKey = authKey
	f.tracker = &sf.tracker
	if sf.prepare != nil {
		sf.prepare(len(sf.made), f)
	}
	sf.made = append(sf.made, f)
	sf.tracker.add(1)
	return f, nil
}

func (sf *sequenceFactory) engine(i int) *fakeEngine {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	if i >= len(sf.made) {
		return nil
	}
	return sf.made[i]
}

func (sf *sequenceFactory) count() int {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	return len(sf.made)
}

func (s *Session) activeEngine() engine {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.gen == nil || s.gen.phase != backendStarted {
		return nil
	}
	return s.gen.eng
}

func TestStartStopStartCannotResurrectStaleGeneration(t *testing.T) {
	for i := 0; i < 50; i++ {
		gateA := make(chan struct{})
		enteredA := make(chan struct{})
		sf := &sequenceFactory{prepare: func(i int, f *fakeEngine) {
			f.status = StatusSelf("node-"+string(rune('a'+i)), "100.64.0.1")
			if i == 0 {
				f.startGate = gateA
				f.startEntered = enteredA
			}
		}}
		s, err := newSession(testCfg(t), nil, nil, sf.factory)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()

		errA := make(chan error, 1)
		go func() { errA <- s.Start(ctx, nil) }()
		<-enteredA

		errStop := make(chan error, 1)
		go func() { errStop <- s.Stop(ctx) }()
		eventually(t, "stop to mark generation stopping", func() bool { return s.backendLifecycle() == backendStopping })

		errB := make(chan error, 1)
		go func() { errB <- s.Start(ctx, nil) }()
		time.Sleep(5 * time.Millisecond)
		if n := sf.count(); n != 1 {
			t.Fatalf("start B created an engine before A finished cleanup: engines=%d", n)
		}

		close(gateA)
		if err := <-errA; !errors.Is(err, ErrStopped) {
			t.Fatalf("start A: got %v want ErrStopped", err)
		}
		if err := <-errStop; err != nil {
			t.Fatalf("stop: %v", err)
		}
		if err := <-errB; err != nil {
			t.Fatalf("start B: %v", err)
		}

		engA, engB := sf.engine(0), sf.engine(1)
		if !engA.isClosed() {
			t.Fatal("engine A not closed")
		}
		if engB == nil || s.activeEngine() != engine(engB) {
			t.Fatal("live generation is not B")
		}
		if peak := sf.tracker.peak(); peak != 1 {
			t.Fatalf("engines alive at once on one state dir: %d", peak)
		}
		eventually(t, "B connected", func() bool { return s.State() == domain.StateConnected })
		if got := s.LocalNode().NodeID; got != "node-b" {
			t.Fatalf("local node from stale generation: %s", got)
		}
		if err := s.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWatcherErrorAlwaysReachesError(t *testing.T) {
	for i := 0; i < 50; i++ {
		bus := events.NewBus(256, 64)
		eng := NewFakeEngine()
		s, err := newSession(testCfg(t), bus, nil, newFakeFactory(eng, nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
		eng.watchErr <- errors.New("unexpected EOF")
		eventually(t, "cleanup after watcher failure", func() bool { return s.backendLifecycle() == backendStopped })
		if s.State() != domain.StateError {
			t.Fatalf("iteration %d: state=%s want error", i, s.State())
		}
		if !eng.isClosed() {
			t.Fatal("engine not closed after watcher failure")
		}
		if n := eng.watchers(); n != 0 {
			t.Fatalf("watcher goroutines still running: %d", n)
		}
		if n := count[events.NetworkStateChanged](filterState(history(t, bus), domain.StateError)); n != 1 {
			t.Fatalf("error transitions=%d want 1", n)
		}
		bus.Close()
	}
}

func filterState(ps []events.Payload, st domain.NetworkConnectionState) []events.Payload {
	var out []events.Payload
	for _, p := range ps {
		if c, ok := p.(events.NetworkStateChanged); ok && c.State == st {
			out = append(out, p)
		}
	}
	return out
}

func TestSessionRestartsAfterWatcherFailure(t *testing.T) {
	sf := &sequenceFactory{}
	s, err := newSession(testCfg(t), nil, nil, sf.factory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	sf.engine(0).watchErr <- errors.New("boom")
	eventually(t, "stopped", func() bool { return s.backendLifecycle() == backendStopped })
	if err := s.Start(ctx, nil); err != nil {
		t.Fatalf("restart after watcher failure: %v", err)
	}
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < sf.count(); i++ {
		if n := sf.engine(i).watchers(); n != 0 {
			t.Fatalf("engine %d leaked watcher", i)
		}
	}
	if sf.tracker.peak() != 1 {
		t.Fatal("dual engines")
	}
}

func TestStopCancellationIsNotWatcherFailure(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	s, err := newSession(testCfg(t), bus, nil, newFakeFactory(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.State() != domain.StateDisconnected {
		t.Fatalf("state=%s", s.State())
	}
	if n := len(filterState(history(t, bus), domain.StateError)); n != 0 {
		t.Fatalf("cancellation produced %d error transitions", n)
	}
}

func TestStaleGenerationWatcherFailureIgnored(t *testing.T) {
	sf := &sequenceFactory{}
	s, err := newSession(testCfg(t), nil, nil, sf.factory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	s.lifeMu.Lock()
	stale := s.gen
	s.lifeMu.Unlock()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	s.watcherFailed(stale, errors.New("late failure"))
	if s.State() != domain.StateConnected || s.backendLifecycle() != backendStarted {
		t.Fatalf("stale watcher affected live generation: state=%s backend=%s", s.State(), s.backendLifecycle())
	}
	if sf.engine(1).isClosed() {
		t.Fatal("live engine closed by stale failure")
	}
	_ = s.Stop(ctx)
}

func TestStopTimeoutDoesNotWedge(t *testing.T) {
	gate := make(chan struct{})
	sf := &sequenceFactory{prepare: func(i int, f *fakeEngine) {
		if i == 0 {
			f.closeGate = gate
		}
	}}
	s, err := newSession(testCfg(t), nil, nil, sf.factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Stop(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop: got %v want deadline", err)
	}
	if s.backendLifecycle() != backendStopping {
		t.Fatalf("backend=%s", s.backendLifecycle())
	}

	short2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := s.Start(short2, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("start during cleanup: got %v want deadline", err)
	}

	secondStop := make(chan error, 1)
	go func() { secondStop <- s.Stop(context.Background()) }()

	close(gate)
	if err := <-secondStop; err != nil {
		t.Fatalf("second stop should join in-flight cleanup: %v", err)
	}
	if s.backendLifecycle() != backendStopped || s.State() != domain.StateDisconnected {
		t.Fatalf("backend=%s state=%s", s.backendLifecycle(), s.State())
	}
	if !sf.engine(0).isClosed() || sf.engine(0).watchers() != 0 {
		t.Fatal("engine A not fully cleaned up")
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatalf("start after recovered cleanup: %v", err)
	}
	if sf.tracker.peak() != 1 {
		t.Fatal("dual engines")
	}
	_ = s.Stop(context.Background())
}

func TestStaleStatusCannotOverwriteNewerGeneration(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	statusGate := make(chan struct{})
	sf := &sequenceFactory{prepare: func(i int, f *fakeEngine) {
		if i == 0 {
			f.status = StatusWithPeer("stale-peer", "100.64.0.9")
			f.statusGate = statusGate
			return
		}
		f.status = StatusWithPeer("fresh-peer", "100.64.0.2")
	}}
	s, err := newSession(testCfg(t), bus, nil, sf.factory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- s.Stop(ctx) }()
	eventually(t, "stopping", func() bool { return s.backendLifecycle() == backendStopping })
	startB := make(chan error, 1)
	go func() { startB <- s.Start(ctx, nil) }()

	close(statusGate)
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	if err := <-startB; err != nil {
		t.Fatal(err)
	}
	eventually(t, "fresh peer", func() bool {
		for _, d := range s.Devices() {
			if d.ID.NodeID == "fresh-peer" {
				return true
			}
		}
		return false
	})
	for _, d := range s.Devices() {
		if d.ID.NodeID == "stale-peer" {
			t.Fatal("stale generation status applied")
		}
	}
	for _, p := range history(t, bus) {
		if a, ok := p.(events.PeerAdded); ok && a.Device.ID.NodeID == "stale-peer" {
			t.Fatal("stale generation emitted PeerAdded")
		}
	}
	_ = s.Stop(ctx)
}

func TestApprovalRequiredEmittedOncePerTransition(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	eng := NewFakeEngine()
	eng.status = StatusNeedsMachineAuth()
	s, err := newSession(testCfg(t), bus, nil, newFakeFactory(eng, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "awaiting approval", func() bool { return s.State() == domain.StateAwaitingApproval })
	for i := 0; i < 5; i++ {
		eng.push(notifySnap{NetMapChanged: true})
	}
	time.Sleep(50 * time.Millisecond)
	ps := history(t, bus)
	if n := count[events.ApprovalRequired](ps); n != 1 {
		t.Fatalf("ApprovalRequired=%d want 1", n)
	}
	if n := len(filterState(ps, domain.StateAwaitingApproval)); n != 1 {
		t.Fatalf("AwaitingApproval transitions=%d want 1", n)
	}
	_ = s.Stop(context.Background())
}

func TestInteractiveLoginSingleFlow(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	const url = "https://login.example.com/a/flow1"
	eng := NewFakeEngine()
	eng.status = statusSnap{BackendState: "NeedsLogin", AuthURL: url}
	s, err := newSession(testCfg(t), bus, nil, newFakeFactory(eng, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "authenticating", func() bool { return s.State() == domain.StateAuthenticating })
	first := s.AuthPrompt()
	state := "NeedsLogin"
	u := url
	for i := 0; i < 3; i++ {
		eng.push(notifySnap{BackendState: &state})
		eng.push(notifySnap{BrowseToURL: &u})
		eng.push(notifySnap{NetMapChanged: true})
	}
	time.Sleep(50 * time.Millisecond)
	if n := count[events.AuthenticationRequired](history(t, bus)); n != 1 {
		t.Fatalf("AuthenticationRequired=%d want 1", n)
	}
	if got := s.AuthPrompt(); got == nil || got.FlowID != first.FlowID {
		t.Fatal("auth flow restarted for identical URL")
	}
	_ = s.Stop(context.Background())
}

func TestSessionLogsOnlyAuthHost(t *testing.T) {
	const token = "LATTICE_CANARY_TOKEN_91c2"
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelDebug)
	eng := NewFakeEngine()
	eng.status = statusSnap{BackendState: "NeedsLogin"}
	s, err := newSession(testCfg(t), nil, log, newFakeFactory(eng, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	u := "https://login.example.com/a/" + token + "?token=" + token
	eng.push(notifySnap{BrowseToURL: &u})
	eventually(t, "prompt", func() bool { return s.AuthPrompt() != nil })
	_ = s.Stop(context.Background())
	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("auth token leaked into logs: %s", out)
	}
	if !strings.Contains(out, "auth_host=login.example.com") {
		t.Fatalf("expected auth host in logs: %s", out)
	}
}

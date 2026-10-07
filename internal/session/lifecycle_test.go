package session_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
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

func filterState(ps []events.Payload, st domain.NetworkConnectionState) []events.Payload {
	var out []events.Payload
	for _, p := range ps {
		if c, ok := p.(events.NetworkStateChanged); ok && c.State == st {
			out = append(out, p)
		}
	}
	return out
}

func newSession(t *testing.T, bus *events.Bus, log *slog.Logger, f session.EngineFactory) *session.Session {
	t.Helper()
	s, err := session.NewSessionWithFactory(testCfg(t), bus, log, f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStartStopStartCannotResurrectStaleGeneration(t *testing.T) {
	for i := 0; i < 50; i++ {
		var enteredA <-chan struct{}
		var releaseA func()
		seq := &sessiontest.Sequence{Prepare: func(i int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
			e.SetStatus(sessiontest.StatusSelf("node-"+string(rune('a'+i)), "100.64.0.1"))
			if i == 0 {
				enteredA, releaseA = e.GateStart()
			}
		}}
		s := newSession(t, nil, nil, seq.Factory)
		ctx := context.Background()

		errA := make(chan error, 1)
		go func() { errA <- s.Start(ctx, nil) }()
		eventually(t, "engine A created", func() bool { return seq.Count() == 1 })
		<-enteredA

		errStop := make(chan error, 1)
		go func() { errStop <- s.Stop(ctx) }()
		eventually(t, "stop to mark generation stopping", func() bool { return s.Lifecycle() == "stopping" })

		errB := make(chan error, 1)
		go func() { errB <- s.Start(ctx, nil) }()
		time.Sleep(5 * time.Millisecond)
		if n := seq.Count(); n != 1 {
			t.Fatalf("start B created an engine before A finished cleanup: engines=%d", n)
		}

		releaseA()
		if err := <-errA; !errors.Is(err, session.ErrStopped) {
			t.Fatalf("start A: got %v want ErrStopped", err)
		}
		if err := <-errStop; err != nil {
			t.Fatalf("stop: %v", err)
		}
		if err := <-errB; err != nil {
			t.Fatalf("start B: %v", err)
		}

		engA, engB := seq.Engine(0), seq.Engine(1)
		if !engA.Closed() {
			t.Fatal("engine A not closed")
		}
		if engB == nil || s.ActiveEngine() != session.Engine(engB) {
			t.Fatal("live generation is not B")
		}
		if peak := seq.Peak(); peak != 1 {
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
		eng := sessiontest.NewEngine()
		s := newSession(t, bus, nil, sessiontest.Shared(eng))
		if err := s.Start(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
		eng.FailWatch(errors.New("unexpected EOF"))
		eventually(t, "cleanup after watcher failure", func() bool { return s.Lifecycle() == "stopped" })
		if s.State() != domain.StateError {
			t.Fatalf("iteration %d: state=%s want error", i, s.State())
		}
		if !eng.Closed() {
			t.Fatal("engine not closed after watcher failure")
		}
		if n := eng.Watchers(); n != 0 {
			t.Fatalf("watcher goroutines still running: %d", n)
		}
		if n := len(filterState(history(t, bus), domain.StateError)); n != 1 {
			t.Fatalf("error transitions=%d want 1", n)
		}
		bus.Close()
	}
}

func TestSessionRestartsAfterWatcherFailure(t *testing.T) {
	seq := &sessiontest.Sequence{}
	s := newSession(t, nil, nil, seq.Factory)
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	seq.Engine(0).FailWatch(errors.New("boom"))
	eventually(t, "stopped", func() bool { return s.Lifecycle() == "stopped" })
	if err := s.Start(ctx, nil); err != nil {
		t.Fatalf("restart after watcher failure: %v", err)
	}
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < seq.Count(); i++ {
		if n := seq.Engine(i).Watchers(); n != 0 {
			t.Fatalf("engine %d leaked watcher", i)
		}
	}
	if seq.Peak() != 1 {
		t.Fatal("dual engines")
	}
}

func TestStopCancellationIsNotWatcherFailure(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	s := newSession(t, bus, nil, (&sessiontest.Sequence{}).Factory)
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
	seq := &sessiontest.Sequence{}
	s := newSession(t, nil, nil, seq.Factory)
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	stale := s.Generation()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	s.FailWatcher(stale, errors.New("late failure"))
	if s.State() != domain.StateConnected || s.Lifecycle() != "started" {
		t.Fatalf("stale watcher affected live generation: state=%s backend=%s", s.State(), s.Lifecycle())
	}
	if seq.Engine(1).Closed() {
		t.Fatal("live engine closed by stale failure")
	}
	_ = s.Stop(ctx)
}

func TestStopTimeoutDoesNotWedge(t *testing.T) {
	var release func()
	seq := &sessiontest.Sequence{Prepare: func(i int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		if i == 0 {
			release = e.GateClose()
		}
	}}
	s := newSession(t, nil, nil, seq.Factory)
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Stop(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop: got %v want deadline", err)
	}
	if s.Lifecycle() != "stopping" {
		t.Fatalf("backend=%s", s.Lifecycle())
	}

	short2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := s.Start(short2, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("start during cleanup: got %v want deadline", err)
	}

	secondStop := make(chan error, 1)
	go func() { secondStop <- s.Stop(context.Background()) }()

	release()
	if err := <-secondStop; err != nil {
		t.Fatalf("second stop should join in-flight cleanup: %v", err)
	}
	if s.Lifecycle() != "stopped" || s.State() != domain.StateDisconnected {
		t.Fatalf("backend=%s state=%s", s.Lifecycle(), s.State())
	}
	if !seq.Engine(0).Closed() || seq.Engine(0).Watchers() != 0 {
		t.Fatal("engine A not fully cleaned up")
	}
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatalf("start after recovered cleanup: %v", err)
	}
	if seq.Peak() != 1 {
		t.Fatal("dual engines")
	}
	_ = s.Stop(context.Background())
}

func TestStaleStatusCannotOverwriteNewerGeneration(t *testing.T) {
	bus := events.NewBus(256, 64)
	defer bus.Close()
	var release func()
	seq := &sessiontest.Sequence{Prepare: func(i int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		if i == 0 {
			e.SetStatus(sessiontest.StatusWithPeer("stale-peer", "100.64.0.9"))
			release = e.GateStatus()
			return
		}
		e.SetStatus(sessiontest.StatusWithPeer("fresh-peer", "100.64.0.2"))
	}}
	s := newSession(t, bus, nil, seq.Factory)
	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- s.Stop(ctx) }()
	eventually(t, "stopping", func() bool { return s.Lifecycle() == "stopping" })
	startB := make(chan error, 1)
	go func() { startB <- s.Start(ctx, nil) }()

	release()
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
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusNeedsMachineAuth())
	s := newSession(t, bus, nil, sessiontest.Shared(eng))
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "awaiting approval", func() bool { return s.State() == domain.StateAwaitingApproval })
	for i := 0; i < 5; i++ {
		eng.PushNetMap()
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
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusNeedsLogin(url))
	s := newSession(t, bus, nil, sessiontest.Shared(eng))
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "authenticating", func() bool { return s.State() == domain.StateAuthenticating })
	first := s.AuthPrompt()
	for i := 0; i < 3; i++ {
		eng.PushState("NeedsLogin")
		eng.PushBrowse(url)
		eng.PushNetMap()
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
	eng := sessiontest.NewEngine()
	eng.SetStatus(sessiontest.StatusNeedsLogin(""))
	s := newSession(t, nil, log, sessiontest.Shared(eng))
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	eng.PushBrowse("https://login.example.com/a/" + token + "?token=" + token)
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

func TestBeginAcceptsThenStreams(t *testing.T) {
	eng := sessiontest.NewEngine()
	entered, release := eng.GateStart()
	s := newSession(t, nil, nil, sessiontest.Shared(eng))
	if err := s.Begin(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := s.Begin(context.Background(), nil); !errors.Is(err, session.ErrBusy) {
		t.Fatalf("second begin while starting: %v", err)
	}
	release()
	eventually(t, "connected", func() bool { return s.State() == domain.StateConnected })
	if err := s.Begin(context.Background(), nil); !errors.Is(err, session.ErrAlreadyActive) {
		t.Fatalf("begin while active: %v", err)
	}
	_ = s.Stop(context.Background())
}

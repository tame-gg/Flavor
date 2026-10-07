package session

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

type fakeEngine struct {
	mu             sync.Mutex
	started        bool
	closed         bool
	authKey        string
	cleared        bool
	status         statusSnap
	notifies       chan notifySnap
	watchErr       chan error
	startErr       error
	slowStart      time.Duration
	startEntered   chan struct{}
	startGate      chan struct{}
	closeGate      chan struct{}
	statusGate     chan struct{}
	activeWatchers int
	tracker        *liveTracker
}

type liveTracker struct {
	mu   sync.Mutex
	live int
	max  int
}

func (l *liveTracker) add(d int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.live += d
	if l.live > l.max {
		l.max = l.live
	}
}

func (l *liveTracker) peak() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.max
}

func newFakeFactory(shared *fakeEngine, counter *int) engineFactory {
	return func(cfg provider.ResolvedSessionConfig, authKey string, _ *slog.Logger) (engine, error) {
		if shared != nil {
			shared.mu.Lock()
			shared.authKey = authKey
			shared.closed = false
			shared.mu.Unlock()
			return shared, nil
		}
		f := NewFakeEngine()
		f.authKey = authKey
		f.status.Self.Hostname = cfg.NodeHostname
		f.status.Self.DNSName = cfg.NodeHostname + ".tailnet.ts.net"
		if counter != nil {
			*counter++
		}
		return f, nil
	}
}

func (f *fakeEngine) Start() error {
	f.mu.Lock()
	slow, entered, gate := f.slowStart, f.startEntered, f.startGate
	f.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if gate != nil {
		<-gate
	}
	if slow > 0 {
		time.Sleep(slow)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.started = true
	return nil
}

func (f *fakeEngine) Close() error {
	f.mu.Lock()
	gate := f.closeGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed && f.tracker != nil {
		f.tracker.add(-1)
	}
	f.closed = true
	f.started = false
	return nil
}

func (f *fakeEngine) ClearAuthKey() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authKey = ""
	f.cleared = true
}

func (f *fakeEngine) Status(ctx context.Context) (statusSnap, error) {
	f.mu.Lock()
	gate := f.statusGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeEngine) Watch(ctx context.Context, emit func(notifySnap)) error {
	f.mu.Lock()
	f.activeWatchers++
	notifies, errs := f.notifies, f.watchErr
	initial := f.status.BackendState
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.activeWatchers--
		f.mu.Unlock()
	}()
	emit(notifySnap{BackendState: &initial})
	for {
		select {
		case <-ctx.Done():
			return errors.New("ipn bus stream closed")
		case err := <-errs:
			return err
		case n := <-notifies:
			emit(n)
		}
	}
}

func (f *fakeEngine) push(n notifySnap) {
	f.notifies <- n
}

func (f *fakeEngine) setStatus(st statusSnap) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = st
}

func (f *fakeEngine) watchers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeWatchers
}

func (f *fakeEngine) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func selfSnap(nodeID string, ip netip.Addr) *peerSnap {
	return &peerSnap{
		NodeID:    domain.NodeID(nodeID),
		Hostname:  nodeID,
		DNSName:   nodeID + ".ts.net",
		Addresses: []netip.Addr{ip},
		Online:    true,
	}
}

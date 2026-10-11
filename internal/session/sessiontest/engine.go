package sessiontest

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
)

type Engine struct {
	mu             sync.Mutex
	closed         bool
	authKey        string
	cleared        bool
	status         session.EngineStatus
	notifies       chan session.EngineNotify
	watchErr       chan error
	startErr       error
	slowStart      time.Duration
	startEntered   chan struct{}
	startGate      chan struct{}
	closeGate      chan struct{}
	statusGate     chan struct{}
	activeWatchers int
	tracker        *tracker
	dial           func(ctx context.Context, network, address string) (net.Conn, error)
	exitNode       domain.NodeID
	exitNodeCalls  []domain.NodeID
	advertised     []netip.Prefix
	approved       []netip.Prefix
	routeCalls     [][]netip.Prefix
}

func NewEngine() *Engine {
	return &Engine{
		notifies: make(chan session.EngineNotify, 16),
		watchErr: make(chan error, 1),
		status:   StatusSelf("node-self", "100.64.0.1"),
	}
}

func (e *Engine) Start() error {
	e.mu.Lock()
	slow, entered, gate := e.slowStart, e.startEntered, e.startGate
	e.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if gate != nil {
		<-gate
	}
	if slow > 0 {
		time.Sleep(slow)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startErr
}

func (e *Engine) Close() error {
	e.mu.Lock()
	gate := e.closeGate
	e.mu.Unlock()
	if gate != nil {
		<-gate
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed && e.tracker != nil {
		e.tracker.add(-1)
	}
	e.closed = true
	return nil
}

func (e *Engine) ClearAuthKey() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.authKey = ""
	e.cleared = true
}

func (e *Engine) Status(ctx context.Context) (session.EngineStatus, error) {
	e.mu.Lock()
	gate := e.statusGate
	e.mu.Unlock()
	if gate != nil {
		<-gate
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.status
	st.ExitNode = e.exitNode
	st.AdvertisedRoutes = slices.Clone(e.advertised)
	st.ApprovedRoutes = slices.Clone(e.approved)
	return st, nil
}

func (e *Engine) SetAdvertisedRoutes(_ context.Context, routes []netip.Prefix) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advertised = slices.Clone(routes)
	e.routeCalls = append(e.routeCalls, slices.Clone(routes))
	return nil
}

func (e *Engine) AdvertisedRoutes() []netip.Prefix {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.advertised)
}

func (e *Engine) AdvertisedRoutesCalls() [][]netip.Prefix {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.routeCalls)
}

func (e *Engine) ApproveRoutes(routes ...netip.Prefix) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.approved = slices.Clone(routes)
}

func (e *Engine) ApproveExitNode(approved bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status.Self != nil {
		self := *e.status.Self
		self.ExitNodeOption = approved
		e.status.Self = &self
	}
}

func (e *Engine) SetExitNode(_ context.Context, id domain.NodeID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exitNode = id
	e.exitNodeCalls = append(e.exitNodeCalls, id)
	return nil
}

func (e *Engine) ExitNode() domain.NodeID {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exitNode
}

func (e *Engine) ExitNodeCalls() []domain.NodeID {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.exitNodeCalls)
}

func (e *Engine) Watch(ctx context.Context, emit func(session.EngineNotify)) error {
	e.mu.Lock()
	e.activeWatchers++
	notifies, errs := e.notifies, e.watchErr
	initial := e.status.BackendState
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.activeWatchers--
		e.mu.Unlock()
	}()
	emit(session.EngineNotify{BackendState: &initial})
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

var ErrNoDialer = errors.New("sessiontest: no dialer configured")

func (e *Engine) SetDial(fn func(ctx context.Context, network, address string) (net.Conn, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dial = fn
}

func (e *Engine) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	e.mu.Lock()
	fn, closed := e.dial, e.closed
	e.mu.Unlock()
	if closed {
		return nil, net.ErrClosed
	}
	if fn == nil {
		return nil, ErrNoDialer
	}
	return fn(ctx, network, address)
}

func (e *Engine) SetStatus(st session.EngineStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = st
}

func (e *Engine) SetStartErr(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startErr = err
}

func (e *Engine) SetSlowStart(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.slowStart = d
}

func (e *Engine) GateStart() (entered <-chan struct{}, release func()) {
	in, gate := make(chan struct{}), make(chan struct{})
	e.mu.Lock()
	e.startEntered, e.startGate = in, gate
	e.mu.Unlock()
	return in, func() { close(gate) }
}

func (e *Engine) GateClose() (release func()) {
	gate := make(chan struct{})
	e.mu.Lock()
	e.closeGate = gate
	e.mu.Unlock()
	return func() { close(gate) }
}

func (e *Engine) GateStatus() (release func()) {
	gate := make(chan struct{})
	e.mu.Lock()
	e.statusGate = gate
	e.mu.Unlock()
	return func() { close(gate) }
}

func (e *Engine) Push(n session.EngineNotify) { e.notifies <- n }

func (e *Engine) PushNetMap() { e.Push(session.EngineNotify{NetMapChanged: true}) }

func (e *Engine) PushState(state string) { e.Push(session.EngineNotify{BackendState: &state}) }

func (e *Engine) PushBrowse(url string) { e.Push(session.EngineNotify{BrowseToURL: &url}) }

func (e *Engine) FailWatch(err error) { e.watchErr <- err }

func (e *Engine) Closed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}

func (e *Engine) Watchers() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.activeWatchers
}

func (e *Engine) AuthKey() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authKey
}

func (e *Engine) AuthKeyCleared() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cleared
}

func Shared(e *Engine) session.EngineFactory {
	return func(_ provider.ResolvedSessionConfig, authKey string, _ *slog.Logger) (session.Engine, error) {
		e.mu.Lock()
		e.authKey = authKey
		e.closed = false
		e.mu.Unlock()
		return e, nil
	}
}

type tracker struct {
	mu   sync.Mutex
	live int
	max  int
}

func (t *tracker) add(d int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live += d
	if t.live > t.max {
		t.max = t.live
	}
}

type Sequence struct {
	Prepare func(i int, cfg provider.ResolvedSessionConfig, e *Engine)

	mu      sync.Mutex
	tracker tracker
	made    []*Engine
}

func (s *Sequence) Factory(cfg provider.ResolvedSessionConfig, authKey string, _ *slog.Logger) (session.Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := NewEngine()
	e.authKey = authKey
	e.tracker = &s.tracker
	e.status.Self.Hostname = cfg.NodeHostname
	if s.Prepare != nil {
		s.Prepare(len(s.made), cfg, e)
	}
	s.made = append(s.made, e)
	s.tracker.add(1)
	return e, nil
}

func (s *Sequence) Engine(i int) *Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.made) {
		return nil
	}
	return s.made[i]
}

func (s *Sequence) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.made)
}

func (s *Sequence) Peak() int {
	s.tracker.mu.Lock()
	defer s.tracker.mu.Unlock()
	return s.tracker.max
}

func StatusSelf(nodeID, ip string) session.EngineStatus {
	addr := netip.MustParseAddr(ip)
	return session.EngineStatus{
		BackendState: "Running",
		TailscaleIPs: []netip.Addr{addr},
		Self: &session.EnginePeer{
			NodeID:    domain.NodeID(nodeID),
			Hostname:  nodeID,
			DNSName:   nodeID + ".ts.net",
			Addresses: []netip.Addr{addr},
			Online:    true,
		},
	}
}

func StatusWithPeer(nodeID, ip string) session.EngineStatus {
	st := StatusSelf("node-self", "100.64.0.1")
	addr := netip.MustParseAddr(ip)
	st.Peers = []session.EnginePeer{{
		NodeID:    domain.NodeID(nodeID),
		Hostname:  nodeID,
		DNSName:   nodeID + ".ts.net",
		Addresses: []netip.Addr{addr},
		Online:    true,
	}}
	return st
}

func StatusNeedsLogin(authURL string) session.EngineStatus {
	return session.EngineStatus{BackendState: "NeedsLogin", AuthURL: authURL}
}

func StatusNeedsMachineAuth() session.EngineStatus {
	return session.EngineStatus{BackendState: "NeedsMachineAuth"}
}

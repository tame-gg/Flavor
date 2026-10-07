package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

type NetworkSession interface {
	ID() domain.NetworkID
	Provider() domain.ProviderType
	Start(ctx context.Context, enrollment *EnrollmentInput) error
	Begin(ctx context.Context, enrollment *EnrollmentInput) error
	Stop(ctx context.Context) error
	State() domain.NetworkConnectionState
	LocalNode() domain.LocalNode
	Devices() []domain.Device
	AuthPrompt() *domain.AuthPrompt
	Diagnostics(ctx context.Context) domain.SessionDiagnostics
}

type Session struct {
	cfg    provider.ResolvedSessionConfig
	bus    *events.Bus
	log    *slog.Logger
	newEng EngineFactory

	lifeMu sync.Mutex
	gen    *generation

	stateMu sync.RWMutex
	state   domain.NetworkConnectionState
	local   domain.LocalNode
	devices map[domain.NodeID]domain.Device
	prompt  *domain.AuthPrompt
	lastErr string
}

type generation struct {
	phase     backendLifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	eng       Engine
	watchDone chan struct{}
	final     domain.NetworkConnectionState
	finalMsg  string
}

func NewSession(cfg provider.ResolvedSessionConfig, bus *events.Bus, log *slog.Logger) (*Session, error) {
	return newSession(cfg, bus, log, newTsnetEngine)
}

func newSession(cfg provider.ResolvedSessionConfig, bus *events.Bus, log *slog.Logger, factory EngineFactory) (*Session, error) {
	if cfg.NetworkID == "" || cfg.StateDir == "" || cfg.NodeHostname == "" {
		return nil, ErrInvalidConfig
	}
	if log == nil {
		log = slog.Default()
	}
	return &Session{
		cfg:     cfg,
		bus:     bus,
		log:     log,
		newEng:  factory,
		state:   domain.StateDisconnected,
		devices: make(map[domain.NodeID]domain.Device),
		local:   domain.LocalNode{NetworkID: cfg.NetworkID},
	}, nil
}

func (s *Session) ID() domain.NetworkID          { return s.cfg.NetworkID }
func (s *Session) Provider() domain.ProviderType { return s.cfg.Provider }

func (s *Session) Start(ctx context.Context, enrollment *EnrollmentInput) error {
	if err := enrollment.validate(); err != nil {
		return err
	}
	g, err := s.reserve(ctx)
	if err != nil {
		return err
	}
	return s.run(g, enrollment)
}

func (s *Session) Begin(ctx context.Context, enrollment *EnrollmentInput) error {
	if err := enrollment.validate(); err != nil {
		return err
	}
	g, err := s.reserve(ctx)
	if err != nil {
		return err
	}
	go func() {
		if err := s.run(g, enrollment); err != nil {
			s.log.Info("session start failed", "network_id", s.cfg.NetworkID, "err", err.Error())
		}
	}()
	return nil
}

func (s *Session) run(g *generation, enrollment *EnrollmentInput) error {
	authKey := ""
	if enrollment != nil && enrollment.Method == EnrollmentAuthKey {
		authKey = enrollment.Credential.Reveal()
	}

	s.transition(domain.StateConnecting, "")

	eng, err := s.newEng(s.cfg, authKey, s.log)
	if err != nil {
		return s.abortStart(g, err)
	}
	s.lifeMu.Lock()
	g.eng = eng
	s.lifeMu.Unlock()
	if err := eng.Start(); err != nil {
		return s.abortStart(g, err)
	}
	eng.ClearAuthKey()

	s.lifeMu.Lock()
	if g.phase != backendStarting {
		s.lifeMu.Unlock()
		s.retire(g)
		return ErrStopped
	}
	g.phase = backendStarted
	g.watchDone = make(chan struct{})
	s.lifeMu.Unlock()

	go s.watch(g)
	return nil
}

func (s *Session) reserve(ctx context.Context) (*generation, error) {
	for {
		s.lifeMu.Lock()
		prev := s.gen
		if prev == nil {
			runCtx, cancel := context.WithCancel(context.Background())
			g := &generation{
				phase:  backendStarting,
				ctx:    runCtx,
				cancel: cancel,
				done:   make(chan struct{}),
				final:  domain.StateDisconnected,
			}
			s.gen = g
			s.lifeMu.Unlock()
			return g, nil
		}
		phase := prev.phase
		s.lifeMu.Unlock()
		switch phase {
		case backendStarted:
			return nil, ErrAlreadyActive
		case backendStarting:
			return nil, ErrBusy
		}
		select {
		case <-prev.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *Session) abortStart(g *generation, err error) error {
	s.lifeMu.Lock()
	if g.phase == backendStarting {
		g.phase = backendStopping
		g.final, g.finalMsg = domain.StateError, err.Error()
	}
	s.lifeMu.Unlock()
	s.retire(g)
	return err
}

func (s *Session) Stop(ctx context.Context) error {
	s.lifeMu.Lock()
	g := s.gen
	if g == nil {
		s.lifeMu.Unlock()
		return nil
	}
	prev := g.phase
	g.phase = backendStopping
	g.final, g.finalMsg = domain.StateDisconnected, ""
	s.lifeMu.Unlock()

	g.cancel()
	if prev == backendStarted {
		go s.retire(g)
	}
	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) retire(g *generation) {
	g.cancel()
	s.lifeMu.Lock()
	eng, watchDone := g.eng, g.watchDone
	s.lifeMu.Unlock()
	if eng != nil {
		_ = eng.Close()
	}
	if watchDone != nil {
		<-watchDone
	}

	s.lifeMu.Lock()
	final, msg := g.final, g.finalMsg
	s.lifeMu.Unlock()

	s.stateMu.Lock()
	s.devices = make(map[domain.NodeID]domain.Device)
	s.local = domain.LocalNode{NetworkID: s.cfg.NetworkID}
	s.prompt = nil
	s.lastErr = msg
	s.stateMu.Unlock()
	s.transition(final, msg)

	s.lifeMu.Lock()
	s.gen = nil
	s.lifeMu.Unlock()
	close(g.done)
}

func (s *Session) watch(g *generation) {
	err := g.eng.Watch(g.ctx, func(n EngineNotify) { s.handleNotify(g, n) })
	close(g.watchDone)
	if g.ctx.Err() != nil {
		return
	}
	if err == nil {
		err = errWatcherEnded
	}
	s.watcherFailed(g, err)
}

func (s *Session) watcherFailed(g *generation, err error) {
	const safeMsg = "ipn watcher failed"
	s.lifeMu.Lock()
	if s.gen != g || g.phase != backendStarted {
		s.lifeMu.Unlock()
		return
	}
	g.phase = backendStopping
	g.final, g.finalMsg = domain.StateError, safeMsg
	s.lifeMu.Unlock()

	s.log.Info("session watcher failed", "network_id", s.cfg.NetworkID, "err", err.Error())
	s.transition(domain.StateError, safeMsg)
	s.retire(g)
}

func (s *Session) ifCurrent(g *generation, fn func()) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.gen == g && g.phase == backendStarted {
		fn()
	}
}

func (s *Session) handleNotify(g *generation, n EngineNotify) {
	if n.BrowseToURL != nil || n.LoginFinished {
		s.ifCurrent(g, func() {
			if n.BrowseToURL != nil {
				s.setAuthPrompt(*n.BrowseToURL)
			}
			if n.LoginFinished {
				s.clearAuthPrompt(true)
			}
		})
	}
	if n.BackendState == nil && !n.NetMapChanged && n.BrowseToURL == nil && !n.LoginFinished {
		return
	}
	snap, err := g.eng.Status(g.ctx)
	if err != nil {
		if g.ctx.Err() == nil {
			s.log.Info("session status refresh failed", "network_id", s.cfg.NetworkID, "err", err.Error())
		}
		return
	}
	s.ifCurrent(g, func() { s.applyStatus(snap) })
}

func (s *Session) applyStatus(snap EngineStatus) {
	if snap.BackendState == "NeedsLogin" && snap.AuthURL != "" {
		s.setAuthPrompt(snap.AuthURL)
	}

	s.stateMu.RLock()
	hadPrompt := s.prompt != nil
	s.stateMu.RUnlock()

	var local domain.LocalNode
	local.NetworkID = s.cfg.NetworkID
	if snap.Self != nil {
		local.NodeID = snap.Self.NodeID
		local.Hostname = snap.Self.Hostname
		local.DNSName = snap.Self.DNSName
		local.Addresses = append([]netip.Addr(nil), snap.Self.Addresses...)
	} else if len(snap.TailscaleIPs) > 0 {
		local.Addresses = append([]netip.Addr(nil), snap.TailscaleIPs...)
	}

	newDevices := make(map[domain.NodeID]domain.Device, len(snap.Peers)+1)
	for _, p := range snap.Peers {
		if p.NodeID == "" {
			continue
		}
		newDevices[p.NodeID] = domain.Device{
			ID:        domain.DeviceIdentity{NetworkID: s.cfg.NetworkID, NodeID: p.NodeID},
			Hostname:  p.Hostname,
			DNSName:   p.DNSName,
			Addresses: append([]netip.Addr(nil), p.Addresses...),
			Online:    p.Online,
			LastSeen:  p.LastSeen,
		}
	}
	if snap.Self != nil && snap.Self.NodeID != "" {
		newDevices[snap.Self.NodeID] = domain.Device{
			ID:        domain.DeviceIdentity{NetworkID: s.cfg.NetworkID, NodeID: snap.Self.NodeID},
			Hostname:  snap.Self.Hostname,
			DNSName:   snap.Self.DNSName,
			Addresses: append([]netip.Addr(nil), snap.Self.Addresses...),
			Online:    snap.Self.Online,
			LastSeen:  snap.Self.LastSeen,
		}
	}

	s.stateMu.Lock()
	old := s.devices
	s.devices = newDevices
	s.local = local
	s.stateMu.Unlock()

	next := mapConnectionState(snap, hadPrompt)
	if next == domain.StateConnected || next == domain.StateDegraded {
		s.clearAuthPrompt(true)
	}
	if s.transition(next, "") && next == domain.StateAwaitingApproval {
		s.publish(events.ApprovalRequired{
			NetworkID:   s.cfg.NetworkID,
			Provider:    s.cfg.Provider,
			SafeMessage: "machine authorization required",
		})
	}
	s.diffDevices(old, newDevices)
}

func (s *Session) diffDevices(old, next map[domain.NodeID]domain.Device) {
	for id, d := range next {
		prev, ok := old[id]
		if !ok {
			s.publish(events.PeerAdded{Device: cloneDevice(d)})
			continue
		}
		if !deviceEqual(prev, d) {
			s.publish(events.PeerUpdated{Device: cloneDevice(d)})
		}
	}
	for id, d := range old {
		if _, ok := next[id]; !ok {
			s.publish(events.PeerRemoved{ID: d.ID})
		}
	}
}

func (s *Session) setAuthPrompt(raw string) {
	safe, err := parseAuthURL(raw)
	if err != nil {
		s.log.Info("authentication url rejected",
			"network_id", s.cfg.NetworkID,
			"provider", s.cfg.Provider,
		)
		return
	}
	flow := newFlowID()
	s.stateMu.Lock()
	if s.prompt != nil && s.prompt.URL == safe {
		s.stateMu.Unlock()
		return
	}
	s.prompt = &domain.AuthPrompt{
		FlowID:    flow,
		URL:       safe,
		Provider:  s.cfg.Provider,
		CreatedAt: nowUTC(),
	}
	s.stateMu.Unlock()
	s.transition(domain.StateAuthenticating, "")
	s.log.Info("authentication required",
		"network_id", s.cfg.NetworkID,
		"provider", s.cfg.Provider,
		"auth_host", authHost(safe),
		"flow_id", flow,
	)
	s.publish(events.AuthenticationRequired{
		NetworkID: s.cfg.NetworkID,
		Provider:  s.cfg.Provider,
		AuthURL:   safe,
		FlowID:    flow,
	})
}

func (s *Session) clearAuthPrompt(completed bool) {
	s.stateMu.Lock()
	had := s.prompt != nil
	s.prompt = nil
	s.stateMu.Unlock()
	if had && completed {
		s.publish(events.AuthenticationCompleted{NetworkID: s.cfg.NetworkID})
	}
}

func (s *Session) transition(next domain.NetworkConnectionState, safeMsg string) bool {
	s.stateMu.Lock()
	prev := s.state
	if prev == next {
		if safeMsg != "" {
			s.lastErr = safeMsg
		}
		s.stateMu.Unlock()
		return false
	}
	s.state = next
	if safeMsg != "" {
		s.lastErr = safeMsg
	}
	s.stateMu.Unlock()
	s.publish(events.NetworkStateChanged{
		NetworkID:   s.cfg.NetworkID,
		State:       next,
		SafeMessage: safeMsg,
	})
	return true
}

func (s *Session) publish(p events.Payload) {
	if s.bus == nil {
		return
	}
	_, _ = s.bus.Publish(p)
}

func (s *Session) State() domain.NetworkConnectionState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

func (s *Session) LocalNode() domain.LocalNode {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneLocalNode(s.local)
}

func (s *Session) Devices() []domain.Device {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	out := make([]domain.Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, cloneDevice(d))
	}
	return out
}

func (s *Session) AuthPrompt() *domain.AuthPrompt {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneAuthPrompt(s.prompt)
}

func (s *Session) Diagnostics(ctx context.Context) domain.SessionDiagnostics {
	backend := string(s.backendLifecycle())
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	host := ""
	if s.cfg.ControlURL != "" {
		host = logging.SanitizeURL(s.cfg.ControlURL)
	} else {
		host = "tailscale:default"
	}
	return domain.SessionDiagnostics{
		NetworkID:        s.cfg.NetworkID,
		Provider:         s.cfg.Provider,
		ConnectionState:  s.state,
		BackendLifecycle: backend,
		ControlHost:      host,
		LocalAddrCount:   len(s.local.Addresses),
		DeviceCount:      len(s.devices),
		HasAuthPrompt:    s.prompt != nil,
	}
}

func (s *Session) backendLifecycle() backendLifecycle {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.gen == nil {
		return backendStopped
	}
	return s.gen.phase
}

func newFlowID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Session) String() string {
	return fmt.Sprintf("session(%s)", s.cfg.NetworkID)
}

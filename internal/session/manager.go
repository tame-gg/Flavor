package session

import (
	"context"
	"log/slog"
	"sync"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/events"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
)

type Manager struct {
	mu       sync.Mutex
	sessions map[domain.NetworkID]*Session
	bus      *events.Bus
	log      *slog.Logger
	newEng   EngineFactory
}

func NewManager(bus *events.Bus, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		sessions: make(map[domain.NetworkID]*Session),
		bus:      bus,
		log:      log,
		newEng:   newTsnetEngine,
	}
}

func NewManagerWithFactory(bus *events.Bus, log *slog.Logger, f EngineFactory) *Manager {
	m := NewManager(bus, log)
	m.newEng = f
	return m
}

func (m *Manager) GetOrCreate(cfg provider.ResolvedSessionConfig) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[cfg.NetworkID]; ok {
		return s, nil
	}
	s, err := newSession(cfg, m.bus, m.log, m.newEng)
	if err != nil {
		return nil, err
	}
	m.sessions[cfg.NetworkID] = s
	return s, nil
}

func (m *Manager) Get(id domain.NetworkID) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) Start(ctx context.Context, cfg provider.ResolvedSessionConfig, enrollment *EnrollmentInput) (*Session, error) {
	s, err := m.GetOrCreate(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.Start(ctx, enrollment); err != nil {
		return s, err
	}
	return s, nil
}

func (m *Manager) Stop(ctx context.Context, id domain.NetworkID) error {
	s, ok := m.Get(id)
	if !ok {
		return nil
	}
	return s.Stop(ctx)
}

func (m *Manager) Remove(id domain.NetworkID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil
	}
	if s.backendLifecycle() != backendStopped {
		return ErrBusy
	}
	delete(m.sessions, id)
	return nil
}

func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	errs := make(chan error, len(all))
	for _, s := range all {
		go func() { errs <- s.Stop(ctx) }()
	}
	var first error
	for range all {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

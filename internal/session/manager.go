package session

import (
	"context"
	"log/slog"
	"sync"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

type Manager struct {
	mu       sync.Mutex
	sessions map[domain.NetworkID]*Session
	bus      *events.Bus
	log      *slog.Logger
	newEng   engineFactory
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

func (m *Manager) withEngineFactory(f engineFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newEng = f
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

func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

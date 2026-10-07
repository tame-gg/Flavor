package session

import (
	"log/slog"

	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

func NewSessionWithFactory(cfg provider.ResolvedSessionConfig, bus *events.Bus, log *slog.Logger, f EngineFactory) (*Session, error) {
	return newSession(cfg, bus, log, f)
}

func (s *Session) Lifecycle() string { return string(s.backendLifecycle()) }

func (s *Session) Generation() any {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	return s.gen
}

func (s *Session) FailWatcher(g any, err error) { s.watcherFailed(g.(*generation), err) }

func (s *Session) ActiveEngine() Engine {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.gen == nil || s.gen.phase != backendStarted {
		return nil
	}
	return s.gen.eng
}

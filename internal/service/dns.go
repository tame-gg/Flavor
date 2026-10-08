package service

import (
	"fmt"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/events"
)

const ambiguityWarningInterval = time.Minute

func (s *Service) Warn(code, message string) {
	s.publish(events.DaemonWarning{Code: code, SafeMessage: message})
}

func (s *Service) WarnAmbiguousName(name string, candidates, networks int) {
	s.mu.Lock()
	if s.warned == nil {
		s.warned = make(map[string]time.Time)
	}
	last, seen := s.warned[name]
	now := time.Now()
	if seen && now.Sub(last) < ambiguityWarningInterval {
		s.mu.Unlock()
		return
	}
	s.warned[name] = now
	s.mu.Unlock()
	s.Warn("dns_ambiguous", fmt.Sprintf("DNS lookup for %s was refused: it matches %d candidates on %d networks. Use a Lattice name or set a preference.", name, candidates, networks))
}

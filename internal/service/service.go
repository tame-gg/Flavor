package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/config"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
	"git.lunarlabs.dev/lattice/lattice/internal/version"
)

type Code string

const (
	CodeNetworkNotFound   Code = "NETWORK_NOT_FOUND"
	CodeInvalidControlURL Code = "INVALID_CONTROL_URL"
	CodeAlreadyConnected  Code = "NETWORK_ALREADY_CONNECTED"
	CodeBusy              Code = "NETWORK_BUSY"
	CodeStateDirectory    Code = "STATE_DIRECTORY_ERROR"
	CodeShuttingDown      Code = "DAEMON_SHUTTING_DOWN"
	CodeInvalidArgument   Code = "INVALID_ARGUMENT"
	CodeInternal          Code = "INTERNAL"
)

type Error struct {
	Code        Code
	SafeMessage string
	Retryable   bool
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.SafeMessage }

func fail(code Code, msg string, retryable bool) error {
	return &Error{Code: code, SafeMessage: msg, Retryable: retryable}
}

type Config struct {
	Store        *store.DB
	Paths        config.Paths
	Sessions     *session.Manager
	Bus          *events.Bus
	Secrets      secret.Store
	Log          *slog.Logger
	InstanceID   string
	NodeHostname string
	StopTimeout  time.Duration
}

type Service struct {
	cfg Config

	mu           sync.Mutex
	ops          map[domain.NetworkID]string
	shuttingDown bool
}

func New(cfg Config) *Service {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.StopTimeout <= 0 {
		cfg.StopTimeout = 30 * time.Second
	}
	if cfg.NodeHostname == "" {
		cfg.NodeHostname = DefaultNodeHostname()
	}
	return &Service{cfg: cfg, ops: make(map[domain.NetworkID]string)}
}

type DaemonInfo struct {
	version.BuildInfo
	InstanceID string
}

func (s *Service) Info() DaemonInfo {
	return DaemonInfo{BuildInfo: version.Info(), InstanceID: s.cfg.InstanceID}
}

func (s *Service) InstanceID() string { return s.cfg.InstanceID }

func (s *Service) Bus() *events.Bus { return s.cfg.Bus }

func (s *Service) acquire(id domain.NetworkID, op string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return nil, fail(CodeShuttingDown, "daemon is shutting down", true)
	}
	if _, busy := s.ops[id]; busy {
		return nil, fail(CodeBusy, "another operation is in progress for this network", true)
	}
	s.ops[id] = op
	return func() {
		s.mu.Lock()
		delete(s.ops, id)
		s.mu.Unlock()
	}, nil
}

func (s *Service) checkRunning() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return fail(CodeShuttingDown, "daemon is shutting down", true)
	}
	return nil
}

func (s *Service) pendingOp(id domain.NetworkID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ops[id]
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()
	return s.cfg.Sessions.StopAll(ctx)
}

func (s *Service) storeErr(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fail(CodeNetworkNotFound, "network not found", false)
	case errors.Is(err, store.ErrInvalidInput):
		return fail(CodeInvalidArgument, "invalid network configuration", false)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fail(CodeBusy, "operation did not complete in time", true)
	}
	s.cfg.Log.Error("store operation failed", "err", err.Error())
	return fail(CodeInternal, "internal storage error", true)
}

func parseID(raw string) (domain.NetworkID, error) {
	id, err := domain.ParseNetworkID(raw)
	if err != nil {
		return "", fail(CodeInvalidArgument, "invalid network id", false)
	}
	return id, nil
}

func DefaultNodeHostname() string {
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(strings.ToLower(host), ".")
	var b strings.Builder
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	if domain.ValidateNodeHostname(out) != nil {
		return "lattice"
	}
	return out
}

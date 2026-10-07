package session

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
)

type backendLifecycle string

const (
	backendStopped  backendLifecycle = "stopped"
	backendStarting backendLifecycle = "starting"
	backendStarted  backendLifecycle = "started"
	backendStopping backendLifecycle = "stopping"
)

type EnginePeer struct {
	NodeID    domain.NodeID
	Hostname  string
	DNSName   string
	Addresses []netip.Addr
	Online    bool
	LastSeen  time.Time
}

type EngineStatus struct {
	BackendState string
	AuthURL      string
	TailscaleIPs []netip.Addr
	Self         *EnginePeer
	Peers        []EnginePeer
	Health       []string
}

type EngineNotify struct {
	BackendState  *string
	BrowseToURL   *string
	LoginFinished bool
	NetMapChanged bool
}

type Engine interface {
	Start() error
	Close() error
	Status(ctx context.Context) (EngineStatus, error)
	Watch(ctx context.Context, emit func(EngineNotify)) error
	ClearAuthKey()
}

type EngineFactory func(cfg provider.ResolvedSessionConfig, authKey string, log *slog.Logger) (Engine, error)

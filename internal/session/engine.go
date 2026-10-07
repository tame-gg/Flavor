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

type peerSnap struct {
	NodeID    domain.NodeID
	Hostname  string
	DNSName   string
	Addresses []netip.Addr
	Online    bool
	LastSeen  time.Time
}

type statusSnap struct {
	BackendState string
	AuthURL      string
	TailscaleIPs []netip.Addr
	Self         *peerSnap
	Peers        []peerSnap
	Health       []string
}

type notifySnap struct {
	BackendState  *string
	BrowseToURL   *string
	LoginFinished bool
	NetMapChanged bool
}

type engine interface {
	Start() error
	Close() error
	Status(ctx context.Context) (statusSnap, error)
	Watch(ctx context.Context, emit func(notifySnap)) error
	ClearAuthKey()
}

type engineFactory func(cfg provider.ResolvedSessionConfig, authKey string, log *slog.Logger) (engine, error)

package session

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
)

type backendLifecycle string

const (
	backendStopped  backendLifecycle = "stopped"
	backendStarting backendLifecycle = "starting"
	backendStarted  backendLifecycle = "started"
	backendStopping backendLifecycle = "stopping"
)

type EnginePeer struct {
	NodeID         domain.NodeID
	Hostname       string
	DNSName        string
	Addresses      []netip.Addr
	Online         bool
	LastSeen       time.Time
	OS             string
	Tags           []string
	Routes         []netip.Prefix
	ExitNodeOption bool
}

type EngineStatus struct {
	BackendState string
	AuthURL      string
	TailscaleIPs []netip.Addr
	Self         *EnginePeer
	Peers        []EnginePeer
	Health       []string
	DNSRecords   []domain.DNSRecord
	ExitNode     domain.NodeID

	AdvertisedRoutes []netip.Prefix
	ApprovedRoutes   []netip.Prefix
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
	Dial(ctx context.Context, network, address string) (net.Conn, error)
	SetExitNode(ctx context.Context, id domain.NodeID) error
	SetAdvertisedRoutes(ctx context.Context, routes []netip.Prefix) error
}

type EngineFactory func(cfg provider.ResolvedSessionConfig, authKey string, log *slog.Logger) (Engine, error)

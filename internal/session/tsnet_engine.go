package session

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"sync"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/logging"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/types/logger"
)

type tsnetEngine struct {
	srv *tsnet.Server
	lc  *local.Client

	mu      sync.Mutex
	records []domain.DNSRecord
}

func newTsnetEngine(cfg provider.ResolvedSessionConfig, authKey string, log *slog.Logger) (Engine, error) {
	if cfg.StateDir == "" || cfg.NodeHostname == "" {
		return nil, ErrInvalidConfig
	}
	if !processEnvPrepared() {
		return nil, ErrUnpreparedEnv
	}
	srv := &tsnet.Server{
		Dir:       cfg.StateDir,
		Hostname:  cfg.NodeHostname,
		AuthKey:   authKey,
		Ephemeral: false,
		UserLogf:  tsnetUserLogf(log.With("network_id", cfg.NetworkID, "provider", cfg.Provider)),
		Logf:      logger.Discard,
	}
	if cfg.ControlURL != "" {
		srv.ControlURL = cfg.ControlURL
	}
	return &tsnetEngine{srv: srv}, nil
}

var tsnetSecrets = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://\S+|\b(?:tskey|hskey)-\S+|\b(?:[a-z]+key|nlpriv|nlpub|chalpriv):[0-9a-f]+|\b[0-9a-f]{32,}\b|[a-z0-9_-]{40,}`)

func tsnetUserLogf(log *slog.Logger) logger.Logf {
	return func(format string, args ...any) {
		log.Debug("tsnet", "line", tsnetSecrets.ReplaceAllString(fmt.Sprintf(format, args...), logging.Redacted))
	}
}

func (e *tsnetEngine) Start() error {
	if err := e.srv.Start(); err != nil {
		return err
	}
	lc, err := e.srv.LocalClient()
	if err != nil {
		_ = e.srv.Close()
		return err
	}
	e.lc = lc
	return nil
}

func (e *tsnetEngine) Close() error {
	return e.srv.Close()
}

func (e *tsnetEngine) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return e.srv.Dial(ctx, network, address)
}

func (e *tsnetEngine) SetExitNode(ctx context.Context, id domain.NodeID) error {
	if e.lc == nil {
		return fmt.Errorf("local client unavailable")
	}
	_, err := e.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:           ipn.Prefs{ExitNodeID: tailcfg.StableNodeID(id)},
		ExitNodeIDSet:   true,
		ExitNodeIPSet:   true,
		AutoExitNodeSet: true,
	})
	return err
}

func (e *tsnetEngine) SetAdvertisedRoutes(ctx context.Context, routes []netip.Prefix) error {
	if e.lc == nil {
		return fmt.Errorf("local client unavailable")
	}
	_, err := e.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              ipn.Prefs{AdvertiseRoutes: routes},
		AdvertiseRoutesSet: true,
	})
	return err
}

func (e *tsnetEngine) ClearAuthKey() {
	e.srv.AuthKey = ""
}

func (e *tsnetEngine) Status(ctx context.Context) (EngineStatus, error) {
	if e.lc == nil {
		return EngineStatus{}, fmt.Errorf("local client unavailable")
	}
	st, err := e.lc.Status(ctx)
	if err != nil {
		return EngineStatus{}, err
	}
	out := projectStatus(st)
	if prefs, err := e.lc.GetPrefs(ctx); err == nil && prefs != nil {
		out.AdvertisedRoutes = slices.Clone(prefs.AdvertiseRoutes)
	}
	e.mu.Lock()
	out.DNSRecords = domain.CloneDNSRecords(e.records)
	e.mu.Unlock()
	return out, nil
}

func (e *tsnetEngine) Watch(ctx context.Context, emit func(EngineNotify)) error {
	if e.lc == nil {
		return fmt.Errorf("local client unavailable")
	}
	w, err := e.lc.WatchIPNBus(ctx, ipn.NotifyInitialState|ipn.NotifyInitialNetMap|ipn.NotifyNoPrivateKeys)
	if err != nil {
		return err
	}
	defer w.Close()
	for {
		n, err := w.Next()
		if err != nil {
			return err
		}
		if n.NetMap != nil {
			records := projectDNSRecords(&n.NetMap.DNS)
			e.mu.Lock()
			e.records = records
			e.mu.Unlock()
		}
		ev := EngineNotify{LoginFinished: n.LoginFinished != nil, NetMapChanged: n.NetMap != nil}
		if n.State != nil {
			s := n.State.String()
			ev.BackendState = &s
		}
		if n.BrowseToURL != nil {
			u := *n.BrowseToURL
			ev.BrowseToURL = &u
		}
		emit(ev)
	}
}

package session

import (
	"context"
	"fmt"
	"log/slog"

	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
	"tailscale.com/types/logger"
)

type tsnetEngine struct {
	srv *tsnet.Server
	lc  *local.Client
}

func newTsnetEngine(cfg provider.ResolvedSessionConfig, authKey string, log *slog.Logger) (engine, error) {
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

func tsnetUserLogf(log *slog.Logger) logger.Logf {
	return func(format string, _ ...any) {
		log.Debug("tsnet", "format", format)
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

func (e *tsnetEngine) ClearAuthKey() {
	e.srv.AuthKey = ""
}

func (e *tsnetEngine) Status(ctx context.Context) (statusSnap, error) {
	if e.lc == nil {
		return statusSnap{}, fmt.Errorf("local client unavailable")
	}
	st, err := e.lc.Status(ctx)
	if err != nil {
		return statusSnap{}, err
	}
	return projectStatus(st), nil
}

func (e *tsnetEngine) Watch(ctx context.Context, emit func(notifySnap)) error {
	if e.lc == nil {
		return fmt.Errorf("local client unavailable")
	}
	w, err := e.lc.WatchIPNBus(ctx, ipn.NotifyInitialState|ipn.NotifyNoPrivateKeys)
	if err != nil {
		return err
	}
	defer w.Close()
	for {
		n, err := w.Next()
		if err != nil {
			return err
		}
		ev := notifySnap{LoginFinished: n.LoginFinished != nil, NetMapChanged: n.NetMap != nil}
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

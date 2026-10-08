package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"

	"git.lunarlabs.dev/lattice/lattice/internal/dataplane"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/netd"
	"git.lunarlabs.dev/lattice/lattice/internal/service"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
	"git.lunarlabs.dev/lattice/lattice/internal/syndns"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
)

func startSynthetic(ctx context.Context, opts Options, db *store.DB, svc *service.Service, sessions *session.Manager, log *slog.Logger) (func(), error) {
	if opts.ExperimentalDNS == "" && opts.TUN == nil && opts.SyntheticHelper == "" {
		return func() {}, nil
	}
	alloc, err := synthetic.Open(ctx, db, nil)
	if err != nil {
		return nil, err
	}
	if err := alloc.EnsurePool(ctx, synthetic.LocalPrefixes()); err != nil {
		log.Warn("ipv4 synthetic addresses disabled", "err", err.Error())
	}
	engine := &syndns.Engine{Resolver: svc, Addresser: alloc, Ambiguous: svc.WarnAmbiguousName}
	if opts.ExperimentalDNS != "" {
		addr, err := serveExperimentalDNS(ctx, opts.ExperimentalDNS, engine)
		if err != nil {
			return nil, err
		}
		log.Info("experimental synthetic dns listening", "addr", addr.String())
		if opts.OnDNSReady != nil {
			opts.OnDNSReady(addr)
		}
	}
	dial := func(ctx context.Context, id domain.NetworkID, proto, address string) (net.Conn, error) {
		s, ok := sessions.Get(id)
		if !ok {
			return nil, session.ErrNotRunning
		}
		return s.Dial(ctx, proto, address)
	}
	if opts.TUN != nil {
		plane, err := dataplane.Start(ctx, opts.TUN, alloc, engine, dial, log)
		if err != nil {
			return nil, err
		}
		return plane.Close, nil
	}
	if opts.SyntheticHelper == "" {
		return func() {}, nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		netd.Supervisor{
			Socket: opts.SyntheticHelper,
			V6:     alloc.ULA(),
			V4:     alloc.Pool(),
			MTU:    dataplane.MTU,
			Log:    log,
			Warn:   svc.Warn,
			Attach: func(ctx context.Context, tun *os.File) (<-chan struct{}, func(), error) {
				plane, err := dataplane.Start(ctx, tun, alloc, engine, dial, log)
				if err != nil {
					return nil, nil, err
				}
				return plane.Done(), plane.Close, nil
			},
		}.Run(ctx)
	}()
	return func() { <-done }, nil
}

func serveExperimentalDNS(ctx context.Context, listen string, engine *syndns.Engine) (net.Addr, error) {
	ap, err := netip.ParseAddrPort(listen)
	if err != nil || !ap.Addr().Unmap().IsLoopback() {
		return nil, errors.New("listen address must be a loopback IP and port")
	}
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp", ap.String())
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()
	go engine.ServePacket(ctx, pc)
	return pc.LocalAddr(), nil
}

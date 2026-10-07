package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"

	"git.lunarlabs.dev/lattice/lattice/internal/service"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
	"git.lunarlabs.dev/lattice/lattice/internal/syndns"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
)

func serveExperimentalDNS(ctx context.Context, listen string, db *store.DB, svc *service.Service, log *slog.Logger) (net.Addr, error) {
	ap, err := netip.ParseAddrPort(listen)
	if err != nil || !ap.Addr().Unmap().IsLoopback() {
		return nil, errors.New("listen address must be a loopback IP and port")
	}
	alloc, err := synthetic.Open(ctx, db, nil)
	if err != nil {
		return nil, err
	}
	engine := &syndns.Engine{Resolver: svc, Addresser: alloc, Ambiguous: svc.WarnAmbiguousName}
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp", ap.String())
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			resp, err := engine.Answer(ctx, buf[:n])
			if err != nil {
				log.Debug("dropping malformed dns query")
				continue
			}
			_, _ = pc.WriteTo(resp, from)
		}
	}()
	return pc.LocalAddr(), nil
}

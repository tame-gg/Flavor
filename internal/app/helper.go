//go:build linux || windows

package app

import (
	"context"
	"log/slog"
	"net/netip"

	"git.lunarlabs.dev/flavor/flavor/internal/dataplane"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
	"git.lunarlabs.dev/flavor/flavor/internal/syndns"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
)

func superviseHelper(ctx context.Context, socket string, alloc *synthetic.Allocator, engine *syndns.Engine, dial dataplane.Dialer, svc *service.Service, log *slog.Logger) (func(), error) {
	done := make(chan struct{})
	routes := &dataplane.RouteTable{}
	go func() {
		defer close(done)
		netd.Supervisor{
			Socket:  socket,
			V6:      alloc.ULA(),
			V4:      alloc.Pool(),
			MTU:     dataplane.MTU,
			Log:     log,
			Warn:    svc.Warn,
			Domains: func() []string { return svc.RoutingDomains(ctx) },
			Routes: func() []netip.Prefix {
				want := map[netip.Prefix]domain.NetworkID{}
				var out []netip.Prefix
				for _, r := range svc.SystemRoutes(ctx) {
					want[r.Prefix] = r.Network
					out = append(out, r.Prefix)
				}
				routes.Want(want)
				return out
			},
			Applied: func(active bool, installed []netip.Prefix, refused map[netip.Prefix]string) {
				routes.Install(installed)
				svc.SetInstalledRoutes(active, installed, refused)
			},
			Nudge: svc.RoutesChanged(),
			Attach: func(ctx context.Context, tun netd.Packets) (<-chan struct{}, func(), error) {
				plane, err := dataplane.Start(ctx, tun, alloc, routes, engine, dial, log)
				if err != nil {
					return nil, nil, err
				}
				return plane.Done(), plane.Close, nil
			},
		}.Run(ctx)
	}()
	return func() { <-done }, nil
}

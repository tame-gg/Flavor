package app

import (
	"context"
	"log/slog"
	"os"

	"git.lunarlabs.dev/flavor/flavor/internal/dataplane"
	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
	"git.lunarlabs.dev/flavor/flavor/internal/syndns"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
)

func superviseHelper(ctx context.Context, socket string, alloc *synthetic.Allocator, engine *syndns.Engine, dial dataplane.Dialer, svc *service.Service, log *slog.Logger) (func(), error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		netd.Supervisor{
			Socket: socket,
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

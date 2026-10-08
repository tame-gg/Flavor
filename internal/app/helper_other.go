//go:build !linux

package app

import (
	"context"
	"errors"
	"log/slog"

	"git.lunarlabs.dev/flavor/flavor/internal/dataplane"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
	"git.lunarlabs.dev/flavor/flavor/internal/syndns"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
)

func superviseHelper(context.Context, string, *synthetic.Allocator, *syndns.Engine, dataplane.Dialer, *service.Service, *slog.Logger) (func(), error) {
	return nil, errors.New("system-wide names through flavor-netd are only supported on Linux")
}

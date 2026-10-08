package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"github.com/godbus/dbus/v5"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("flavor-netd failed", "err", err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ln, err := netd.ActivatedListener()
	if err != nil {
		return err
	}
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer bus.Close()
	srv := netd.NewServer(netd.Config{
		Kernel:     netd.Netlink{},
		DNS:        netd.Resolved{Conn: bus},
		Authorizer: netd.Polkit{Conn: bus, Log: log},
		Sessions:   netd.Logind{Conn: bus},
		Log:        log,
		IdleExit:   netd.DefaultIdleExit,
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	_ = netd.Notify("READY=1")
	err = srv.Serve(ctx, ln)
	_ = netd.Notify("STOPPING=1")
	return err
}

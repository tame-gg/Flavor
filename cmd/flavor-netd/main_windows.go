package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"golang.org/x/sys/windows/svc"
)

func main() {
	if len(os.Args) == 2 {
		var err error
		switch os.Args[1] {
		case "install":
			err = install()
		case "uninstall":
			err = uninstall()
		case "check":
			if !current() {
				os.Exit(1)
			}
			return
		default:
			err = fmt.Errorf("unknown command %q", os.Args[1])
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "flavor-netd:", err)
			os.Exit(1)
		}
		return
	}
	if err := svc.Run(netd.ServiceName, helper{}); err != nil {
		fmt.Fprintln(os.Stderr, "flavor-netd: run it through install; it is a Windows service:", err)
		os.Exit(1)
	}
}

type helper struct{}

func (helper) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	var out io.Writer = io.Discard
	if exe, err := os.Executable(); err == nil {
		if f, err := os.Create(filepath.Join(filepath.Dir(exe), "flavor-netd.log")); err == nil {
			defer f.Close()
			out = f
		}
	}
	log := slog.New(slog.NewTextHandler(out, nil))
	if err := (netd.NRPT{}).Revert(context.Background(), 0); err != nil {
		log.Warn("removing stale dns rules failed", "err", err.Error())
	}
	ln, err := netd.Listen(netd.DefaultSocket)
	if err != nil {
		log.Error("listening failed", "err", err.Error())
		return true, 1
	}
	srv := netd.NewServer(netd.Config{
		Kernel:     netd.NewWintun(),
		DNS:        netd.NRPT{},
		Authorizer: netd.Interactive{},
		Sessions:   netd.Terminal{},
		Log:        log,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				log.Error("flavor-netd failed", "err", err.Error())
				return true, 1
			}
			return false, 0
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				return false, 0
			}
		}
	}
}

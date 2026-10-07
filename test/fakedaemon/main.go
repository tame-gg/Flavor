package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

func main() {
	runtimeDir := flag.String("runtime-dir", "", "runtime directory parent")
	peers := flag.Int("peers", 3, "synthetic peers per connected network")
	flag.Parse()

	seq := &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		switch host := strings.ToLower(cfg.ControlURL); {
		case strings.Contains(host, "login"):
			e.SetStatus(sessiontest.StatusNeedsLogin("https://login.example.com/a/" + string(cfg.NetworkID)))
		case strings.Contains(host, "approval"):
			e.SetStatus(sessiontest.StatusNeedsMachineAuth())
		default:
			e.SetStatus(status(cfg.NetworkID, *peers))
		}
	}}

	log := logging.New(os.Stderr, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := app.Run(ctx, app.Options{
		RuntimeOverride: *runtimeDir,
		SecretMode:      secret.ModeMemory,
		Log:             log,
		EngineFactory:   seq.Factory,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func status(id domain.NetworkID, peers int) session.EngineStatus {
	st := sessiontest.StatusSelf("self-"+string(id), "100.64.0.1")
	for i := range peers {
		addr := netip.AddrFrom4([4]byte{100, 64, byte((i + 2) >> 8), byte(i + 2)})
		node := fmt.Sprintf("peer-%04d", i+1)
		st.Peers = append(st.Peers, session.EnginePeer{
			NodeID:    domain.NodeID(node + "-" + string(id)),
			Hostname:  node,
			DNSName:   node + ".tail.example.",
			Addresses: []netip.Addr{addr},
			Online:    i%4 != 3,
		})
	}
	return st
}

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
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
	experimentalDNS := flag.String("experimental-dns", "", "serve synthetic DNS on this loopback address")
	flag.Parse()

	seq := &sessiontest.Sequence{Prepare: func(n int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		switch host := strings.ToLower(cfg.ControlURL); {
		case strings.Contains(host, "login"):
			e.SetStatus(sessiontest.StatusNeedsLogin("https://login.example.com/a/" + string(cfg.NetworkID)))
		case strings.Contains(host, "approval"):
			e.SetStatus(sessiontest.StatusNeedsMachineAuth())
		default:
			e.SetStatus(status(cfg, n, *peers))
		}
		e.SetDial(identify(cfg))
	}}

	log := logging.New(os.Stderr, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := app.Run(ctx, app.Options{
		RuntimeOverride: *runtimeDir,
		SecretMode:      secret.ModeMemory,
		Log:             log,
		EngineFactory:   seq.Factory,
		ExperimentalDNS: *experimentalDNS,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var peerNames = []string{"postgres", "grafana", "prod-api", "nas", "build-runner", "pi-hole"}

var peerTags = map[string][]string{
	"postgres": {"tag:db"},
	"grafana":  {"tag:monitoring"},
	"prod-api": {"tag:prod", "tag:api"},
}

var subnetRoutes = [][]string{
	{"10.10.0.0/16", "10.0.0.0/8"},
	{"10.10.0.0/16"},
	{"10.20.30.0/24"},
}

var peerOS = []string{"linux", "linux", "linux", "windows", "macOS", "linux"}

func status(cfg provider.ResolvedSessionConfig, n, peers int) session.EngineStatus {
	id := cfg.NetworkID
	st := sessiontest.StatusSelf("self-"+string(id), "100.64.0.1")
	st.Self.Hostname = cfg.NodeHostname
	st.Self.DNSName = cfg.NodeHostname + "." + strings.ToLower(string(id)) + ".lattice.test"
	st.Self.OS = "linux"
	for i := range peers {
		addr := netip.AddrFrom4([4]byte{100, 64, byte((i + 2) >> 8), byte(i + 2)})
		node := fmt.Sprintf("peer-%04d", i+1)
		if i < len(peerNames) {
			node = peerNames[(i+n)%len(peerNames)]
		}
		st.Peers = append(st.Peers, session.EnginePeer{
			NodeID:    domain.NodeID(fmt.Sprintf("%s-%d", id, i+1)),
			Hostname:  node,
			DNSName:   node + "." + strings.ToLower(string(id)) + ".lattice.test.",
			Addresses: []netip.Addr{addr},
			Online:    i%4 != 3,
			OS:        peerOS[(i+n)%len(peerOS)],
			Tags:      peerTags[node],
			Routes:    routesFor(i, n),
		})
	}
	return st
}

func routesFor(peer, network int) []netip.Prefix {
	if peer != 0 {
		return nil
	}
	var out []netip.Prefix
	for _, r := range subnetRoutes[network%len(subnetRoutes)] {
		out = append(out, netip.MustParsePrefix(r))
	}
	return out
}

func identify(cfg provider.ResolvedSessionConfig) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _, address string) (net.Conn, error) {
		ours, theirs := net.Pipe()
		go func() {
			defer theirs.Close()
			_, _ = bufio.NewReader(theirs).ReadString('\n')
			body := fmt.Sprintf("network=%s target=%s\n", cfg.NetworkID, address)
			fmt.Fprintf(theirs, "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
		}()
		return ours, nil
	}
}

package app_test

import (
	"bufio"
	"context"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/dataplane/dataplanetest"
	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
	"golang.org/x/net/dns/dnsmessage"
)

func TestSyntheticDataPlaneEndToEnd(t *testing.T) {
	plane, kernel := dataplanetest.Pair(t)
	ready := make(chan net.Addr, 1)
	d := start(t, testEnv(t), app.Options{
		EngineFactory:   identifyingEngines().Factory,
		ExperimentalDNS: "127.0.0.1:0",
		OnDNSReady:      func(a net.Addr) { ready <- a },
		TUN:             plane,
	})
	server := (<-ready).String()
	c := d.client
	ctx := context.Background()
	lunar := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	home := addHeadscale(t, c, "Home", "https://home.example.com", false)
	for _, id := range []string{lunar.Id, home.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}

	v6 := ask(t, server, "postgres.home.flavor.internal", dnsmessage.TypeAAAA, false).addrs
	v4 := ask(t, server, "postgres.home.flavor.internal", dnsmessage.TypeA, false).addrs
	if len(v6) != 1 || len(v4) != 1 {
		t.Fatalf("%v %v", v6, v4)
	}
	ula, pool := netip.PrefixFrom(v6[0], 48).Masked(), netip.PrefixFrom(v4[0], synthetic.PoolBits).Masked()
	peer := dataplanetest.NewPeer(t, kernel, ula, pool)

	resolve := func(server netip.Addr, framed bool, name string, qtype dnsmessage.Type) dnsAnswer {
		t.Helper()
		ap := netip.AddrPortFrom(server, 53)
		var conn net.Conn
		var err error
		if framed {
			dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			conn, err = peer.DialTCP(dctx, ap)
		} else {
			conn, err = peer.DialUDP(ap)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		return askOn(t, conn, framed, name, qtype, false)
	}
	r6, r4 := layout.ResolverAddress(ula), layout.ResolverV4(pool)
	if got := resolve(r6, false, "postgres.home.flavor.internal", dnsmessage.TypeAAAA); len(got.addrs) != 1 || got.addrs[0] != v6[0] {
		t.Fatalf("netstack resolver over udp: %+v", got)
	}
	if got := resolve(r4, true, "postgres.home.flavor.internal", dnsmessage.TypeA); len(got.addrs) != 1 || got.addrs[0] != v4[0] {
		t.Fatalf("netstack resolver over tcp: %+v", got)
	}
	if got := resolve(r6, true, "postgres", dnsmessage.TypeAAAA); got.rcode != dnsmessage.RCodeServerFailure {
		t.Fatalf("ambiguous names must fail inside the netstack too: %+v", got)
	}
	lunar6 := resolve(r4, false, "postgres.lunarlabs.flavor.internal", dnsmessage.TypeAAAA).addrs
	lunar4 := resolve(r6, false, "postgres.lunarlabs.flavor.internal", dnsmessage.TypeA).addrs
	if len(lunar6) != 1 || len(lunar4) != 1 {
		t.Fatalf("%v %v", lunar6, lunar4)
	}

	for _, tc := range []struct {
		dst     netip.Addr
		network string
	}{{v6[0], home.Id}, {v4[0], home.Id}, {lunar6[0], lunar.Id}, {lunar4[0], lunar.Id}} {
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		conn, err := peer.DialTCP(dctx, netip.AddrPortFrom(tc.dst, 5432))
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", tc.dst, err)
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		line, err := bufio.NewReader(conn).ReadString('\n')
		conn.Close()
		if want := "network=" + tc.network + " target=100.64.0.9:5432\n"; err != nil || line != want {
			t.Fatalf("%s reached %q (%v), want %q", tc.dst, line, err, want)
		}
	}

	if _, err := c.Networks.DisconnectNetwork(ctx, connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: home.Id})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "disconnected", func() bool {
		return networkState(t, c, home.Id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISCONNECTED
	})
	dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if conn, err := peer.DialTCP(dctx, netip.AddrPortFrom(v6[0], 5432)); err == nil {
		conn.Close()
		t.Fatal("a disconnected network must refuse, never fall back to another network")
	}
}

func TestMissingHelperDegradesToAWarning(t *testing.T) {
	d := start(t, testEnv(t), app.Options{
		EngineFactory:   identifyingEngines().Factory,
		SyntheticHelper: filepath.Join(t.TempDir(), "netd.sock"),
	})
	col, stop := watch(t, d.client, snapshot(t, d.client).DaemonInstanceId, 0)
	defer stop()
	eventually(t, "synthetic warning", func() bool {
		for _, ev := range col.snapshot() {
			if w := ev.GetDaemonWarning(); w != nil && w.Code == netd.WarnUnavailable && strings.Contains(w.SafeMessage, "not reachable") {
				return true
			}
		}
		return false
	})
	addHeadscale(t, d.client, "Home", "https://home.example.com", false)
}

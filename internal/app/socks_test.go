package app_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"golang.org/x/net/proxy"
)

func socksReplyCode(t *testing.T, proxyAddr string, req []byte) byte {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var sel [2]byte
	if _, err := io.ReadFull(c, sel[:]); err != nil || sel != [2]byte{5, 0} {
		t.Fatalf("method selection %v %v", sel, err)
	}
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	var reply [10]byte
	if _, err := io.ReadFull(c, reply[:]); err != nil {
		t.Fatal(err)
	}
	return reply[1]
}

func domainRequest(cmd byte, host string, port uint16) []byte {
	b := []byte{5, cmd, 0, 3, byte(len(host))}
	b = append(b, host...)
	return append(b, byte(port>>8), byte(port))
}

func TestSocksRoutesEachRequestThroughTheDecisionEngine(t *testing.T) {
	d := start(t, testEnv(t), app.Options{EngineFactory: identifyingEngines().Factory})
	c := d.client
	ctx := context.Background()
	a := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, c, "Home", "https://home.example.com", false)
	for _, id := range []string{a.Id, b.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.Forwards.Proxy(sctx, connect.NewRequest(&v1.ProxyRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() {
		t.Fatal(stream.Err())
	}
	addr := stream.Msg().GetStarted().GetListenAddress()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("socks must bind loopback by default: %q", addr)
	}
	go func() {
		for stream.Receive() {
		}
	}()

	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	read := func(dest string) string {
		t.Helper()
		conn, err := dialer.Dial("tcp", dest)
		if err != nil {
			t.Fatalf("%s: %v", dest, err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(line)
	}

	var wg sync.WaitGroup
	got := map[string]string{}
	var mu sync.Mutex
	for _, dest := range []string{"postgres.home.lattice.internal:5432", "postgres.lunarlabs.lattice.internal:5432"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				line := read(dest)
				mu.Lock()
				if prev, ok := got[dest]; ok && prev != line {
					t.Errorf("%s answered by two networks: %q then %q", dest, prev, line)
				}
				got[dest] = line
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if got["postgres.home.lattice.internal:5432"] != "network="+b.Id+" target=100.64.0.9:5432" ||
		got["postgres.lunarlabs.lattice.internal:5432"] != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("concurrent sessions crossed networks: %v", got)
	}

	if code := socksReplyCode(t, addr, []byte{5, 1, 0, 1, 100, 64, 0, 9, 0x15, 0x38}); code != 0x02 {
		t.Fatalf("ambiguous raw IPv4 must be refused by policy, got reply %#x", code)
	}
	if code := socksReplyCode(t, addr, domainRequest(1, "example.com", 80)); code != 0x04 {
		t.Fatalf("destination outside Lattice networks: reply %#x", code)
	}
	if code := socksReplyCode(t, addr, domainRequest(2, "postgres.home.lattice.internal", 5432)); code != 0x07 {
		t.Fatalf("BIND must be unsupported: reply %#x", code)
	}

	if _, err := c.Preferences.SetDestinationPreference(ctx, connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: "100.64.0.9", NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if line := read("100.64.0.9:5432"); line != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("preference: %q", line)
	}

	cancel()
	eventually(t, "proxy closed", func() bool {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
		}
		return err != nil
	})
}

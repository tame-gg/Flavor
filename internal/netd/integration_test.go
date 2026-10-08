package netd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/dataplane"
	"git.lunarlabs.dev/lattice/lattice/internal/dataplane/dataplanetest"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
	"git.lunarlabs.dev/lattice/lattice/internal/syndns"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
	"golang.org/x/net/dns/dnsmessage"
)

type nothing struct{}

func (nothing) Inspect(context.Context, string) (inspect.Result, uint64, error) {
	return inspect.Result{}, 0, errors.New("no such device")
}

func TestHelperDescriptorDrivesTheDataPlane(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lattice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	network := domain.Network{ID: domain.NewNetworkID(), DisplayName: "Home", Provider: domain.ProviderTailscale, NodeHostname: "ws", CreatedAt: now, UpdatedAt: now}
	if err := db.Networks().Create(ctx, network); err != nil {
		t.Fatal(err)
	}
	alloc, err := synthetic.Open(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := alloc.EnsurePool(ctx, nil); err != nil {
		t.Fatal(err)
	}

	h := newHarness(t, nil)
	c := h.client()
	resp, tun, err := c.Create(alloc.ULA(), alloc.Pool(), dataplane.MTU)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConfigureDNS(nil); err != nil {
		t.Fatal(err)
	}
	dial := func(_ context.Context, id domain.NetworkID, proto, address string) (net.Conn, error) {
		near, far := net.Pipe()
		go func() {
			defer far.Close()
			fmt.Fprintf(far, "network=%s %s %s\n", id, proto, address)
		}()
		return near, nil
	}
	plane, err := dataplane.Start(ctx, tun, alloc, &syndns.Engine{Resolver: nothing{}, Addresser: alloc}, dial, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer plane.Close()
	peer := dataplanetest.NewPeer(t, h.kernel.link(resp.InterfaceName).kernel, alloc.ULA(), alloc.Pool())

	var servers []netip.Addr
	h.with(func() { servers = h.dns.servers })
	for _, raw := range [][]byte{resp.V6ResolverAddress, resp.V4ResolverAddress} {
		server, _ := netip.AddrFromSlice(raw)
		if !slices.Contains(servers, server) {
			t.Fatalf("resolved is pointed at %v but the helper advertised %v", servers, server)
		}
		u, err := peer.DialUDP(netip.AddrPortFrom(server, 53))
		if err != nil {
			t.Fatal(err)
		}
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 9})
		_ = b.StartQuestions()
		_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName("x.home.lattice.internal."), Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET})
		q, _ := b.Finish()
		_ = u.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = u.Write(q)
		buf := make([]byte, 1500)
		n, err := u.Read(buf)
		var p dnsmessage.Parser
		hdr, perr := p.Start(buf[:n])
		if err != nil || perr != nil || hdr.ID != 9 || hdr.RCode != dnsmessage.RCodeNameError {
			t.Fatalf("resolver %s: %v %v %v", server, err, perr, hdr)
		}
		u.Close()
	}

	a, err := alloc.For(ctx, network.ID, netip.MustParseAddr("100.64.0.7"))
	if err != nil {
		t.Fatal(err)
	}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := peer.DialTCP(dctx, netip.AddrPortFrom(a.V4, 443))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if want := fmt.Sprintf("network=%s tcp 100.64.0.7:443\n", network.ID); err != nil || line != want {
		t.Fatalf("%q %v", line, err)
	}
	conn.Close()

	if err := c.Destroy(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-plane.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the data plane must stop when the helper removes the device")
	}
}

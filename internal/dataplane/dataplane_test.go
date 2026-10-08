package dataplane

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/dataplane/dataplanetest"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
	"git.lunarlabs.dev/lattice/lattice/internal/syndns"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
	"git.lunarlabs.dev/lattice/lattice/internal/synthetic/layout"
	"golang.org/x/net/dns/dnsmessage"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

var shared = netip.MustParseAddr("100.64.0.2")

type upstreams struct {
	mu       sync.Mutex
	fail     bool
	keepOpen bool
	calls    []string
	conns    chan net.Conn
}

func (u *upstreams) dial(_ context.Context, network domain.NetworkID, proto, address string) (net.Conn, error) {
	u.mu.Lock()
	u.calls = append(u.calls, fmt.Sprintf("%s %s %s", network, proto, address))
	fail, keepOpen := u.fail, u.keepOpen
	u.mu.Unlock()
	if fail {
		return nil, errors.New("unreachable")
	}
	greeting := fmt.Sprintf("network=%s %s %s\n", network, proto, address)
	if proto == "tcp" {
		near, far, err := tcpPair()
		if err != nil {
			return nil, err
		}
		u.conns <- near
		go func() {
			_, _ = far.Write([]byte(greeting))
			_, _ = io.Copy(far, far)
			if !keepOpen {
				far.Close()
			}
		}()
		return near, nil
	}
	near, far := net.Pipe()
	go func() {
		defer far.Close()
		buf := make([]byte, 1500)
		for {
			n, err := far.Read(buf)
			if err != nil {
				return
			}
			_, _ = far.Write(append([]byte(greeting), buf[:n]...))
		}
	}()
	return near, nil
}

func tcpPair() (net.Conn, net.Conn, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer ln.Close()
	near, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}
	far, err := ln.Accept()
	if err != nil {
		near.Close()
		return nil, nil, err
	}
	return near, far, nil
}

type notFound struct{}

func (notFound) Inspect(context.Context, string) (inspect.Result, uint64, error) {
	return inspect.Result{}, 0, errors.New("no such device")
}

type fixture struct {
	alloc *synthetic.Allocator
	ids   []domain.NetworkID
	up    *upstreams
	plane *Plane
	peer  *dataplanetest.Peer
	raw   *os.File
}

func start(t *testing.T, withPeer bool) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lattice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fixture{up: &upstreams{conns: make(chan net.Conn, 16)}}
	for _, name := range []string{"Home", "LunarLabs"} {
		now := time.Now().UTC()
		n := domain.Network{ID: domain.NewNetworkID(), DisplayName: name, Provider: domain.ProviderTailscale, NodeHostname: "ws", CreatedAt: now, UpdatedAt: now}
		if err := db.Networks().Create(ctx, n); err != nil {
			t.Fatal(err)
		}
		f.ids = append(f.ids, n.ID)
	}
	if f.alloc, err = synthetic.Open(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.alloc.EnsurePool(ctx, nil); err != nil {
		t.Fatal(err)
	}
	dev, other := dataplanetest.Pair(t)
	f.plane, err = Start(ctx, dev, f.alloc, &syndns.Engine{Resolver: notFound{}, Addresser: f.alloc}, f.up.dial, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.plane.Close)
	if withPeer {
		f.peer = dataplanetest.NewPeer(t, other, f.alloc.ULA(), f.alloc.Pool())
	} else {
		f.raw = other
	}
	return f
}

func (f *fixture) synthetic(t *testing.T, i int, real netip.Addr) synthetic.Addresses {
	t.Helper()
	a, err := f.alloc.For(context.Background(), f.ids[i], real)
	if err != nil || !a.V4.IsValid() || !a.V6.IsValid() {
		t.Fatalf("%v %v", a, err)
	}
	return a
}

func dialTCP(t *testing.T, f *fixture, ap netip.AddrPort) (net.Conn, *bufio.Reader) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := f.peer.DialTCP(ctx, ap)
	if err != nil {
		t.Fatalf("dial %s: %v", ap, err)
	}
	t.Cleanup(func() { c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	return c, bufio.NewReader(c)
}

func TestTCPReachesTheDecodedNetworkEvenWhenTheRawAddressIsAmbiguous(t *testing.T) {
	f := start(t, true)
	for i := range f.ids {
		a := f.synthetic(t, i, shared)
		for _, dst := range []netip.Addr{a.V6, a.V4} {
			c, r := dialTCP(t, f, netip.AddrPortFrom(dst, 8080))
			line, err := r.ReadString('\n')
			want := fmt.Sprintf("network=%s tcp 100.64.0.2:8080\n", f.ids[i])
			if err != nil || line != want {
				t.Fatalf("%s: %q %v, want %q", dst, line, err, want)
			}
			if _, err := c.Write([]byte("ping\n")); err != nil {
				t.Fatal(err)
			}
			if echo, err := r.ReadString('\n'); err != nil || echo != "ping\n" {
				t.Fatalf("echo %q %v", echo, err)
			}
		}
	}
}

func TestUDPReachesTheDecodedNetwork(t *testing.T) {
	f := start(t, true)
	for i := range f.ids {
		a := f.synthetic(t, i, shared)
		for _, dst := range []netip.Addr{a.V6, a.V4} {
			c, err := f.peer.DialUDP(netip.AddrPortFrom(dst, 5353))
			if err != nil {
				t.Fatal(err)
			}
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			for _, msg := range []string{"one", "two"} {
				if _, err := c.Write([]byte(msg)); err != nil {
					t.Fatal(err)
				}
				buf := make([]byte, 1500)
				n, err := c.Read(buf)
				want := fmt.Sprintf("network=%s udp 100.64.0.2:5353\n%s", f.ids[i], msg)
				if err != nil || string(buf[:n]) != want {
					t.Fatalf("%s: %q %v, want %q", dst, buf[:n], err, want)
				}
			}
			c.Close()
		}
	}
}

func TestUnknownDestinationsAreRefusedNotAccepted(t *testing.T) {
	f := start(t, true)
	pool := f.alloc.Pool()
	unmapped := pool.Addr().Next().Next().Next().Next().Next()
	other, _ := layout.EmbedV4(f.alloc.ULA(), 999, shared)
	for _, ap := range []netip.AddrPort{
		netip.AddrPortFrom(unmapped, 80),
		netip.AddrPortFrom(other, 80),
		netip.AddrPortFrom(layout.ResolverAddress(f.alloc.ULA()), 80),
		netip.AddrPortFrom(layout.ResolverV4(pool), 80),
		netip.AddrPortFrom(layout.HostV4(pool), 80),
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := f.peer.DialTCP(ctx, ap)
		cancel()
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("%s: want connection refused, got %v", ap, err)
		}
	}
	f.up.mu.Lock()
	f.up.fail = true
	f.up.mu.Unlock()
	a := f.synthetic(t, 0, shared)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.peer.DialTCP(ctx, netip.AddrPortFrom(a.V6, 80)); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("an unreachable upstream must refuse the handshake, got %v", err)
	}
	f.up.mu.Lock()
	defer f.up.mu.Unlock()
	if len(f.up.calls) != 1 {
		t.Fatalf("only the mapped destination may be dialed: %v", f.up.calls)
	}
}

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 7, RecursionDesired: true})
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET})
	q, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func rcode(t *testing.T, resp []byte) dnsmessage.RCode {
	t.Helper()
	var p dnsmessage.Parser
	h, err := p.Start(resp)
	if err != nil || h.ID != 7 || !h.Response {
		t.Fatalf("bad response %v %v", h, err)
	}
	return h.RCode
}

func TestResolverAnswersOverUDPAndTCPOnBothFamilies(t *testing.T) {
	f := start(t, true)
	q := query(t, "missing.home.lattice.internal.", dnsmessage.TypeAAAA)
	for _, server := range []netip.Addr{layout.ResolverAddress(f.alloc.ULA()), layout.ResolverV4(f.alloc.Pool())} {
		ap := netip.AddrPortFrom(server, 53)
		u, err := f.peer.DialUDP(ap)
		if err != nil {
			t.Fatal(err)
		}
		_ = u.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = u.Write(q)
		buf := make([]byte, 1500)
		n, err := u.Read(buf)
		if err != nil || rcode(t, buf[:n]) != dnsmessage.RCodeNameError {
			t.Fatalf("udp %s: %v", server, err)
		}
		u.Close()

		c, r := dialTCP(t, f, ap)
		for range 2 {
			if _, err := c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(q))), q...)); err != nil {
				t.Fatal(err)
			}
			var size [2]byte
			if _, err := io.ReadFull(r, size[:]); err != nil {
				t.Fatalf("tcp %s: %v", server, err)
			}
			resp := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(r, resp); err != nil || rcode(t, resp) != dnsmessage.RCodeNameError {
				t.Fatalf("tcp %s: %v", server, err)
			}
		}
	}
	f.up.mu.Lock()
	defer f.up.mu.Unlock()
	if len(f.up.calls) != 0 {
		t.Fatalf("dns must be answered locally: %v", f.up.calls)
	}
}

func TestOpenFlowsHoldTheirSyntheticAddress(t *testing.T) {
	f := start(t, true)
	ctx := context.Background()
	a := f.synthetic(t, 0, shared)
	c, r := dialTCP(t, f, netip.AddrPortFrom(a.V4, 22))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if err := f.alloc.Release(ctx, a.V4); !errors.Is(err, synthetic.ErrInUse) {
		t.Fatalf("an address with an open flow must not be released: %v", err)
	}
	c.Close()
	eventually(t, func() bool { return f.alloc.Release(ctx, a.V4) == nil })
}

func TestCloseEndsFlowsAndReleasesReferences(t *testing.T) {
	f := start(t, true)
	f.up.mu.Lock()
	f.up.keepOpen = true
	f.up.mu.Unlock()
	a := f.synthetic(t, 0, shared)
	_, r := dialTCP(t, f, netip.AddrPortFrom(a.V4, 22))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	near := <-f.up.conns
	closed := make(chan struct{})
	go func() {
		f.plane.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close must not wait for upstreams that keep their side open")
	}
	if _, err := near.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("upstream must be closed with the plane: %v", err)
	}
	if err := f.alloc.Release(context.Background(), a.V4); err != nil {
		t.Fatalf("references must be released on shutdown: %v", err)
	}
}

func TestDeviceLossShutsThePlaneDown(t *testing.T) {
	f := start(t, true)
	a := f.synthetic(t, 1, shared)
	_, r := dialTCP(t, f, netip.AddrPortFrom(a.V4, 22))
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	near := <-f.up.conns
	f.peer.Close()
	select {
	case <-f.plane.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("plane did not shut down after losing its device")
	}
	if _, err := near.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("losing the device must close upstream flows: %v", err)
	}
}

func echoRequests(src4, dst4, src6, dst6 netip.Addr) [][]byte {
	v4 := make([]byte, header.IPv4MinimumSize+header.ICMPv4MinimumSize)
	ip4 := header.IPv4(v4)
	ip4.Encode(&header.IPv4Fields{TotalLength: uint16(len(v4)), TTL: 64, Protocol: uint8(header.ICMPv4ProtocolNumber), SrcAddr: tcpip.AddrFrom4(src4.As4()), DstAddr: tcpip.AddrFrom4(dst4.As4())})
	ip4.SetChecksum(^ip4.CalculateChecksum())
	ic4 := header.ICMPv4(v4[header.IPv4MinimumSize:])
	ic4.SetType(header.ICMPv4Echo)
	ic4.SetIdent(1)
	ic4.SetSequence(1)
	ic4.SetChecksum(header.ICMPv4Checksum(ic4, 0))

	v6 := make([]byte, header.IPv6MinimumSize+header.ICMPv6EchoMinimumSize)
	ip6 := header.IPv6(v6)
	src, dst := tcpip.AddrFrom16(src6.As16()), tcpip.AddrFrom16(dst6.As16())
	ip6.Encode(&header.IPv6Fields{PayloadLength: header.ICMPv6EchoMinimumSize, TransportProtocol: header.ICMPv6ProtocolNumber, HopLimit: 64, SrcAddr: src, DstAddr: dst})
	ic6 := header.ICMPv6(v6[header.IPv6MinimumSize:])
	ic6.SetType(header.ICMPv6EchoRequest)
	ic6.SetIdent(1)
	ic6.SetSequence(1)
	ic6.SetChecksum(header.ICMPv6Checksum(header.ICMPv6ChecksumParams{Header: ic6, Src: src, Dst: dst}))
	return [][]byte{v4, v6}
}

func TestICMPEchoIsNeverAnsweredOnBehalfOfARemoteDevice(t *testing.T) {
	f := start(t, false)
	a := f.synthetic(t, 0, shared)
	pings := echoRequests(layout.HostV4(f.alloc.Pool()), a.V4, layout.HostAddress(f.alloc.ULA()), a.V6)

	bare, ep, err := newStack()
	if err != nil {
		t.Fatal(err)
	}
	defer bare.Close()
	for i, b := range pings {
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(b)})
		ep.InjectInbound([]tcpip.NetworkProtocolNumber{header.IPv4ProtocolNumber, header.IPv6ProtocolNumber}[i], pkt)
		pkt.DecRef()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		reply := ep.ReadContext(ctx)
		cancel()
		if reply == nil {
			t.Fatalf("control: an unfiltered netstack must answer echo %d, otherwise this test proves nothing", i)
		}
		reply.DecRef()
	}

	for _, b := range pings {
		if _, err := f.raw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.raw.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if n, err := f.raw.Read(make([]byte, 1500)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("the data plane answered an echo it cannot vouch for (%d bytes, %v)", n, err)
	}
}

func TestAdmitOnlyTCPAndUDPToSyntheticRanges(t *testing.T) {
	ula := netip.MustParsePrefix("fd12:3456:789a::/48")
	pool := netip.MustParsePrefix("198.19.240.0/20")
	p := &Plane{ula: ula, pool: pool}
	v4 := func(proto byte, dst string) []byte {
		b := make([]byte, 20)
		b[0], b[9] = 0x45, proto
		copy(b[16:], netip.MustParseAddr(dst).AsSlice())
		return b
	}
	v6 := func(next byte, dst string, frag byte) []byte {
		b := make([]byte, 48)
		b[0], b[6], b[40] = 0x60, next, frag
		copy(b[24:], netip.MustParseAddr(dst).AsSlice())
		return b
	}
	for _, c := range []struct {
		pkt  []byte
		want bool
	}{
		{v4(6, "198.19.240.9"), true},
		{v4(17, "198.19.240.9"), true},
		{v4(1, "198.19.240.9"), false},
		{v4(6, "198.19.0.9"), false},
		{v4(6, "100.64.0.2"), false},
		{v6(6, "fd12:3456:789a:1::1", 0), true},
		{v6(17, "fd12:3456:789a:1::1", 0), true},
		{v6(58, "fd12:3456:789a:1::1", 0), false},
		{v6(ipv6Fragment, "fd12:3456:789a:1::1", 6), true},
		{v6(ipv6Fragment, "fd12:3456:789a:1::1", 58), false},
		{v6(0, "fd12:3456:789a:1::1", 0), false},
		{v6(6, "fd00::1", 0), false},
		{[]byte{0x45}, false},
		{nil, false},
	} {
		if _, got := p.admit(c.pkt); got != c.want {
			t.Fatalf("%x: got %v", c.pkt, got)
		}
	}
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("condition not reached")
}

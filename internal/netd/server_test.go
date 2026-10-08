package netd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	netdv1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/netd/v1"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

func hello() *netdv1.Request {
	return &netdv1.Request{ProtocolMajor: ProtocolMajor, RequestId: 1, Body: &netdv1.Request_Hello{Hello: &netdv1.HelloRequest{}}}
}

func TestIdentityComesFromTheKernel(t *testing.T) {
	a, b, err := pair()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()
	p, err := Identify(a)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.UID != uint32(os.Getuid()) || p.PID != int32(os.Getpid()) {
		t.Fatalf("SO_PEERCRED: %+v", p)
	}
	if p.PIDFD == nil {
		t.Fatal("SO_PEERPIDFD is supported on this kernel and must be used")
	}
	if want, _ := startTime(int32(os.Getpid())); p.StartTime == 0 || p.StartTime != want {
		t.Fatalf("start time %d, want %d", p.StartTime, want)
	}
}

func TestLivenessNeedsNoSignalPermission(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.PidfdOpen(cmd.Process.Pid, 0)
	if err != nil {
		t.Fatal(err)
	}
	pidfd := os.NewFile(uintptr(fd), "pidfd")
	defer pidfd.Close()
	if !alive(pidfd) {
		t.Fatal("a running process reported as exited")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if alive(pidfd) {
		t.Fatal("an exited process reported as alive")
	}
}

func pair() (*net.UnixConn, *net.UnixConn, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	conv := func(fd int) *net.UnixConn {
		f := os.NewFile(uintptr(fd), "pair")
		defer f.Close()
		c, _ := net.FileConn(f)
		return c.(*net.UnixConn)
	}
	return conv(fds[0]), conv(fds[1]), nil
}

func TestParseStartTimeSurvivesHostileProcessNames(t *testing.T) {
	stat := "4242 (evil) 1 2 3 (x) S 1 1 1 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 10"
	if got, err := parseStartTime(stat); err != nil || got != 987654 {
		t.Fatalf("%d %v", got, err)
	}
	if _, err := parseStartTime("garbage"); err == nil {
		t.Fatal("malformed stat accepted")
	}
}

func TestHelloMustComeFirstAndMajorMustMatch(t *testing.T) {
	h := newHarness(t, nil)
	c := h.raw()
	resp := send(t, c, &netdv1.Request{ProtocolMajor: ProtocolMajor, RequestId: 7, Body: &netdv1.Request_Status{Status: &netdv1.StatusRequest{}}})
	if resp.Error.GetCode() != netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR || resp.RequestId != 7 {
		t.Fatalf("%v", resp)
	}
	closed(t, c)

	c = h.raw()
	if resp := send(t, c, &netdv1.Request{ProtocolMajor: 2, Body: &netdv1.Request_Hello{Hello: &netdv1.HelloRequest{}}}); resp.Error.GetCode() != netdv1.ErrorCode_ERROR_CODE_PROTOCOL_INCOMPATIBLE {
		t.Fatalf("%v", resp)
	}
	closed(t, c)

	c = h.raw()
	resp = send(t, c, hello())
	if resp.Error != nil || resp.GetHello().GetProtocolMajor() != ProtocolMajor {
		t.Fatalf("%v", resp)
	}
}

func TestSilentConnectionsAreClosed(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.HelloTimeout = 50 * time.Millisecond })
	closed(t, h.raw())
}

func TestOversizedPacketsAndDescriptorsAreRejected(t *testing.T) {
	h := newHarness(t, nil)
	c := h.raw()
	valid, _ := proto.Marshal(hello())
	pad := maxPacket - len(valid) - 4
	valid = append(valid, 0xa2, 0x06, byte(pad|0x80), byte(pad>>7))
	valid = append(valid, bytes.Repeat([]byte{'x'}, pad)...)
	if len(valid) != maxPacket || proto.Unmarshal(valid, &netdv1.Request{}) != nil {
		t.Fatalf("test packet must be a valid %d-byte request, got %d", maxPacket, len(valid))
	}
	if _, err := c.Write(append(valid, "overflow"...)); err != nil {
		t.Fatal(err)
	}
	if resp := receive(t, c); resp.Error.GetCode() != netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR {
		t.Fatalf("%v", resp)
	}
	closed(t, c)

	c = h.raw()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, _ := proto.Marshal(hello())
	if _, _, err := c.WriteMsgUnix(b, unix.UnixRights(int(w.Fd())), nil); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if resp := receive(t, c); resp.Error.GetCode() != netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR {
		t.Fatalf("descriptors from clients must be refused: %v", resp)
	}
	closed(t, c)
	_ = r.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := r.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("the helper must close a descriptor it was sent (pipe should hit EOF): %v", err)
	}
}

func TestUnauthorizedCallersTouchNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.with(func() { h.auth.deny = true })
	c := h.client()
	_, _, err := c.Create(ula, pool, 1280)
	if code(err) != netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED {
		t.Fatal(err)
	}
	for _, call := range []func() error{c.Destroy, c.ClearDNS, func() error { _, err := c.ConfigureDNS(nil); return err }} {
		if code(call()) != netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED {
			t.Fatal("every mutating operation is authorized")
		}
	}
	var applies int
	var peer Peer
	h.with(func() { applies, peer = h.dns.applies, h.auth.peers[0] })
	if len(h.ops()) != 0 || applies != 0 {
		t.Fatalf("kernel or resolved touched: %v", h.ops())
	}
	if p := peer; p.UID != uint32(os.Getuid()) || p.PID != int32(os.Getpid()) || p.PIDFD == nil {
		t.Fatalf("authorization must use the kernel identity: %+v", p)
	}
}

func TestCallersWithoutALoginSessionAreRefused(t *testing.T) {
	h := newHarness(t, nil)
	h.with(func() { h.sessions.none = true })
	if _, _, err := h.client().Create(ula, pool, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED || len(h.ops()) != 0 {
		t.Fatal(err)
	}
}

func TestInactiveOrRemoteSessionsCannotCreate(t *testing.T) {
	h := newHarness(t, nil)
	h.with(func() { h.sessions.inactive = true })
	if _, _, err := h.client().Create(ula, pool, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED || len(h.ops()) != 0 {
		t.Fatalf("polkit may authorize a session-less process while the user has any active session; the helper must still refuse: %v", err)
	}
}

func TestCreateValidatesRangesBeforeTouchingTheKernel(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	bad6 := netdv1.ErrorCode_ERROR_CODE_INVALID_V6_PREFIX
	bad4 := netdv1.ErrorCode_ERROR_CODE_INVALID_V4_PREFIX
	badMTU := netdv1.ErrorCode_ERROR_CODE_INVALID_MTU
	for _, tc := range []struct {
		v6, v4 string
		mtu    uint32
		want   netdv1.ErrorCode
	}{
		{"fd12:3456:789a::/56", "", 1280, bad6},
		{"fc12:3456:789a::/48", "", 1280, bad6},
		{"2001:db8:1::/48", "", 1280, bad6},
		{"10.0.0.0/8", "", 1280, bad6},
		{"fd12:3456:789a::/48", "10.0.0.0/20", 1280, bad4},
		{"fd12:3456:789a::/48", "198.19.255.252/30", 1280, bad4},
		{"fd12:3456:789a::/48", "198.16.0.0/14", 1280, bad4},
		{"fd12:3456:789a::/48", "fd00::/48", 1280, bad4},
		{"fd12:3456:789a::/48", "", 1279, badMTU},
		{"fd12:3456:789a::/48", "", 9001, badMTU},
	} {
		var v4 netip.Prefix
		if tc.v4 != "" {
			v4 = netip.MustParsePrefix(tc.v4)
		}
		_, _, err := c.Create(netip.MustParsePrefix(tc.v6), v4, tc.mtu)
		if code(err) != tc.want {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	unmasked := &netdv1.CreateSyntheticInterfaceRequest{V6InstallPrefix: &netdv1.IpPrefix{Address: netip.MustParseAddr("fd12:3456:789a::1").AsSlice(), Bits: 48}, Mtu: 1280}
	if _, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_CreateSyntheticInterface{CreateSyntheticInterface: unmasked}}, true); code(err) != bad6 {
		t.Fatalf("host bits must be zero: %v", err)
	}
	if hasOp(h.kernel, "tun") {
		t.Fatal("invalid input reached the kernel")
	}
}

func TestOverlapsAndUnanalysableStateAreRefused(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	h.with(func() {
		h.kernel.occupied = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0"), netip.MustParsePrefix("fd12:3456:789a:5::/64")}
	})
	if _, _, err := c.Create(ula, pool, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_RANGE_OVERLAP {
		t.Fatal(err)
	}
	h.with(func() { h.kernel.occupied = []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")} })
	if _, _, err := c.Create(ula, pool, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_RANGE_OVERLAP {
		t.Fatal(err)
	}
	h.with(func() { h.kernel.occupied, h.kernel.occupiedErr = nil, errors.New("netlink dump interrupted") })
	if _, _, err := c.Create(ula, pool, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_RANGE_OVERLAP {
		t.Fatal(err)
	}
	if hasOp(h.kernel, "tun") {
		t.Fatal("never shadow: nothing may be created after a failed analysis")
	}
	h.with(func() { h.kernel.occupiedErr = nil })
	h.with(func() {
		h.kernel.occupied = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("fe80::/64")}
	})
	if _, f, err := c.Create(ula, pool, 1280); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
}

func TestCreateTransfersExactlyOneDescriptorAndKeepsNoCopy(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	resp, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	name := LinkName(uint32(os.Getuid()))
	if resp.InterfaceName != name || resp.InterfaceIndex == 0 {
		t.Fatalf("%v", resp)
	}
	for got, want := range map[string]string{
		ip(resp.V6HostAddress):     "fd12:3456:789a::1",
		ip(resp.V6ResolverAddress): "fd12:3456:789a::53",
		ip(resp.V4HostAddress):     "198.19.240.1",
		ip(resp.V4ResolverAddress): "198.19.240.2",
	} {
		if got != want {
			t.Fatalf("got %s want %s", got, want)
		}
	}
	l := h.kernel.link(name)
	if !l.up || l.mtu != 1280 {
		t.Fatalf("%+v", l)
	}
	if !slices.Equal(l.addrs, []netip.Addr{netip.MustParseAddr("fd12:3456:789a::1"), netip.MustParseAddr("198.19.240.1")}) {
		t.Fatalf("only host-side addresses go on the link, never the resolvers: %v", l.addrs)
	}
	if !slices.Equal(l.routes, []netip.Prefix{ula, pool}) {
		t.Fatalf("routes: %v", l.routes)
	}
	if !errors.Is(l.tun.Close(), os.ErrClosed) {
		t.Fatal("the helper must close its copy of the TUN descriptor after sending it")
	}
	if _, err := l.kernel.Write([]byte("packet")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	_ = tun.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := tun.Read(buf); err != nil || string(buf[:n]) != "packet" {
		t.Fatalf("received descriptor is not the created device: %q %v", buf[:n], err)
	}
}

func ip(b []byte) string {
	a, _ := netip.AddrFromSlice(b)
	return a.String()
}

func TestCreateWithoutIPv4(t *testing.T) {
	h := newHarness(t, nil)
	resp, tun, err := h.client().Create(ula, netip.Prefix{}, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	if resp.V4HostAddress != nil || resp.V4ResolverAddress != nil || hasOp(h.kernel, "addr4") || hasOp(h.kernel, "route4") {
		t.Fatalf("%v %v", resp, h.ops())
	}
}

func TestEveryFailedStepRollsBackCompletely(t *testing.T) {
	for _, step := range []string{"tun", "up", "addr6", "route6", "addr4", "route4"} {
		t.Run(step, func(t *testing.T) {
			h := newHarness(t, nil)
			h.with(func() { h.kernel.failAt = step })
			c := h.client()
			_, _, err := c.Create(ula, pool, 1280)
			want := netdv1.ErrorCode_ERROR_CODE_NETLINK_FAILED
			if step == "tun" {
				want = netdv1.ErrorCode_ERROR_CODE_TUN_UNAVAILABLE
			}
			if code(err) != want {
				t.Fatal(err)
			}
			if h.linkCount() != 0 {
				t.Fatalf("leftover links after rollback: %v", h.ops())
			}
			h.with(func() { h.kernel.failAt = "" })
			_, tun, err := c.Create(ula, pool, 1280)
			if err != nil {
				t.Fatalf("slot must be free after a rollback: %v", err)
			}
			tun.Close()
		})
	}
}

func TestOneOwnerPerHostAndPrivateStatus(t *testing.T) {
	h := newHarness(t, nil)
	owner, other, same := h.client(1000), h.client(1001), h.client(1000)
	_, tun, err := owner.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	for _, c := range []*Client{other, same} {
		if _, _, err := c.Create(netip.MustParsePrefix("fd99::/48"), netip.Prefix{}, 1280); code(err) != netdv1.ErrorCode_ERROR_CODE_HOST_INSTANCE_IN_USE {
			t.Fatal(err)
		}
		if err := c.Destroy(); code(err) != netdv1.ErrorCode_ERROR_CODE_INSTANCE_NOT_FOUND {
			t.Fatal("only the owning connection may destroy")
		}
		if _, err := c.ConfigureDNS(nil); code(err) != netdv1.ErrorCode_ERROR_CODE_INSTANCE_NOT_FOUND {
			t.Fatal("only the owning connection may configure dns")
		}
	}
	st, err := other.Status()
	if err != nil || !st.OccupiedByOtherUser || st.Instance != nil {
		t.Fatalf("another user learns only that the host is occupied: %v %v", st, err)
	}
	other.Close()
	h.uids <- 1001
	fresh, err := Dial(context.Background(), h.path)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if !fresh.Hello.OccupiedByOtherUser || fresh.Hello.CallerOwnsInstance {
		t.Fatalf("%v", fresh.Hello)
	}
	if st, _ := owner.Status(); st.Instance.GetInterfaceName() != LinkName(1000) {
		t.Fatalf("%v", st)
	}
}

func TestOwnerDisconnectRemovesEverything(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	resp, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	if _, err := c.ConfigureDNS(nil); err != nil {
		t.Fatal(err)
	}
	c.Close()
	eventually(t, "teardown", func() bool {
		h.kernel.mu.Lock()
		defer h.kernel.mu.Unlock()
		return len(h.kernel.links) == 0
	})
	h.dns.mu.Lock()
	reverted := slices.Contains(h.dns.reverts, resp.InterfaceIndex)
	h.dns.mu.Unlock()
	if !reverted {
		t.Fatal("resolved configuration must be reverted when the owner goes away")
	}
	if _, tun, err := h.client().Create(ula, pool, 1280); err != nil {
		t.Fatal(err)
	} else {
		tun.Close()
	}
}

func TestDestroyIsExplicitTeardown(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	_, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	if err := c.Destroy(); err != nil {
		t.Fatal(err)
	}
	if h.linkCount() != 0 {
		t.Fatal("destroy must delete the link")
	}
	if err := c.Destroy(); code(err) != netdv1.ErrorCode_ERROR_CODE_INSTANCE_NOT_FOUND {
		t.Fatal(err)
	}
}

func TestConfigureDNSUsesOnlyTheExpectedResolvers(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	resp, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	got, err := c.ConfigureDNS([]string{"Tail1234.TS.net.", "tail1234.ts.net", "home.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lattice.internal", "tail1234.ts.net", "home.example.com"}; !slices.Equal(got, want) || !slices.Equal(h.applied(resp.InterfaceIndex), want) {
		t.Fatalf("%v", got)
	}
	var servers []netip.Addr
	h.with(func() { servers = h.dns.servers })
	if want := []netip.Addr{netip.MustParseAddr("fd12:3456:789a::53"), netip.MustParseAddr("198.19.240.2")}; !slices.Equal(servers, want) {
		t.Fatalf("servers %v", servers)
	}
	for _, bad := range [][]string{{"."}, {"~."}, {""}, {"local"}, {"printer.local"}, {"1.168.192.in-addr.arpa"}, {"localhost"}, {"com"}, {"*.example.com"}, {"~example.com"}, {"exa mple.com"}, {"-x.example.com"}, {strings.Repeat("a", 64) + ".com"}} {
		if _, err := c.ConfigureDNS(bad); code(err) != netdv1.ErrorCode_ERROR_CODE_INVALID_DOMAIN && code(err) != netdv1.ErrorCode_ERROR_CODE_DNS_CAPTURE_NOT_ALLOWED {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
	many := make([]string, 32)
	for i := range many {
		many[i] = "n" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".example.com"
	}
	if _, err := c.ConfigureDNS(many); code(err) != netdv1.ErrorCode_ERROR_CODE_INVALID_DOMAIN {
		t.Fatalf("more than 32 domains in total accepted: %v", err)
	}
	if !slices.Equal(h.applied(resp.InterfaceIndex), []string{"lattice.internal", "tail1234.ts.net", "home.example.com"}) {
		t.Fatal("a rejected request must not change the applied configuration")
	}
	if err := c.ClearDNS(); err != nil || h.applied(resp.InterfaceIndex) != nil {
		t.Fatalf("clear: %v", err)
	}
	h.with(func() { h.dns.fail = true })
	if _, err := c.ConfigureDNS(nil); code(err) != netdv1.ErrorCode_ERROR_CODE_RESOLVED_UNAVAILABLE {
		t.Fatal(err)
	}
}

func TestResolvedRestartIsReapplied(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	_, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	if _, err := c.ConfigureDNS(nil); err != nil {
		t.Fatal(err)
	}
	h.dns.mu.Lock()
	h.dns.gen = ":1.99"
	h.dns.mu.Unlock()
	eventually(t, "reapply", func() bool {
		h.dns.mu.Lock()
		defer h.dns.mu.Unlock()
		return h.dns.applies == 2
	})
}

func TestInactiveSessionRemovesTheInstance(t *testing.T) {
	h := newHarness(t, nil)
	c := h.client()
	_, tun, err := c.Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	h.sessions.mu.Lock()
	h.sessions.inactive = true
	h.sessions.mu.Unlock()
	eventually(t, "teardown on session change", func() bool {
		h.kernel.mu.Lock()
		defer h.kernel.mu.Unlock()
		return len(h.kernel.links) == 0
	})
	if _, err := c.Status(); err == nil {
		t.Fatal("the owner connection must be closed when its session goes inactive")
	}
}

func TestConnectionAndRateLimits(t *testing.T) {
	h := newHarness(t, nil)
	h.client(1000)
	h.client(1000)
	h.uids <- 1000
	if _, err := Dial(context.Background(), h.path); code(err) != netdv1.ErrorCode_ERROR_CODE_RATE_LIMITED {
		t.Fatalf("third connection for one uid: %v", err)
	}
	for uid := uint32(2000); uid < 2006; uid++ {
		h.client(uid)
	}
	h.uids <- 3000
	if _, err := Dial(context.Background(), h.path); code(err) != netdv1.ErrorCode_ERROR_CODE_RATE_LIMITED {
		t.Fatalf("ninth connection in total: %v", err)
	}

	r := newHarness(t, nil)
	c := r.client()
	limited := false
	for range requestBurst + 10 {
		if _, err := c.Status(); code(err) == netdv1.ErrorCode_ERROR_CODE_RATE_LIMITED {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("per-uid request rate is not limited")
	}
}

func TestStartupRemovesOrphanedLinks(t *testing.T) {
	h := newHarness(t, func(c *Config) {
		k := c.Kernel.(*fakeKernel)
		f, _, err := k.CreateTUN("lat-u4242")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		k.ops = nil
	})
	eventually(t, "reconcile", func() bool {
		h.kernel.mu.Lock()
		defer h.kernel.mu.Unlock()
		return slices.Contains(h.kernel.deleted, "lat-u4242")
	})
}

func TestIdleExit(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.IdleExit = 100 * time.Millisecond })
	select {
	case <-h.done:
		if h.err != nil {
			t.Fatal(h.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("idle helper did not exit")
	}
}

func TestNoIdleExitWhileAnInstanceExists(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.IdleExit = 50 * time.Millisecond })
	_, tun, err := h.client().Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	select {
	case <-h.done:
		t.Fatal("exited with a live owner")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestShutdownRemovesTheInstance(t *testing.T) {
	h := newHarness(t, nil)
	_, tun, err := h.client().Create(ula, pool, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	h.stop()
	if h.linkCount() != 0 {
		t.Fatal("stopping the helper must remove the instance")
	}
}

func TestPolkitSubjects(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := Peer{UID: 1000, PID: 4242, PIDFD: f, StartTime: 987654}
	s := pidfdSubject(p)
	if s.Kind != "unix-process" || s.Details["pidfd"].Signature().String() != "h" || s.Details["uid"].Value() != int32(1000) || len(s.Details) != 2 {
		t.Fatalf("%+v", s)
	}
	s = processSubject(p)
	if s.Details["pid"].Value() != uint32(4242) || s.Details["start-time"].Value() != uint64(987654) || s.Details["uid"].Value() != int32(1000) || len(s.Details) != 3 {
		t.Fatalf("%+v", s)
	}
}

func TestVerifyTUNRejectsOtherDescriptors(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if VerifyTUN(r, "lat-u1000") == nil {
		t.Fatal("a pipe passed as a TUN")
	}
}

func TestNetlinkAnalysisIsReadOnlyAndComplete(t *testing.T) {
	occupied, err := Netlink{}.Occupied()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(occupied, func(p netip.Prefix) bool { return p.Contains(netip.MustParseAddr("127.0.0.1")) }) {
		t.Fatalf("loopback missing from the analysis: %v", occupied)
	}
	if _, err := (Netlink{}).Links(); err != nil {
		t.Fatal(err)
	}
}

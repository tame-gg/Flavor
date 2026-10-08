package netd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"google.golang.org/protobuf/proto"
)

type fakeLink struct {
	name   string
	mtu    uint32
	up     bool
	addrs  []netip.Addr
	routes []netip.Prefix
	tun    *os.File
	kernel *os.File
}

type fakeKernel struct {
	mu          sync.Mutex
	next        uint32
	links       map[uint32]*fakeLink
	occupied    []netip.Prefix
	occupiedErr error
	failAt      string
	ops         []string
	deleted     []string
}

func newFakeKernel() *fakeKernel { return &fakeKernel{next: 10, links: map[uint32]*fakeLink{}} }

func (k *fakeKernel) step(op string) error {
	k.ops = append(k.ops, op)
	if op == k.failAt {
		return errors.New("injected failure at " + op)
	}
	return nil
}

func (k *fakeKernel) Occupied() ([]netip.Prefix, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.ops = append(k.ops, "occupied")
	return k.occupied, k.occupiedErr
}

func (k *fakeKernel) CreateTUN(name string) (*os.File, uint32, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.step("tun"); err != nil {
		return nil, 0, err
	}
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, 0, err
	}
	k.next++
	l := &fakeLink{name: name, tun: os.NewFile(uintptr(fds[0]), "tun"), kernel: os.NewFile(uintptr(fds[1]), "kernel")}
	k.links[k.next] = l
	return l.tun, k.next, nil
}

func (k *fakeKernel) Up(index, mtu uint32) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.step("up"); err != nil {
		return err
	}
	k.links[index].up, k.links[index].mtu = true, mtu
	return nil
}

func (k *fakeKernel) AddAddress(index uint32, a netip.Addr) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	op := "addr6"
	if a.Is4() {
		op = "addr4"
	}
	if err := k.step(op); err != nil {
		return err
	}
	k.links[index].addrs = append(k.links[index].addrs, a)
	return nil
}

func (k *fakeKernel) AddRoute(index uint32, p netip.Prefix) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	op := "route6"
	if p.Addr().Is4() {
		op = "route4"
	}
	if err := k.step(op); err != nil {
		return err
	}
	k.links[index].routes = append(k.links[index].routes, p)
	return nil
}

func (k *fakeKernel) DeleteLink(index uint32) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if l, ok := k.links[index]; ok {
		k.deleted = append(k.deleted, l.name)
		l.kernel.Close()
		delete(k.links, index)
	}
	return nil
}

func (k *fakeKernel) Links() ([]Link, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []Link
	for i, l := range k.links {
		out = append(out, Link{Name: l.name, Index: i})
	}
	return out, nil
}

func (k *fakeKernel) link(name string) *fakeLink {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, l := range k.links {
		if l.name == name {
			return l
		}
	}
	return nil
}

type fakeDNS struct {
	mu      sync.Mutex
	applied map[uint32][]string
	servers []netip.Addr
	applies int
	reverts []uint32
	gen     string
	fail    bool
}

func (d *fakeDNS) Apply(_ context.Context, index uint32, servers []netip.Addr, domains []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return errors.New("resolved down")
	}
	d.applies++
	d.applied[index], d.servers = domains, servers
	return nil
}

func (d *fakeDNS) Revert(_ context.Context, index uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.applied, index)
	d.reverts = append(d.reverts, index)
	return nil
}

func (d *fakeDNS) Generation(context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gen, nil
}

type fakeAuth struct {
	mu    sync.Mutex
	deny  bool
	peers []Peer
}

func (a *fakeAuth) Authorize(_ context.Context, p Peer) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.peers = append(a.peers, p)
	return !a.deny, nil
}

type fakeSessions struct {
	mu       sync.Mutex
	inactive bool
	none     bool
}

func (s *fakeSessions) Session(context.Context, Peer) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.none {
		return "", errors.New("no session")
	}
	return "/org/freedesktop/login1/session/_31", nil
}

func (s *fakeSessions) Active(context.Context, string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.inactive, nil
}

type harness struct {
	t        *testing.T
	path     string
	kernel   *fakeKernel
	dns      *fakeDNS
	auth     *fakeAuth
	sessions *fakeSessions
	srv      *Server
	cancel   context.CancelFunc
	done     chan struct{}
	err      error
	uids     chan uint32
}

func newHarness(t *testing.T, tweak func(*Config)) *harness {
	t.Helper()
	h := &harness{
		t:        t,
		path:     filepath.Join(t.TempDir(), "netd.sock"),
		kernel:   newFakeKernel(),
		dns:      &fakeDNS{applied: map[uint32][]string{}, gen: ":1.1"},
		auth:     &fakeAuth{},
		sessions: &fakeSessions{},
		uids:     make(chan uint32, 16),
		done:     make(chan struct{}),
	}
	cfg := Config{
		Kernel: h.kernel, DNS: h.dns, Authorizer: h.auth, Sessions: h.sessions,
		Log:  slog.New(slog.DiscardHandler),
		Poll: 20 * time.Millisecond,
		Identify: func(c *net.UnixConn) (Peer, error) {
			p, err := Identify(c)
			select {
			case uid := <-h.uids:
				p.UID = uid
			default:
			}
			return p, err
		},
	}
	if tweak != nil {
		tweak(&cfg)
	}
	h.srv = NewServer(cfg)
	ln, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: h.path, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		h.err = h.srv.Serve(ctx, ln)
		close(h.done)
	}()
	t.Cleanup(h.stop)
	return h
}

func (h *harness) with(f func()) {
	h.kernel.mu.Lock()
	h.dns.mu.Lock()
	h.auth.mu.Lock()
	h.sessions.mu.Lock()
	defer h.kernel.mu.Unlock()
	defer h.dns.mu.Unlock()
	defer h.auth.mu.Unlock()
	defer h.sessions.mu.Unlock()
	f()
}

func (h *harness) stop() {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.t.Error("server did not stop")
	}
}

func (h *harness) client(uid ...uint32) *Client {
	h.t.Helper()
	if len(uid) > 0 {
		h.uids <- uid[0]
	}
	c, err := Dial(context.Background(), h.path)
	if err != nil {
		h.t.Fatal(err)
	}
	c.verify = func(*os.File, string) error { return nil }
	h.t.Cleanup(func() { c.Close() })
	return c
}

func (h *harness) raw() *net.UnixConn {
	h.t.Helper()
	c, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: h.path, Net: "unixpacket"})
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.Close() })
	return c
}

func send(t *testing.T, c *net.UnixConn, req *netdv1.Request) *netdv1.Response {
	t.Helper()
	if err := writePacket(c, req, nil); err != nil {
		t.Fatal(err)
	}
	return receive(t, c)
}

func receive(t *testing.T, c *net.UnixConn) *netdv1.Response {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, files, err := readPacket(c)
	closeAll(files)
	if err != nil {
		t.Fatal(err)
	}
	var resp netdv1.Response
	if err := proto.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	return &resp
}

func closed(t *testing.T, c *net.UnixConn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := readPacket(c); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("connection must be closed by the helper: %v", err)
	}
}

func code(err error) netdv1.ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return netdv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("timed out waiting for " + what)
}

var (
	ula  = netip.MustParsePrefix("fd12:3456:789a::/48")
	pool = netip.MustParsePrefix("198.19.240.0/20")
)

func hasOp(k *fakeKernel, op string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Contains(k.ops, op)
}

func (h *harness) linkCount() (n int) {
	h.with(func() { n = len(h.kernel.links) })
	return n
}

func (h *harness) ops() (out []string) {
	h.with(func() { out = slices.Clone(h.kernel.ops) })
	return out
}

func (h *harness) applied(index uint32) (out []string) {
	h.with(func() { out = h.dns.applied[index] })
	return out
}

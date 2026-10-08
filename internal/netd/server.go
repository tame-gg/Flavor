package netd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
)

const (
	MaxConnections       = 8
	MaxConnectionsPerUID = 2
	HelloTimeout         = 10 * time.Second
	DefaultPollInterval  = 2 * time.Second
	DefaultIdleExit      = 60 * time.Second
	requestsPerSecond    = 20
	requestBurst         = 40
	backendCallTimeout   = 10 * time.Second
)

type Kernel interface {
	Occupied() ([]netip.Prefix, error)
	CreateTUN(name string) (*os.File, uint32, error)
	Up(index, mtu uint32) error
	AddAddress(index uint32, a netip.Addr) error
	AddRoute(index uint32, p netip.Prefix) error
	DeleteLink(index uint32) error
	Links() ([]Link, error)
}

type DNS interface {
	Apply(ctx context.Context, index uint32, servers []netip.Addr, domains []string) error
	Revert(ctx context.Context, index uint32) error
	Generation(ctx context.Context) (string, error)
}

type Authorizer interface {
	Authorize(ctx context.Context, p Peer) (bool, error)
}

type Sessions interface {
	Session(ctx context.Context, p Peer) (string, error)
	Active(ctx context.Context, session string) (bool, error)
}

type Config struct {
	Kernel       Kernel
	DNS          DNS
	Authorizer   Authorizer
	Sessions     Sessions
	Log          *slog.Logger
	Identify     func(*net.UnixConn) (Peer, error)
	HelloTimeout time.Duration
	Poll         time.Duration
	IdleExit     time.Duration
}

type Server struct {
	cfg Config

	mu       sync.Mutex
	conns    map[*conn]struct{}
	perUID   map[uint32]int
	limiters map[uint32]*rate.Limiter
	owner    *instance
	idleFrom time.Time
}

type conn struct {
	c    *net.UnixConn
	peer Peer
}

type instance struct {
	conn       *conn
	uid        uint32
	session    string
	name       string
	index      uint32
	ranges     ranges
	domains    []string
	generation string
}

func NewServer(cfg Config) *Server {
	if cfg.Identify == nil {
		cfg.Identify = Identify
	}
	if cfg.HelloTimeout == 0 {
		cfg.HelloTimeout = HelloTimeout
	}
	if cfg.Poll == 0 {
		cfg.Poll = DefaultPollInterval
	}
	return &Server{cfg: cfg, conns: map[*conn]struct{}{}, perUID: map[uint32]int{}, limiters: map[uint32]*rate.Limiter{}, idleFrom: time.Now()}
}

func (s *Server) Serve(ctx context.Context, ln *net.UnixListener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.reconcile(ctx)
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go s.watch(ctx, cancel)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		c, err := ln.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				s.shutdown()
				return nil
			}
			return err
		}
		peer, err := s.cfg.Identify(c)
		if err != nil {
			s.cfg.Log.Warn("refusing unidentified peer", "err", err.Error())
			c.Close()
			continue
		}
		cn := &conn{c: c, peer: peer}
		if !s.admit(cn) {
			go refuse(cn)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handle(ctx, cn)
		}()
	}
}

func refuse(cn *conn) {
	defer cn.peer.Close()
	defer cn.c.Close()
	_ = cn.c.SetReadDeadline(time.Now().Add(time.Second))
	var req netdv1.Request
	if b, files, err := readPacket(cn.c); err == nil {
		closeAll(files)
		_ = proto.Unmarshal(b, &req)
	}
	_ = writePacket(cn.c, failure(req.RequestId, netdv1.ErrorCode_ERROR_CODE_RATE_LIMITED, "too many connections"), nil)
}

func (s *Server) admit(cn *conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid := cn.peer.UID
	if len(s.conns) >= MaxConnections || s.perUID[uid] >= MaxConnectionsPerUID {
		return false
	}
	s.conns[cn] = struct{}{}
	s.perUID[uid]++
	return true
}

func (s *Server) release(cn *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, cn)
	if s.perUID[cn.peer.UID]--; s.perUID[cn.peer.UID] <= 0 {
		delete(s.perUID, cn.peer.UID)
	}
	if s.owner != nil && s.owner.conn == cn {
		s.teardownLocked()
	}
	if len(s.conns) == 0 && s.owner == nil {
		s.idleFrom = time.Now()
	}
}

func (s *Server) limiter(uid uint32) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.limiters[uid]
	if !ok {
		l = rate.NewLimiter(requestsPerSecond, requestBurst)
		s.limiters[uid] = l
	}
	return l
}

func failure(id uint64, code netdv1.ErrorCode, detail string) *netdv1.Response {
	return &netdv1.Response{RequestId: id, Error: &netdv1.Error{Code: code, Detail: detail}}
}

func (s *Server) handle(ctx context.Context, cn *conn) {
	defer cn.peer.Close()
	defer cn.c.Close()
	defer s.release(cn)
	limiter := s.limiter(cn.peer.UID)
	hello := false
	for {
		deadline := time.Time{}
		if !hello {
			deadline = time.Now().Add(s.cfg.HelloTimeout)
		}
		_ = cn.c.SetReadDeadline(deadline)
		b, files, err := readPacket(cn.c)
		if errors.Is(err, errProtocol) || len(files) > 0 {
			closeAll(files)
			_ = writePacket(cn.c, failure(0, netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR, "oversized packet or ancillary data"), nil)
			return
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				s.cfg.Log.Debug("connection ended", "err", err.Error())
			}
			return
		}
		var req netdv1.Request
		if err := proto.Unmarshal(b, &req); err != nil {
			_ = writePacket(cn.c, failure(0, netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR, "malformed request"), nil)
			return
		}
		if req.ProtocolMajor != ProtocolMajor {
			_ = writePacket(cn.c, failure(req.RequestId, netdv1.ErrorCode_ERROR_CODE_PROTOCOL_INCOMPATIBLE, "protocol major "+strconv.Itoa(ProtocolMajor)+" required"), nil)
			return
		}
		if !hello && req.GetHello() == nil {
			_ = writePacket(cn.c, failure(req.RequestId, netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR, "hello must come first"), nil)
			return
		}
		if !limiter.Allow() {
			if writePacket(cn.c, failure(req.RequestId, netdv1.ErrorCode_ERROR_CODE_RATE_LIMITED, "slow down"), nil) != nil {
				return
			}
			continue
		}
		resp, tun := s.dispatch(ctx, cn, &req)
		resp.RequestId = req.RequestId
		err = writePacket(cn.c, resp, tun)
		if tun != nil {
			tun.Close()
			if err != nil {
				s.cfg.Log.Warn("tun descriptor transfer failed; rolling back", "err", err.Error())
				s.mu.Lock()
				if s.owner != nil && s.owner.conn == cn {
					s.teardownLocked()
				}
				s.mu.Unlock()
			}
		}
		if err != nil {
			return
		}
		hello = true
	}
}

func (s *Server) dispatch(ctx context.Context, cn *conn, req *netdv1.Request) (*netdv1.Response, *os.File) {
	switch body := req.Body.(type) {
	case *netdv1.Request_Hello:
		return s.hello(cn), nil
	case *netdv1.Request_Status:
		return s.status(cn), nil
	case *netdv1.Request_CreateSyntheticInterface:
		if r := s.authorize(ctx, cn); r != nil {
			return r, nil
		}
		return s.create(ctx, cn, body.CreateSyntheticInterface)
	case *netdv1.Request_DestroySyntheticInterface:
		if r := s.authorize(ctx, cn); r != nil {
			return r, nil
		}
		return s.destroy(cn), nil
	case *netdv1.Request_ConfigureDns:
		if r := s.authorize(ctx, cn); r != nil {
			return r, nil
		}
		return s.configureDNS(ctx, cn, body.ConfigureDns), nil
	case *netdv1.Request_ClearDns:
		if r := s.authorize(ctx, cn); r != nil {
			return r, nil
		}
		return s.clearDNS(ctx, cn), nil
	}
	return failure(0, netdv1.ErrorCode_ERROR_CODE_PROTOCOL_ERROR, "unknown request"), nil
}

func (s *Server) authorize(ctx context.Context, cn *conn) *netdv1.Response {
	ctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
	defer cancel()
	ok, err := s.cfg.Authorizer.Authorize(ctx, cn.peer)
	if err != nil {
		s.cfg.Log.Warn("authorization check failed", "uid", cn.peer.UID, "err", err.Error())
	}
	if !ok {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED, "not authorized for "+ManageAction)
	}
	return nil
}

func (s *Server) hello(cn *conn) *netdv1.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &netdv1.Response{Body: &netdv1.Response_Hello{Hello: &netdv1.HelloResponse{
		ProtocolMajor:       ProtocolMajor,
		ProtocolMinor:       ProtocolMinor,
		Capabilities:        []netdv1.Capability{netdv1.Capability_CAPABILITY_IPV6, netdv1.Capability_CAPABILITY_IPV4, netdv1.Capability_CAPABILITY_RESOLVED_DNS},
		OccupiedByOtherUser: s.owner != nil && s.owner.uid != cn.peer.UID,
		CallerOwnsInstance:  s.owner != nil && s.owner.conn == cn,
	}}}
}

func (s *Server) status(cn *conn) *netdv1.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &netdv1.StatusResponse{}
	switch {
	case s.owner == nil:
	case s.owner.uid != cn.peer.UID:
		st.OccupiedByOtherUser = true
	default:
		o := s.owner
		st.Instance = &netdv1.Instance{
			InterfaceName:   o.name,
			InterfaceIndex:  o.index,
			V6InstallPrefix: wirePrefix(o.ranges.v6),
			V4Pool:          wirePrefix(o.ranges.v4),
			Mtu:             o.ranges.mtu,
			DnsConfigured:   o.domains != nil,
			RoutingDomains:  o.domains,
		}
	}
	return &netdv1.Response{Body: &netdv1.Response_Status{Status: st}}
}

func wirePrefix(p netip.Prefix) *netdv1.IpPrefix {
	if !p.IsValid() {
		return nil
	}
	return &netdv1.IpPrefix{Address: p.Addr().AsSlice(), Bits: uint32(p.Bits())}
}

func wireAddr(a netip.Addr) []byte {
	if !a.IsValid() {
		return nil
	}
	return a.AsSlice()
}

func LinkName(uid uint32) string { return LinkPrefix + strconv.FormatUint(uint64(uid), 10) }

func (s *Server) create(ctx context.Context, cn *conn, req *netdv1.CreateSyntheticInterfaceRequest) (*netdv1.Response, *os.File) {
	sctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
	session, err := s.cfg.Sessions.Session(sctx, cn.peer)
	active := false
	if err == nil {
		active, err = s.cfg.Sessions.Active(sctx, session)
	}
	cancel()
	if err != nil || !active {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_UNAUTHORIZED, "caller has no active local login session"), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner != nil {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_HOST_INSTANCE_IN_USE, "synthetic networking is already active on this host"), nil
	}
	r, code := validateCreate(req)
	if code != netdv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return failure(0, code, "invalid range or mtu"), nil
	}
	occupied, err := s.cfg.Kernel.Occupied()
	if err != nil {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_RANGE_OVERLAP, "could not analyse kernel addresses, routes and rules"), nil
	}
	if o, clash := overlapping(r, occupied); clash {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_RANGE_OVERLAP, "requested range overlaps "+o.String()), nil
	}
	name := LinkName(cn.peer.UID)
	tun, index, err := s.cfg.Kernel.CreateTUN(name)
	if err != nil {
		s.cfg.Log.Warn("creating tun failed", "link", name, "err", err.Error())
		return failure(0, netdv1.ErrorCode_ERROR_CODE_TUN_UNAVAILABLE, "could not create "+name), nil
	}
	steps := []func() error{
		func() error { return s.cfg.Kernel.Up(index, r.mtu) },
		func() error { return s.cfg.Kernel.AddAddress(index, r.hostV6()) },
		func() error { return s.cfg.Kernel.AddRoute(index, r.v6) },
	}
	if r.v4.IsValid() {
		steps = append(steps,
			func() error { return s.cfg.Kernel.AddAddress(index, layout.HostV4(r.v4)) },
			func() error { return s.cfg.Kernel.AddRoute(index, r.v4) },
		)
	}
	for _, step := range steps {
		if err := step(); err != nil {
			tun.Close()
			if derr := s.cfg.Kernel.DeleteLink(index); derr != nil {
				s.cfg.Log.Error("rollback could not delete link", "link", name, "err", derr.Error())
			}
			s.cfg.Log.Warn("create failed; rolled back", "link", name, "err", err.Error())
			return failure(0, netdv1.ErrorCode_ERROR_CODE_NETLINK_FAILED, "could not configure "+name), nil
		}
	}
	s.owner = &instance{conn: cn, uid: cn.peer.UID, session: session, name: name, index: index, ranges: r}
	s.cfg.Log.Info("synthetic interface created", "link", name, "uid", cn.peer.UID)
	out := &netdv1.CreateSyntheticInterfaceResponse{
		InterfaceName:     name,
		InterfaceIndex:    index,
		V6HostAddress:     wireAddr(r.hostV6()),
		V6ResolverAddress: wireAddr(r.resolverV6()),
	}
	if r.v4.IsValid() {
		out.V4HostAddress, out.V4ResolverAddress = wireAddr(layout.HostV4(r.v4)), wireAddr(layout.ResolverV4(r.v4))
	}
	return &netdv1.Response{Body: &netdv1.Response_CreateSyntheticInterface{CreateSyntheticInterface: out}}, tun
}

func (s *Server) owned(cn *conn) (*instance, *netdv1.Response) {
	if s.owner == nil || s.owner.conn != cn {
		return nil, failure(0, netdv1.ErrorCode_ERROR_CODE_INSTANCE_NOT_FOUND, "this connection owns no synthetic interface")
	}
	return s.owner, nil
}

func (s *Server) destroy(cn *conn) *netdv1.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, r := s.owned(cn); r != nil {
		return r
	}
	s.teardownLocked()
	return &netdv1.Response{Body: &netdv1.Response_DestroySyntheticInterface{DestroySyntheticInterface: &netdv1.DestroySyntheticInterfaceResponse{}}}
}

func (s *Server) configureDNS(ctx context.Context, cn *conn, req *netdv1.ConfigureDnsRequest) *netdv1.Response {
	domains, code := validateDomains(req.GetRoutingDomains())
	if code != netdv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return failure(0, code, "domain rejected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o, r := s.owned(cn)
	if r != nil {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
	defer cancel()
	gen, _ := s.cfg.DNS.Generation(ctx)
	if err := s.cfg.DNS.Apply(ctx, o.index, o.ranges.resolvers(), domains); err != nil {
		s.cfg.Log.Warn("configuring resolved failed", "err", err.Error())
		o.domains = nil
		return failure(0, netdv1.ErrorCode_ERROR_CODE_RESOLVED_UNAVAILABLE, "systemd-resolved rejected or is unavailable")
	}
	o.domains, o.generation = domains, gen
	return &netdv1.Response{Body: &netdv1.Response_ConfigureDns{ConfigureDns: &netdv1.ConfigureDnsResponse{RoutingDomains: domains}}}
}

func (s *Server) clearDNS(ctx context.Context, cn *conn) *netdv1.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, r := s.owned(cn)
	if r != nil {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
	defer cancel()
	if err := s.cfg.DNS.Revert(ctx, o.index); err != nil {
		return failure(0, netdv1.ErrorCode_ERROR_CODE_RESOLVED_UNAVAILABLE, "systemd-resolved is unavailable")
	}
	o.domains = nil
	return &netdv1.Response{Body: &netdv1.Response_ClearDns{ClearDns: &netdv1.ClearDnsResponse{}}}
}

func (s *Server) teardownLocked() {
	o := s.owner
	if o == nil {
		return
	}
	s.owner = nil
	ctx, cancel := context.WithTimeout(context.Background(), backendCallTimeout)
	defer cancel()
	if o.domains != nil {
		if err := s.cfg.DNS.Revert(ctx, o.index); err != nil {
			s.cfg.Log.Warn("reverting resolved link failed", "link", o.name, "err", err.Error())
		}
	}
	if err := s.cfg.Kernel.DeleteLink(o.index); err != nil {
		s.cfg.Log.Error("deleting link failed", "link", o.name, "err", err.Error())
	}
	s.cfg.Log.Info("synthetic interface removed", "link", o.name)
}

func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teardownLocked()
	for cn := range s.conns {
		cn.c.Close()
	}
}

func (s *Server) reconcile(ctx context.Context) {
	links, err := s.cfg.Kernel.Links()
	if err != nil {
		s.cfg.Log.Error("listing links for reconciliation failed", "err", err.Error())
		return
	}
	for _, l := range links {
		rctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
		_ = s.cfg.DNS.Revert(rctx, l.Index)
		cancel()
		if err := s.cfg.Kernel.DeleteLink(l.Index); err != nil {
			s.cfg.Log.Error("removing orphaned link failed", "link", l.Name, "err", err.Error())
			continue
		}
		s.cfg.Log.Info("removed orphaned link", "link", l.Name)
	}
}

func (s *Server) watch(ctx context.Context, stop context.CancelFunc) {
	t := time.NewTicker(s.cfg.Poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.check(ctx, stop)
	}
}

func (s *Server) check(ctx context.Context, stop context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner == nil {
		if s.cfg.IdleExit > 0 && len(s.conns) == 0 && time.Since(s.idleFrom) >= s.cfg.IdleExit {
			s.cfg.Log.Info("idle; exiting")
			stop()
		}
		return
	}
	o := s.owner
	ctx, cancel := context.WithTimeout(ctx, backendCallTimeout)
	defer cancel()
	if active, err := s.cfg.Sessions.Active(ctx, o.session); err != nil || !active {
		s.cfg.Log.Info("owner session no longer active; removing instance", "uid", o.uid)
		s.teardownLocked()
		o.conn.c.Close()
		return
	}
	if o.domains == nil {
		return
	}
	if gen, err := s.cfg.DNS.Generation(ctx); err == nil && gen != o.generation {
		if err := s.cfg.DNS.Apply(ctx, o.index, o.ranges.resolvers(), o.domains); err != nil {
			s.cfg.Log.Warn("reapplying dns after resolved restart failed", "err", err.Error())
			return
		}
		o.generation = gen
		s.cfg.Log.Info("reapplied dns after resolved restart")
	}
}

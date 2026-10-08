package dataplane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/relay"
	"git.lunarlabs.dev/flavor/flavor/internal/syndns"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	MTU          = 1280
	nicID        = 1
	maxInFlight  = 1024
	dialTimeout  = 30 * time.Second
	udpIdle      = 2 * time.Minute
	ipv6Fragment = 44
)

type Dialer func(ctx context.Context, network domain.NetworkID, proto, address string) (net.Conn, error)

type Plane struct {
	alloc *synthetic.Allocator
	dial  Dialer
	log   *slog.Logger
	ula   netip.Prefix
	pool  netip.Prefix
	dev   io.ReadWriteCloser
	stack *stack.Stack
	ep    *channel.Endpoint

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup

	mu     sync.Mutex
	closed bool
	open   map[io.Closer]struct{}
}

func Start(ctx context.Context, dev io.ReadWriteCloser, alloc *synthetic.Allocator, dns *syndns.Engine, dial Dialer, log *slog.Logger) (*Plane, error) {
	s, ep, err := newStack()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Plane{
		alloc: alloc, dial: dial, log: log, ula: alloc.ULA(), pool: alloc.Pool(), dev: dev, stack: s, ep: ep,
		ctx: ctx, cancel: cancel, done: make(chan struct{}), open: make(map[io.Closer]struct{}),
	}
	resolvers := []netip.Addr{layout.ResolverAddress(p.ula)}
	if p.pool.IsValid() {
		resolvers = append(resolvers, layout.ResolverV4(p.pool))
	}
	for _, a := range resolvers {
		if err := p.serveDNS(a, dns); err != nil {
			cancel()
			p.shutdown()
			return nil, err
		}
	}
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcp.NewForwarder(s, 0, maxInFlight, p.tcp).HandlePacket)
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udp.NewForwarder(s, p.udp).HandlePacket)
	p.wg.Add(2)
	go p.inbound()
	go p.outbound()
	go func() {
		<-ctx.Done()
		p.shutdown()
		close(p.done)
	}()
	return p, nil
}

func (p *Plane) Done() <-chan struct{} { return p.done }

func (p *Plane) Close() {
	p.cancel()
	<-p.done
}

func newStack() (*stack.Stack, *channel.Endpoint, error) {
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	sack := tcpip.TCPSACKEnabled(true)
	recovery := tcpip.TCPRecovery(0)
	reno := tcpip.CongestionControlOption("reno")
	for _, opt := range []tcpip.SettableTransportProtocolOption{&sack, &recovery, &reno} {
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, opt); err != nil {
			return nil, nil, fmt.Errorf("netstack tcp option: %v", err)
		}
	}
	ep := channel.New(512, MTU, "")
	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, nil, fmt.Errorf("netstack nic: %v", err)
	}
	s.SetPromiscuousMode(nicID, true)
	if err := s.SetSpoofing(nicID, true); err != nil {
		return nil, nil, fmt.Errorf("netstack spoofing: %v", err)
	}
	var routes []tcpip.Route
	for _, size := range []int{4, 16} {
		sub, err := tcpip.NewSubnet(tcpip.AddrFromSlice(make([]byte, size)), tcpip.MaskFromBytes(make([]byte, size)))
		if err != nil {
			return nil, nil, err
		}
		routes = append(routes, tcpip.Route{Destination: sub, NIC: nicID})
	}
	s.SetRouteTable(routes)
	return s, ep, nil
}

func (p *Plane) serveDNS(a netip.Addr, dns *syndns.Engine) error {
	addr := tcpip.AddrFromSlice(a.AsSlice())
	proto := protocolOf(a)
	if err := p.stack.AddProtocolAddress(nicID, tcpip.ProtocolAddress{Protocol: proto, AddressWithPrefix: addr.WithPrefix()}, stack.AddressProperties{}); err != nil {
		return fmt.Errorf("resolver address %s: %v", a, err)
	}
	full := tcpip.FullAddress{NIC: nicID, Addr: addr, Port: 53}
	pc, err := gonet.DialUDP(p.stack, &full, nil, proto)
	if err != nil {
		return fmt.Errorf("resolver %s udp: %w", a, err)
	}
	ln, err := gonet.ListenTCP(p.stack, full, proto)
	if err != nil {
		pc.Close()
		return fmt.Errorf("resolver %s tcp: %w", a, err)
	}
	p.mu.Lock()
	p.open[pc], p.open[ln] = struct{}{}, struct{}{}
	p.mu.Unlock()
	go dns.ServePacket(p.ctx, pc)
	go dns.ServeStream(p.ctx, ln)
	return nil
}

func protocolOf(a netip.Addr) tcpip.NetworkProtocolNumber {
	if a.Is4() {
		return ipv4.ProtocolNumber
	}
	return ipv6.ProtocolNumber
}

func (p *Plane) inbound() {
	defer p.wg.Done()
	defer p.cancel()
	buf := make([]byte, 1<<16)
	for {
		n, err := p.dev.Read(buf)
		if err != nil {
			return
		}
		proto, ok := p.admit(buf[:n])
		if !ok {
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(slices.Clone(buf[:n]))})
		p.ep.InjectInbound(proto, pkt)
		pkt.DecRef()
	}
}

func (p *Plane) outbound() {
	defer p.wg.Done()
	defer p.cancel()
	for {
		pkt := p.ep.ReadContext(p.ctx)
		if pkt == nil {
			return
		}
		v := pkt.ToView()
		_, err := p.dev.Write(v.AsSlice())
		v.Release()
		pkt.DecRef()
		if err != nil {
			return
		}
	}
}

func (p *Plane) admit(b []byte) (tcpip.NetworkProtocolNumber, bool) {
	var next byte
	var dst netip.Addr
	var proto tcpip.NetworkProtocolNumber
	switch {
	case len(b) >= header.IPv4MinimumSize && b[0]>>4 == 4:
		next, dst, proto = b[9], netip.AddrFrom4([4]byte(b[16:20])), ipv4.ProtocolNumber
	case len(b) >= header.IPv6MinimumSize && b[0]>>4 == 6:
		next, dst, proto = b[6], netip.AddrFrom16([16]byte(b[24:40])), ipv6.ProtocolNumber
		if next == ipv6Fragment && len(b) > header.IPv6MinimumSize {
			next = b[header.IPv6MinimumSize]
		}
	default:
		return 0, false
	}
	if next != byte(tcp.ProtocolNumber) && next != byte(udp.ProtocolNumber) {
		return 0, false
	}
	return proto, p.ula.Contains(dst) || p.pool.Contains(dst)
}

func (p *Plane) track(cs ...io.Closer) (func(), bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		for _, c := range cs {
			c.Close()
		}
		return nil, false
	}
	for _, c := range cs {
		p.open[c] = struct{}{}
	}
	p.wg.Add(1)
	return func() {
		p.mu.Lock()
		for _, c := range cs {
			delete(p.open, c)
		}
		p.mu.Unlock()
		p.wg.Done()
	}, true
}

func (p *Plane) shutdown() {
	p.mu.Lock()
	p.closed = true
	for c := range p.open {
		c.Close()
	}
	p.mu.Unlock()
	p.dev.Close()
	p.ep.Close()
	p.stack.Close()
	p.wg.Wait()
	p.stack.Wait()
}

func (p *Plane) upstream(dst netip.Addr, port uint16, proto string) (net.Conn, func(), error) {
	network, real, err := p.alloc.Resolve(p.ctx, dst)
	if err != nil {
		return nil, nil, err
	}
	release := p.alloc.Acquire(dst)
	ctx, cancel := context.WithTimeout(p.ctx, dialTimeout)
	defer cancel()
	up, err := p.dial(ctx, network, proto, netip.AddrPortFrom(real, port).String())
	if err != nil {
		release()
		return nil, nil, err
	}
	return up, release, nil
}

func (p *Plane) tcp(r *tcp.ForwarderRequest) {
	id := r.ID()
	dst, _ := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	up, release, err := p.upstream(dst, id.LocalPort, "tcp")
	if err != nil {
		p.log.Debug("synthetic tcp flow refused", "dst", dst.String(), "port", id.LocalPort, "err", err.Error())
		r.Complete(true)
		return
	}
	defer release()
	var wq waiter.Queue
	ep, terr := r.CreateEndpoint(&wq)
	if terr != nil {
		r.Complete(true)
		up.Close()
		return
	}
	r.Complete(false)
	client := gonet.NewTCPConn(&wq, ep)
	done, ok := p.track(client, up)
	if !ok {
		return
	}
	defer done()
	relay.Pipe(client, up)
}

func (p *Plane) udp(r *udp.ForwarderRequest) {
	id := r.ID()
	var wq waiter.Queue
	ep, err := r.CreateEndpoint(&wq)
	if err != nil {
		return
	}
	client := gonet.NewUDPConn(&wq, ep)
	dst, _ := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	done, ok := p.track(client)
	if !ok {
		return
	}
	go func() {
		defer done()
		defer client.Close()
		up, release, err := p.upstream(dst, id.LocalPort, "udp")
		if err != nil {
			p.log.Debug("synthetic udp flow refused", "dst", dst.String(), "port", id.LocalPort, "err", err.Error())
			return
		}
		defer release()
		flow, ok := p.track(up)
		if !ok {
			return
		}
		defer flow()
		datagrams(client, up, udpIdle)
	}()
}

func datagrams(a, b net.Conn, idle time.Duration) {
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	var wg sync.WaitGroup
	forward := func(dst, src net.Conn) {
		defer wg.Done()
		defer a.Close()
		defer b.Close()
		buf := make([]byte, 1<<16)
		for {
			_ = src.SetReadDeadline(time.Now().Add(idle))
			n, err := src.Read(buf)
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && time.Since(time.Unix(0, last.Load())) < idle {
				continue
			}
			if err != nil {
				return
			}
			last.Store(time.Now().UnixNano())
			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go forward(b, a)
	go forward(a, b)
	wg.Wait()
}

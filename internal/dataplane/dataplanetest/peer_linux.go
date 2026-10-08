package dataplanetest

import (
	"context"
	"net"
	"net/netip"
	"os"
	"slices"
	"syscall"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

func Pair(t testing.TB) (*os.File, *os.File) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, b := os.NewFile(uintptr(fds[0]), "plane"), os.NewFile(uintptr(fds[1]), "peer")
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

type Peer struct {
	s      *stack.Stack
	ep     *channel.Endpoint
	dev    *os.File
	cancel context.CancelFunc
}

func NewPeer(t testing.TB, dev *os.File, ula, pool netip.Prefix) *Peer {
	t.Helper()
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	ep := channel.New(512, 1280, "")
	if err := s.CreateNIC(1, ep); err != nil {
		t.Fatal(err)
	}
	var routes []tcpip.Route
	add := func(local netip.Addr, dst netip.Prefix) {
		pa := tcpip.ProtocolAddress{Protocol: ipv6.ProtocolNumber, AddressWithPrefix: tcpip.AddrFromSlice(local.AsSlice()).WithPrefix()}
		if local.Is4() {
			pa.Protocol = ipv4.ProtocolNumber
		}
		if err := s.AddProtocolAddress(1, pa, stack.AddressProperties{}); err != nil {
			t.Fatal(err)
		}
		sub, err := tcpip.NewSubnet(tcpip.AddrFromSlice(dst.Masked().Addr().AsSlice()), tcpip.MaskFromBytes(net.CIDRMask(dst.Bits(), dst.Addr().BitLen())))
		if err != nil {
			t.Fatal(err)
		}
		routes = append(routes, tcpip.Route{Destination: sub, NIC: 1})
	}
	add(layout.HostAddress(ula), ula)
	if pool.IsValid() {
		add(layout.HostV4(pool), pool)
	}
	s.SetRouteTable(routes)
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{s: s, ep: ep, dev: dev, cancel: cancel}
	go func() {
		buf := make([]byte, 1<<16)
		for {
			n, err := dev.Read(buf)
			if err != nil {
				return
			}
			proto := ipv6.ProtocolNumber
			if buf[0]>>4 == 4 {
				proto = ipv4.ProtocolNumber
			}
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(slices.Clone(buf[:n]))})
			ep.InjectInbound(proto, pkt)
			pkt.DecRef()
		}
	}()
	go func() {
		for {
			pkt := ep.ReadContext(ctx)
			if pkt == nil {
				return
			}
			v := pkt.ToView()
			_, _ = dev.Write(v.AsSlice())
			v.Release()
			pkt.DecRef()
		}
	}()
	t.Cleanup(p.Close)
	return p
}

func (p *Peer) Close() {
	p.cancel()
	p.dev.Close()
	p.ep.Close()
	p.s.Close()
}

func full(ap netip.AddrPort) (tcpip.FullAddress, tcpip.NetworkProtocolNumber) {
	proto := ipv6.ProtocolNumber
	if ap.Addr().Is4() {
		proto = ipv4.ProtocolNumber
	}
	return tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(ap.Addr().AsSlice()), Port: ap.Port()}, proto
}

func (p *Peer) DialTCP(ctx context.Context, ap netip.AddrPort) (net.Conn, error) {
	addr, proto := full(ap)
	return gonet.DialContextTCP(ctx, p.s, addr, proto)
}

func (p *Peer) DialUDP(ap netip.AddrPort) (net.Conn, error) {
	addr, proto := full(ap)
	return gonet.DialUDP(p.s, nil, &addr, proto)
}

package netd

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/jsimonetti/rtnetlink"
	"golang.org/x/sys/unix"
)

const (
	LinkPrefix    = "lat-u"
	RouteProtocol = 76
)

type Link struct {
	Name  string
	Index uint32
}

type Netlink struct{}

func (Netlink) dial() (*rtnetlink.Conn, error) { return rtnetlink.Dial(nil) }

func (k Netlink) Occupied() ([]netip.Prefix, error) {
	c, err := k.dial()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var out []netip.Prefix
	addrs, err := c.Address.List()
	if err != nil {
		return nil, fmt.Errorf("list addresses: %w", err)
	}
	for _, a := range addrs {
		if a.Attributes == nil {
			continue
		}
		for _, ip := range []net.IP{a.Attributes.Address, a.Attributes.Local} {
			if p, ok := ipPrefix(ip, a.PrefixLength); ok {
				out = append(out, p)
			}
		}
	}
	routes, err := c.Route.List()
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	for _, r := range routes {
		if p, ok := ipPrefix(r.Attributes.Dst, r.DstLength); ok {
			out = append(out, p)
		}
	}
	rules, err := c.Rule.List()
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	for _, r := range rules {
		if r.Attributes != nil && r.Attributes.Dst != nil {
			if p, ok := ipPrefix(*r.Attributes.Dst, r.DstLength); ok {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func ipPrefix(ip net.IP, bits uint8) (netip.Prefix, bool) {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Prefix{}, false
	}
	p, err := a.Unmap().Prefix(int(bits))
	return p, err == nil
}

func (Netlink) CreateTUN(name string) (*os.File, uint32, error) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, 0, err
	}
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	raw, err := f.SyscallConn()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	var ioctlErr error
	if err := raw.Control(func(fd uintptr) { ioctlErr = unix.IoctlIfreq(int(fd), unix.TUNSETIFF, ifr) }); err != nil || ioctlErr != nil {
		f.Close()
		return nil, 0, errors.Join(err, ioctlErr)
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, uint32(iface.Index), nil
}

func (k Netlink) Up(index, mtu uint32) error {
	c, err := k.dial()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Link.Set(&rtnetlink.LinkMessage{Family: unix.AF_UNSPEC, Index: index, Flags: unix.IFF_UP, Change: unix.IFF_UP, Attributes: &rtnetlink.LinkAttributes{MTU: mtu}})
}

func family(a netip.Addr) uint8 {
	if a.Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
}

func (k Netlink) AddAddress(index uint32, a netip.Addr) error {
	c, err := k.dial()
	if err != nil {
		return err
	}
	defer c.Close()
	ip := net.IP(a.AsSlice())
	return c.Address.New(&rtnetlink.AddressMessage{
		Family:       family(a),
		PrefixLength: uint8(a.BitLen()),
		Flags:        unix.IFA_F_NODAD,
		Scope:        unix.RT_SCOPE_UNIVERSE,
		Index:        index,
		Attributes:   &rtnetlink.AddressAttributes{Address: ip, Local: ip, Flags: unix.IFA_F_NODAD},
	})
}

func (k Netlink) AddRoute(index uint32, p netip.Prefix) error {
	c, err := k.dial()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Route.Add(&rtnetlink.RouteMessage{
		Family:     family(p.Addr()),
		DstLength:  uint8(p.Bits()),
		Table:      unix.RT_TABLE_MAIN,
		Protocol:   RouteProtocol,
		Scope:      unix.RT_SCOPE_LINK,
		Type:       unix.RTN_UNICAST,
		Attributes: rtnetlink.RouteAttributes{Dst: net.IP(p.Addr().AsSlice()), OutIface: index},
	})
}

func (k Netlink) DeleteLink(index uint32) error {
	c, err := k.dial()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Link.Delete(index); err != nil && !errors.Is(err, unix.ENODEV) {
		return err
	}
	return nil
}

func (k Netlink) Links() ([]Link, error) {
	c, err := k.dial()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	links, err := c.Link.List()
	if err != nil {
		return nil, err
	}
	var out []Link
	for _, l := range links {
		if l.Attributes != nil && strings.HasPrefix(l.Attributes.Name, LinkPrefix) {
			out = append(out, Link{Name: l.Attributes.Name, Index: l.Index})
		}
	}
	return out, nil
}

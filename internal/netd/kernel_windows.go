package netd

import (
	"net/netip"
	"sync"

	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type Link struct {
	Name  string
	Index uint32
}

type wintunDevice struct {
	*tun.NativeTun
	once sync.Once
	done chan struct{}
}

func (d *wintunDevice) Close() error {
	d.once.Do(func() { close(d.done) })
	return d.NativeTun.Close()
}

type Wintun struct {
	mu   sync.Mutex
	devs map[uint32]*wintunDevice
}

func NewWintun() *Wintun { return &Wintun{devs: map[uint32]*wintunDevice{}} }

func (k *Wintun) Occupied() ([]netip.Prefix, error) {
	var out []netip.Prefix
	addrs, err := winipcfg.GetUnicastIPAddressTable(windows.AF_UNSPEC)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if p, err := a.Address.Addr().Unmap().Prefix(int(a.OnLinkPrefixLength)); err == nil {
			out = append(out, p)
		}
	}
	routes, err := winipcfg.GetIPForwardTable2(windows.AF_UNSPEC)
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		out = append(out, r.DestinationPrefix.Prefix())
	}
	return out, nil
}

func (k *Wintun) CreateTUN(name string) (Device, uint32, error) {
	dev, err := tun.CreateTUN(name, MinMTU)
	if err != nil {
		return nil, 0, err
	}
	d := &wintunDevice{NativeTun: dev.(*tun.NativeTun), done: make(chan struct{})}
	row, err := winipcfg.LUID(d.LUID()).Interface()
	if err != nil {
		d.Close()
		return nil, 0, err
	}
	k.mu.Lock()
	k.devs[row.InterfaceIndex] = d
	k.mu.Unlock()
	return d, row.InterfaceIndex, nil
}

func (k *Wintun) Up(index, mtu uint32) error {
	luid, err := winipcfg.LUIDFromIndex(index)
	if err != nil {
		return err
	}
	for _, family := range []winipcfg.AddressFamily{windows.AF_INET, windows.AF_INET6} {
		ipif, err := luid.IPInterface(family)
		if err != nil {
			return err
		}
		ipif.NLMTU = mtu
		ipif.DadTransmits = 0
		ipif.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
		if err := ipif.Set(); err != nil {
			return err
		}
	}
	return nil
}

func (k *Wintun) AddAddress(index uint32, a netip.Addr) error {
	luid, err := winipcfg.LUIDFromIndex(index)
	if err != nil {
		return err
	}
	return luid.AddIPAddress(netip.PrefixFrom(a, a.BitLen()))
}

func (k *Wintun) AddRoute(index uint32, p netip.Prefix) error {
	luid, err := winipcfg.LUIDFromIndex(index)
	if err != nil {
		return err
	}
	next := netip.IPv6Unspecified()
	if p.Addr().Is4() {
		next = netip.IPv4Unspecified()
	}
	return luid.AddRoute(p, next, 0)
}

func (k *Wintun) DeleteLink(index uint32) error {
	k.mu.Lock()
	d, ok := k.devs[index]
	delete(k.devs, index)
	k.mu.Unlock()
	if !ok {
		return nil
	}
	return d.Close()
}

func (k *Wintun) Links() ([]Link, error) { return nil, nil }

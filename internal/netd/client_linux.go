package netd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"golang.org/x/sys/unix"
)

const DefaultSocket = "/run/flavor/netd.sock"

type Client struct {
	c      *net.UnixConn
	next   uint64
	verify func(f *os.File, name string) error
	Hello  *netdv1.HelloResponse
}

func Dial(ctx context.Context, path string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unixpacket", path)
	if err != nil {
		return nil, err
	}
	c := &Client{c: conn.(*net.UnixConn), verify: VerifyTUN}
	if err := c.hello(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Create(v6, v4 netip.Prefix, mtu uint32) (*netdv1.CreateSyntheticInterfaceResponse, *os.File, error) {
	resp, f, err := c.roundTrip(createRequest(v6, v4, mtu), true)
	if err != nil {
		return nil, nil, err
	}
	out := resp.GetCreateSyntheticInterface()
	if err := c.verify(f, out.GetInterfaceName()); err != nil {
		f.Close()
		return nil, nil, err
	}
	return out, f, nil
}

func VerifyTUN(f *os.File, name string) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var st unix.Stat_t
	var ifr *unix.Ifreq
	var opErr error
	if err := raw.Control(func(fd uintptr) {
		if opErr = unix.Fstat(int(fd), &st); opErr != nil {
			return
		}
		if ifr, opErr = unix.NewIfreq(""); opErr != nil {
			return
		}
		opErr = unix.IoctlIfreq(int(fd), unix.TUNGETIFF, ifr)
	}); err != nil {
		return err
	}
	if opErr != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR {
		return errors.New("received descriptor is not a TUN device")
	}
	if ifr.Name() != name || ifr.Uint16()&(unix.IFF_TUN|unix.IFF_TAP|unix.IFF_NO_PI) != unix.IFF_TUN|unix.IFF_NO_PI {
		return fmt.Errorf("received TUN %q does not match %q in IFF_TUN|IFF_NO_PI mode", ifr.Name(), name)
	}
	return nil
}

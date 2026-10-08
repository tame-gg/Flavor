package netd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultSocket = "/run/flavor/netd.sock"
	clientTimeout = 30 * time.Second
)

type Error struct {
	Code   netdv1.ErrorCode
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("flavor-netd: %s: %s", e.Code, e.Detail) }

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
	resp, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_Hello{Hello: &netdv1.HelloRequest{}}}, false)
	if err != nil {
		conn.Close()
		return nil, err
	}
	c.Hello = resp.GetHello()
	return c, nil
}

func (c *Client) Close() error { return c.c.Close() }

func (c *Client) roundTrip(req *netdv1.Request, wantFile bool) (*netdv1.Response, *os.File, error) {
	c.next++
	req.ProtocolMajor, req.ProtocolMinor, req.RequestId = ProtocolMajor, ProtocolMinor, c.next
	_ = c.c.SetDeadline(time.Now().Add(clientTimeout))
	defer c.c.SetDeadline(time.Time{})
	if err := writePacket(c.c, req, nil); err != nil {
		return nil, nil, err
	}
	b, files, err := readPacket(c.c)
	if err != nil {
		return nil, nil, err
	}
	var resp netdv1.Response
	if err := proto.Unmarshal(b, &resp); err != nil || (resp.RequestId != req.RequestId && (resp.Error == nil || resp.RequestId != 0)) {
		closeAll(files)
		return nil, nil, errProtocol
	}
	if resp.Error != nil {
		closeAll(files)
		return nil, nil, &Error{Code: resp.Error.Code, Detail: resp.Error.Detail}
	}
	if !wantFile || len(files) != 1 {
		closeAll(files)
		if wantFile {
			return nil, nil, fmt.Errorf("%w: expected exactly one descriptor, got %d", errProtocol, len(files))
		}
		return &resp, nil, nil
	}
	return &resp, files[0], nil
}

func (c *Client) Create(v6, v4 netip.Prefix, mtu uint32) (*netdv1.CreateSyntheticInterfaceResponse, *os.File, error) {
	resp, f, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_CreateSyntheticInterface{CreateSyntheticInterface: &netdv1.CreateSyntheticInterfaceRequest{
		V6InstallPrefix: wirePrefix(v6),
		V4Pool:          wirePrefix(v4),
		Mtu:             mtu,
	}}}, true)
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

func (c *Client) Destroy() error {
	_, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_DestroySyntheticInterface{DestroySyntheticInterface: &netdv1.DestroySyntheticInterfaceRequest{}}}, false)
	return err
}

func (c *Client) ConfigureDNS(domains []string) ([]string, error) {
	resp, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_ConfigureDns{ConfigureDns: &netdv1.ConfigureDnsRequest{RoutingDomains: domains}}}, false)
	if err != nil {
		return nil, err
	}
	return resp.GetConfigureDns().GetRoutingDomains(), nil
}

func (c *Client) ClearDNS() error {
	_, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_ClearDns{ClearDns: &netdv1.ClearDnsRequest{}}}, false)
	return err
}

func (c *Client) Status() (*netdv1.StatusResponse, error) {
	resp, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_Status{Status: &netdv1.StatusRequest{}}}, false)
	if err != nil {
		return nil, err
	}
	return resp.GetStatus(), nil
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

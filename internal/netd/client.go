//go:build linux || windows

package netd

import (
	"fmt"
	"net/netip"
	"os"
	"time"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"google.golang.org/protobuf/proto"
)

const clientTimeout = 30 * time.Second

type Error struct {
	Code   netdv1.ErrorCode
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("flavor-netd: %s: %s", e.Code, e.Detail) }

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

func (c *Client) hello() error {
	resp, _, err := c.roundTrip(&netdv1.Request{Body: &netdv1.Request_Hello{Hello: &netdv1.HelloRequest{}}}, false)
	if err != nil {
		return err
	}
	c.Hello = resp.GetHello()
	return nil
}

func createRequest(v6, v4 netip.Prefix, mtu uint32) *netdv1.Request {
	return &netdv1.Request{Body: &netdv1.Request_CreateSyntheticInterface{CreateSyntheticInterface: &netdv1.CreateSyntheticInterfaceRequest{
		V6InstallPrefix: wirePrefix(v6),
		V4Pool:          wirePrefix(v4),
		Mtu:             mtu,
	}}}
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

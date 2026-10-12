package netd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"unsafe"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

const DefaultSocket = `\\.\pipe\flavor-netd`

var ErrForeignHelper = errors.New("the flavor-netd pipe is not served by the system service")

var systemAccounts = []string{"LocalSystem", `NT AUTHORITY\SYSTEM`}

type Client struct {
	c     net.Conn
	next  uint64
	Hello *netdv1.HelloResponse
}

func dialPipe(ctx context.Context, path string) (net.Conn, error) {
	c, err := winio.DialPipeAccess(ctx, path, windows.GENERIC_READ|windows.FILE_WRITE_DATA)
	if err != nil {
		return nil, err
	}
	var pid uint32
	h, ok := c.(handleConn)
	if !ok || windows.GetNamedPipeServerProcessId(windows.Handle(h.Fd()), &pid) != nil {
		c.Close()
		return nil, ErrForeignHelper
	}
	if !serviceServes(pid) {
		c.Close()
		return nil, ErrForeignHelper
	}
	return c, nil
}

func serviceServes(serverPid uint32) bool {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(ServiceName)
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(s)

	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	err = windows.QueryServiceStatusEx(s, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed)
	if err != nil {
		return false
	}

	startName, err := serviceStartName(s)
	if err != nil {
		return false
	}
	return serverAllowed(status.CurrentState, status.ProcessId, serverPid, startName)
}

func serviceStartName(s windows.Handle) (string, error) {
	var needed uint32
	err := windows.QueryServiceConfig(s, nil, 0, &needed)
	if err != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", err
	}
	buf := make([]byte, needed)
	cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
	if err := windows.QueryServiceConfig(s, cfg, needed, &needed); err != nil {
		return "", err
	}
	return windows.UTF16PtrToString(cfg.ServiceStartName), nil
}

func serverAllowed(state, servicePid, serverPid uint32, startName string) bool {
	if state != windows.SERVICE_RUNNING || servicePid == 0 || servicePid != serverPid {
		return false
	}
	for _, account := range systemAccounts {
		if strings.EqualFold(startName, account) {
			return true
		}
	}
	return false
}

func Dial(ctx context.Context, path string) (*Client, error) {
	conn, err := dialPipe(ctx, path)
	if err != nil {
		return nil, err
	}
	c := &Client{c: conn}
	if err := c.hello(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Create(v6, v4 netip.Prefix, mtu uint32) (*netdv1.CreateSyntheticInterfaceResponse, Packets, error) {
	resp, _, err := c.roundTrip(createRequest(v6, v4, mtu), false)
	if err != nil {
		return nil, nil, err
	}
	out := resp.GetCreateSyntheticInterface()
	if !strings.HasPrefix(out.GetPacketPipe(), packetPipePrefix) {
		return nil, nil, fmt.Errorf("%w: unexpected packet pipe %q", errProtocol, out.GetPacketPipe())
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	conn, err := dialPipe(ctx, out.GetPacketPipe())
	if err != nil {
		return nil, nil, err
	}
	return out, newFramedPackets(conn), nil
}

func Installed() bool {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(ServiceName)
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	windows.CloseServiceHandle(s)
	return true
}

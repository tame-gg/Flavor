package netd

import (
	"net"
	"os"
	"strconv"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
)

type (
	Device   = *os.File
	Packets  = *os.File
	wireConn = *net.UnixConn
	userID   = uint32
)

var dnsCapability = netdv1.Capability_CAPABILITY_RESOLVED_DNS

func LinkName(uid uint32) string { return LinkPrefix + strconv.FormatUint(uint64(uid), 10) }

func (s *Server) handoff(_ *conn, tun Device, _ *netdv1.CreateSyntheticInterfaceResponse) (*os.File, error) {
	return tun, nil
}

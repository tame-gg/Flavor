package service

import (
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"unsafe"

	"git.lunarlabs.dev/flavor/flavor/internal/winsid"
	"golang.org/x/sys/windows"
)

const (
	tcpTableOwnerPIDAll = 5
	tcpRowOwnerPID4     = 24
	tcpRowOwnerPID6     = 56
)

var getExtendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func loopbackOwner(c net.Conn) (int, bool) {
	client, err1 := netip.ParseAddrPort(c.RemoteAddr().String())
	server, err2 := netip.ParseAddrPort(c.LocalAddr().String())
	if err1 != nil || err2 != nil {
		return 0, false
	}
	client, server = unmapPort(client), unmapPort(server)
	buf, ok := tcpTable(client.Addr().Is6())
	if !ok {
		return 0, false
	}
	pid, ok := findTCPOwner(buf, client.Addr().Is6(), client, server)
	if !ok || !winsid.IsCurrentUser(pid) {
		return 0, false
	}
	return os.Getuid(), true
}

func tcpTable(v6 bool) ([]byte, bool) {
	family := uintptr(windows.AF_INET)
	if v6 {
		family = windows.AF_INET6
	}
	size := uint32(64 << 10)
	for range 4 {
		buf := make([]byte, size)
		r, _, _ := getExtendedTCPTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, family, tcpTableOwnerPIDAll, 0)
		switch windows.Errno(r) {
		case 0:
			return buf[:size], true
		case windows.ERROR_INSUFFICIENT_BUFFER:
			continue
		default:
			return nil, false
		}
	}
	return nil, false
}

func findTCPOwner(buf []byte, v6 bool, local, remote netip.AddrPort) (uint32, bool) {
	if len(buf) < 4 {
		return 0, false
	}
	n := int(binary.LittleEndian.Uint32(buf))
	rowSize := tcpRowOwnerPID4
	if v6 {
		rowSize = tcpRowOwnerPID6
	}
	rows := buf[4:]
	if n < 0 || n > len(rows)/rowSize {
		return 0, false
	}
	for i := range n {
		row := rows[i*rowSize : (i+1)*rowSize]
		var l, r netip.AddrPort
		var pid uint32
		if v6 {
			l = netip.AddrPortFrom(netip.AddrFrom16([16]byte(row[0:16])), binary.BigEndian.Uint16(row[20:]))
			r = netip.AddrPortFrom(netip.AddrFrom16([16]byte(row[24:40])), binary.BigEndian.Uint16(row[44:]))
			pid = binary.LittleEndian.Uint32(row[52:])
		} else {
			l = netip.AddrPortFrom(netip.AddrFrom4([4]byte(row[4:8])), binary.BigEndian.Uint16(row[8:]))
			r = netip.AddrPortFrom(netip.AddrFrom4([4]byte(row[12:16])), binary.BigEndian.Uint16(row[16:]))
			pid = binary.LittleEndian.Uint32(row[20:])
		}
		if l == local && r == remote {
			return pid, true
		}
	}
	return 0, false
}

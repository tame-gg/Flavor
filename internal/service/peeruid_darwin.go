package service

import (
	"encoding/binary"
	"net"
	"net/netip"

	"golang.org/x/sys/unix"
)

const (
	xsoSocket = 0x001
	xsoInpcb  = 0x010
	inpIPv4   = 0x1
	xinpgenSz = 24
)

func loopbackOwner(c net.Conn) (int, bool) {
	client, err1 := netip.ParseAddrPort(c.RemoteAddr().String())
	server, err2 := netip.ParseAddrPort(c.LocalAddr().String())
	if err1 != nil || err2 != nil {
		return 0, false
	}
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		return 0, false
	}
	return findPCBOwner(buf, unmapPort(client), unmapPort(server))
}

func findPCBOwner(buf []byte, local, remote netip.AddrPort) (int, bool) {
	if len(buf) < 2*xinpgenSz {
		return 0, false
	}
	buf = buf[xinpgenSz : len(buf)-xinpgenSz]
	matched := false
	for len(buf) >= 8 {
		size := int(binary.LittleEndian.Uint32(buf))
		kind := binary.LittleEndian.Uint32(buf[4:])
		step := (size + 7) &^ 7
		if size < 8 || step > len(buf) {
			return 0, false
		}
		rec := buf[:size]
		switch {
		case kind == xsoInpcb && size >= 80:
			l, r := pcbAddrs(rec)
			matched = l == local && r == remote
		case kind == xsoSocket && size >= 68 && matched:
			return int(binary.LittleEndian.Uint32(rec[64:])), true
		}
		buf = buf[step:]
	}
	return 0, false
}

func pcbAddrs(rec []byte) (netip.AddrPort, netip.AddrPort) {
	fport := binary.BigEndian.Uint16(rec[16:])
	lport := binary.BigEndian.Uint16(rec[18:])
	faddr, laddr := rec[48:64], rec[64:80]
	if rec[44]&inpIPv4 != 0 {
		faddr, laddr = faddr[12:], laddr[12:]
	}
	f, _ := netip.AddrFromSlice(faddr)
	l, _ := netip.AddrFromSlice(laddr)
	return netip.AddrPortFrom(l.Unmap(), lport), netip.AddrPortFrom(f.Unmap(), fport)
}

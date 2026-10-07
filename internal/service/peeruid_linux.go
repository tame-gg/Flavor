package service

import (
	"bufio"
	"encoding/hex"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

func loopbackOwner(c net.Conn) (int, bool) {
	client, err1 := netip.ParseAddrPort(c.RemoteAddr().String())
	server, err2 := netip.ParseAddrPort(c.LocalAddr().String())
	if err1 != nil || err2 != nil {
		return 0, false
	}
	path := "/proc/net/tcp"
	if client.Addr().Is6() && !client.Addr().Is4In6() {
		path = "/proc/net/tcp6"
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	return findOwner(f, client, server)
}

func findOwner(r io.Reader, local, remote netip.AddrPort) (int, bool) {
	sc := bufio.NewScanner(r)
	sc.Scan()
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 8 {
			continue
		}
		l, ok1 := parseProcAddr(fields[1])
		rm, ok2 := parseProcAddr(fields[2])
		if !ok1 || !ok2 || l != unmapPort(local) || rm != unmapPort(remote) {
			continue
		}
		uid, err := strconv.Atoi(fields[7])
		if err != nil {
			return 0, false
		}
		return uid, true
	}
	return 0, false
}

func unmapPort(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

func parseProcAddr(s string) (netip.AddrPort, bool) {
	hostHex, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, false
	}
	raw, err := hex.DecodeString(hostHex)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.AddrPort{}, false
	}
	for i := 0; i < len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	addr, _ := netip.AddrFromSlice(raw)
	return netip.AddrPortFrom(addr.Unmap(), uint16(port)), true
}

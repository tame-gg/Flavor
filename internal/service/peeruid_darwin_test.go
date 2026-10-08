package service

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func pcbRecord(local, remote netip.AddrPort, uid uint32) []byte {
	inp := make([]byte, 104)
	binary.LittleEndian.PutUint32(inp, 104)
	binary.LittleEndian.PutUint32(inp[4:], xsoInpcb)
	binary.BigEndian.PutUint16(inp[16:], remote.Port())
	binary.BigEndian.PutUint16(inp[18:], local.Port())
	inp[44] = inpIPv4
	r4, l4 := remote.Addr().As4(), local.Addr().As4()
	copy(inp[60:], r4[:])
	copy(inp[76:], l4[:])
	so := make([]byte, 104)
	binary.LittleEndian.PutUint32(so, 104)
	binary.LittleEndian.PutUint32(so[4:], xsoSocket)
	binary.LittleEndian.PutUint32(so[64:], uid)
	return append(inp, so...)
}

func TestFindPCBOwnerReadsTheClientSocket(t *testing.T) {
	client := netip.MustParseAddrPort("127.0.0.1:49832")
	server := netip.MustParseAddrPort("127.0.0.1:18456")
	buf := make([]byte, xinpgenSz)
	buf = append(buf, pcbRecord(server, client, 1000)...)
	buf = append(buf, pcbRecord(client, server, 1001)...)
	buf = append(buf, make([]byte, xinpgenSz)...)
	uid, ok := findPCBOwner(buf, client, server)
	if !ok || uid != 1001 {
		t.Fatalf("uid=%d ok=%v: must read the client socket's owner, not the server side", uid, ok)
	}
	if _, ok := findPCBOwner(buf, netip.MustParseAddrPort("127.0.0.1:1"), server); ok {
		t.Fatal("unknown socket matched")
	}
	if _, ok := findPCBOwner(buf[:len(buf)-10], client, server); ok {
		t.Fatal("truncated list matched")
	}
}

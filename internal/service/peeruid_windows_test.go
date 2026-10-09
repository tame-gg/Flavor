package service

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func tcp4Row(local, remote netip.AddrPort, pid uint32) []byte {
	row := make([]byte, tcpRowOwnerPID4)
	binary.LittleEndian.PutUint32(row, 5)
	l, r := local.Addr().As4(), remote.Addr().As4()
	copy(row[4:], l[:])
	binary.BigEndian.PutUint16(row[8:], local.Port())
	copy(row[12:], r[:])
	binary.BigEndian.PutUint16(row[16:], remote.Port())
	binary.LittleEndian.PutUint32(row[20:], pid)
	return row
}

func TestFindTCPOwnerReadsTheClientRow(t *testing.T) {
	client := netip.MustParseAddrPort("127.0.0.1:49832")
	server := netip.MustParseAddrPort("127.0.0.1:18456")
	buf := binary.LittleEndian.AppendUint32(nil, 2)
	buf = append(buf, tcp4Row(server, client, 100)...)
	buf = append(buf, tcp4Row(client, server, 200)...)
	pid, ok := findTCPOwner(buf, false, client, server)
	if !ok || pid != 200 {
		t.Fatalf("pid=%d ok=%v: must read the client row, not the server row", pid, ok)
	}
	if _, ok := findTCPOwner(buf, false, netip.MustParseAddrPort("127.0.0.1:1"), server); ok {
		t.Fatal("unknown socket matched")
	}
	if _, ok := findTCPOwner(buf[:len(buf)-1], false, client, server); ok {
		t.Fatal("truncated table matched")
	}
}

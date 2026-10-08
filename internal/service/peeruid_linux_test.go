package service

import (
	"net/netip"
	"strings"
	"testing"
)

const procTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:4818 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1 1 0000000000000000 100 0 0 10 0
   1: 0100007F:C2A8 0100007F:4818 01 00000000:00000000 00:00000000 00000000  1001        0 2 1 0000000000000000 20 4 30 10 -1
   2: 0100007F:4818 0100007F:C2A8 01 00000000:00000000 00:00000000 00000000  1000        0 3 1 0000000000000000 20 4 30 10 -1
`

const procTCP6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:9C40 00000000000000000000000001000000:4818 01 00000000:00000000 00:00000000 00000000  1000        0 4 1 0000000000000000 20 4 30 10 -1
`

func TestFindOwner(t *testing.T) {
	client := netip.MustParseAddrPort("127.0.0.1:49832")
	server := netip.MustParseAddrPort("127.0.0.1:18456")
	uid, ok := findOwner(strings.NewReader(procTCP), client, server)
	if !ok || uid != 1001 {
		t.Fatalf("uid=%d ok=%v: must read the client socket's owner, not the server side", uid, ok)
	}
	if _, ok := findOwner(strings.NewReader(procTCP), netip.MustParseAddrPort("127.0.0.1:1"), server); ok {
		t.Fatal("unknown socket matched")
	}
	uid, ok = findOwner(strings.NewReader(procTCP6), netip.MustParseAddrPort("[::1]:40000"), netip.MustParseAddrPort("[::1]:18456"))
	if !ok || uid != 1000 {
		t.Fatalf("ipv6: uid=%d ok=%v", uid, ok)
	}
}

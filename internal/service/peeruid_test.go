package service

import (
	"net"
	"os"
	"testing"
)

func TestLoopbackOwnerIsCurrentUser(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Logf("%s: %v", addr, err)
			continue
		}
		defer l.Close()
		client, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		server, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		uid, ok := loopbackOwner(server)
		if !ok || uid != os.Getuid() {
			t.Fatalf("%s: uid=%d ok=%v want %d", addr, uid, ok, os.Getuid())
		}
	}
}

package transport

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPipeAcceptsOwnUserBothWays(t *testing.T) {
	path := fmt.Sprintf(`\\.\pipe\flavor-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := Listen(path); err == nil {
		t.Fatal("a second listener on the same pipe name must fail")
	}
	dialed := make(chan error, 1)
	go func() {
		c, err := Dial(context.Background(), path)
		if err == nil {
			defer c.Close()
			_, _ = c.Write([]byte("x"))
			_, _ = c.Read(make([]byte, 1))
		}
		dialed <- err
	}()
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := PeerUID(ConnContext(context.Background(), c))
	if !ok || uid != os.Getuid() {
		t.Fatalf("peer uid %d ok=%v", uid, ok)
	}
	_ = c.Close()
	if err := <-dialed; err != nil {
		t.Fatalf("dial own pipe: %v", err)
	}
}

package relay

import (
	"io"
	"net"
	"sync"
)

func Pipe(client, upstream net.Conn) (sent, received uint64) {
	defer upstream.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(upstream, client)
		sent = uint64(n)
		closeWrite(upstream)
	}()
	n, _ := io.Copy(client, upstream)
	received = uint64(n)
	closeWrite(client)
	wg.Wait()
	return sent, received
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		if cw.CloseWrite() == nil {
			return
		}
	}
	_ = c.Close()
}

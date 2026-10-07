package syndns

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"
)

const streamIdle = 10 * time.Second

func (e *Engine) ServePacket(ctx context.Context, pc net.PacketConn) {
	buf := make([]byte, 1500)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		resp, err := e.Answer(ctx, buf[:n])
		if err != nil {
			continue
		}
		_, _ = pc.WriteTo(resp, from)
	}
}

func (e *Engine) ServeStream(ctx context.Context, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go e.serveConn(ctx, c)
	}
}

func (e *Engine) serveConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	var size [2]byte
	for {
		_ = c.SetDeadline(time.Now().Add(streamIdle))
		if _, err := io.ReadFull(c, size[:]); err != nil {
			return
		}
		q := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, err := io.ReadFull(c, q); err != nil {
			return
		}
		resp, err := e.Answer(ctx, q)
		if err != nil {
			return
		}
		if _, err := c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(resp))), resp...)); err != nil {
			return
		}
	}
}

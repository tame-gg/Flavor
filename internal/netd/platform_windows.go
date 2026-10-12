package netd

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log/slog"
	"net"
	"os"

	netdv1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/netd/v1"
	"github.com/tailscale/go-winio"
)

type (
	Device   = *wintunDevice
	Packets  = io.ReadWriteCloser
	wireConn = net.Conn
	userID   = string
)

const (
	ServiceName      = "flavor-netd"
	AdapterName      = "Flavor"
	packetPipePrefix = `\\.\pipe\flavor-netd-`
	pipeClientAccess = "0x12008b"
	packetPipeBuffer = 1 << 20
)

var dnsCapability = netdv1.Capability_CAPABILITY_NRPT_DNS

func LinkName(string) string { return AdapterName }

func Listen(path string) (net.Listener, error) {
	return winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;" + pipeClientAccess + ";;;IU)"})
}

func (s *Server) handoff(cn *conn, dev Device, out *netdv1.CreateSyntheticInterfaceResponse) (*os.File, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	name := packetPipePrefix + hex.EncodeToString(id[:])
	ln, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;" + pipeClientAccess + ";;;" + cn.peer.UID + ")",
		InputBufferSize:    packetPipeBuffer,
		OutputBufferSize:   packetPipeBuffer,
	})
	if err != nil {
		return nil, err
	}
	go relay(ln, dev, s.cfg.Log)
	out.PacketPipe = name
	return nil, nil
}

func relay(ln net.Listener, dev Device, log *slog.Logger) {
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		ln.Close()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()
	var c net.Conn
	select {
	case <-dev.done:
		ln.Close()
		if c, ok := <-accepted; ok {
			c.Close()
		}
		return
	case got, ok := <-accepted:
		if !ok {
			return
		}
		c = got
	}
	go func() {
		<-dev.done
		c.Close()
	}()
	go func() {
		defer c.Close()
		bufs, sizes := [][]byte{make([]byte, 2+maxFrame)}, []int{0}
		for {
			n, err := dev.Read(bufs, sizes, 2)
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			binary.BigEndian.PutUint16(bufs[0], uint16(sizes[0]))
			if _, err := c.Write(bufs[0][:2+sizes[0]]); err != nil {
				return
			}
		}
	}()
	packets := newFramedPackets(c)
	buf := make([]byte, maxFrame)
	for {
		n, err := packets.Read(buf)
		if err != nil {
			c.Close()
			return
		}
		if _, err := dev.Write([][]byte{buf[:n]}, 0); err != nil {
			log.Debug("writing to the adapter failed", "err", err.Error())
		}
	}
}

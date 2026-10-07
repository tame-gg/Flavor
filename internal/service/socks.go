package service

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/relay"
)

const socksHandshakeTimeout = 10 * time.Second

const (
	socksSucceeded          byte = 0x00
	socksGeneralFailure     byte = 0x01
	socksNotAllowed         byte = 0x02
	socksHostUnreachable    byte = 0x04
	socksConnectionRefused  byte = 0x05
	socksCommandUnsupported byte = 0x07
	socksAddressUnsupported byte = 0x08
	socksNoAcceptableMethod byte = 0xff
)

type ProxyRequest struct {
	Listen string
}

func (s *Service) Proxy(ctx context.Context, req ProxyRequest, emit func(ForwardEvent)) error {
	if err := s.checkRunning(); err != nil {
		return err
	}
	listen, err := parseLoopback(req.Listen)
	if err != nil {
		return err
	}
	if !s.forwardSlot.TryAcquire() {
		return fail(CodeBusy, "too many active forwards and proxies", true)
	}
	defer s.forwardSlot.Release()
	return s.serveLoopback(ctx, listen,
		func(bound netip.AddrPort) { emit(ForwardEvent{Kind: ForwardStarted, Listen: bound}) },
		func(ctx context.Context, c net.Conn, id uint64) { s.serveSocks(ctx, c, id, emit) },
		func(id uint64, client string, err error) {
			emit(ForwardEvent{Kind: ForwardRefused, ConnID: id, Client: client, Err: err})
		})
}

func (s *Service) serveSocks(ctx context.Context, c net.Conn, id uint64, emit func(ForwardEvent)) {
	defer c.Close()
	client := c.RemoteAddr().String()
	_ = c.SetDeadline(time.Now().Add(socksHandshakeTimeout))
	dest, code := socksHandshake(c)
	if code != socksSucceeded {
		socksReply(c, code)
		emit(ForwardEvent{Kind: ForwardRefused, ConnID: id, Client: client, Destination: dest, Err: fail(CodeInvalidArgument, "unsupported SOCKS request", false)})
		return
	}
	route, err := s.Route(ctx, dest, "", 0)
	var up net.Conn
	if err == nil {
		up, err = s.Dial(ctx, route)
	}
	if err != nil {
		socksReply(c, socksCode(err))
		emit(ForwardEvent{Kind: ForwardRefused, ConnID: id, Client: client, Destination: dest, Route: route, Err: err})
		return
	}
	socksReply(c, socksSucceeded)
	_ = c.SetDeadline(time.Time{})
	emit(ForwardEvent{Kind: ForwardOpened, ConnID: id, Client: client, Destination: dest, Route: route})
	sent, received := relay.Pipe(c, up)
	emit(ForwardEvent{Kind: ForwardClosed, ConnID: id, Client: client, Destination: dest, Route: route, BytesSent: sent, BytesReceived: received})
}

func socksHandshake(c net.Conn) (string, byte) {
	var hdr [2]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil || hdr[0] != 5 {
		return "", socksGeneralFailure
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return "", socksGeneralFailure
	}
	noAuth := false
	for _, m := range methods {
		noAuth = noAuth || m == 0
	}
	if !noAuth {
		_, _ = c.Write([]byte{5, socksNoAcceptableMethod})
		return "", socksNoAcceptableMethod
	}
	if _, err := c.Write([]byte{5, 0}); err != nil {
		return "", socksGeneralFailure
	}
	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil || req[0] != 5 {
		return "", socksGeneralFailure
	}
	if req[1] != 1 {
		return "", socksCommandUnsupported
	}
	var host string
	switch req[3] {
	case 1, 4:
		raw := make([]byte, 4)
		if req[3] == 4 {
			raw = make([]byte, 16)
		}
		if _, err := io.ReadFull(c, raw); err != nil {
			return "", socksGeneralFailure
		}
		addr, _ := netip.AddrFromSlice(raw)
		host = addr.Unmap().String()
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(c, n[:]); err != nil {
			return "", socksGeneralFailure
		}
		name := make([]byte, n[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return "", socksGeneralFailure
		}
		host = string(name)
	default:
		return "", socksAddressUnsupported
	}
	var port [2]byte
	if _, err := io.ReadFull(c, port[:]); err != nil {
		return "", socksGeneralFailure
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))), socksSucceeded
}

func socksReply(c net.Conn, code byte) {
	if code == socksNoAcceptableMethod {
		return
	}
	_, _ = c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

func socksCode(err error) byte {
	var se *Error
	if !errors.As(err, &se) {
		return socksGeneralFailure
	}
	switch se.Code {
	case CodeDestinationAmbiguous:
		return socksNotAllowed
	case CodeDestinationNotFound:
		return socksHostUnreachable
	case CodeDestinationUnreachable:
		return socksConnectionRefused
	case CodeInvalidArgument:
		return socksAddressUnsupported
	}
	return socksGeneralFailure
}

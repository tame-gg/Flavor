package service

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"

	"git.lunarlabs.dev/flavor/flavor/internal/relay"
)

const (
	MaxForwards              = 32
	MaxConnectionsPerForward = 64
)

type ForwardEventKind int

const (
	ForwardStarted ForwardEventKind = iota + 1
	ForwardOpened
	ForwardClosed
	ForwardRefused
)

type ForwardEvent struct {
	Kind          ForwardEventKind
	Listen        netip.AddrPort
	Route         Route
	Destination   string
	ConnID        uint64
	Client        string
	BytesSent     uint64
	BytesReceived uint64
	Err           error
}

type ForwardRequest struct {
	Destination string
	Listen      string
	Network     string
}

func parseLoopback(raw string) (netip.AddrPort, error) {
	if raw == "" {
		raw = "127.0.0.1:0"
	}
	ap, err := netip.ParseAddrPort(raw)
	if err != nil {
		if a, aerr := netip.ParseAddr(raw); aerr == nil {
			ap = netip.AddrPortFrom(a, 0)
		} else {
			return netip.AddrPort{}, fail(CodeInvalidArgument, "listen address must be an IP and port such as 127.0.0.1:15432", false)
		}
	}
	if !ap.Addr().Unmap().IsLoopback() {
		return netip.AddrPort{}, fail(CodeInvalidArgument, "only loopback listen addresses (127.0.0.1 or ::1) are allowed", false)
	}
	return ap, nil
}

func (s *Service) Forward(ctx context.Context, req ForwardRequest, emit func(ForwardEvent)) error {
	if err := s.checkRunning(); err != nil {
		return err
	}
	listen, err := parseLoopback(req.Listen)
	if err != nil {
		return err
	}
	route, err := s.Route(ctx, req.Destination, req.Network, 0)
	if err != nil {
		return err
	}
	if !s.forwardSlot.TryAcquire() {
		return fail(CodeBusy, "too many active forwards and proxies", true)
	}
	defer s.forwardSlot.Release()
	emit, stopped := logForward(s.cfg.Log.With("forward", req.Destination), emit)
	defer stopped()
	return s.serveLoopback(ctx, listen,
		func(bound netip.AddrPort) { emit(ForwardEvent{Kind: ForwardStarted, Listen: bound, Route: route}) },
		func(ctx context.Context, c net.Conn, id uint64) { s.serveForward(ctx, req, c, id, emit) },
		func(id uint64, client string, err error) {
			emit(ForwardEvent{Kind: ForwardRefused, ConnID: id, Client: client, Err: err})
		})
}

func logForward(log *slog.Logger, emit func(ForwardEvent)) (func(ForwardEvent), func()) {
	var started []any
	return func(e ForwardEvent) {
			var route []any
			if e.Destination != "" {
				route = append(route, "destination", e.Destination)
			}
			if e.Route.Network.ID != "" {
				route = append(route, "network_id", e.Route.Network.ID, "target", e.Route.DialAddress())
			}
			conn := append([]any{"conn", e.ConnID, "client", e.Client}, route...)
			switch e.Kind {
			case ForwardStarted:
				started = append([]any{"listen", e.Listen}, route...)
				log.Info("listener started", started...)
			case ForwardRefused:
				log.Warn("connection refused", append(conn, "err", e.Err)...)
			case ForwardOpened:
				log.Debug("connection opened", conn...)
			case ForwardClosed:
				log.Debug("connection closed", append(conn, "sent", e.BytesSent, "received", e.BytesReceived)...)
			}
			emit(e)
		}, func() {
			if started != nil {
				log.Info("listener stopped", started...)
			}
		}
}

func (s *Service) serveLoopback(
	ctx context.Context,
	listen netip.AddrPort,
	started func(netip.AddrPort),
	handle func(context.Context, net.Conn, uint64),
	refuse func(id uint64, client string, err error),
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := s.trackForward(cancel)
	defer stop()

	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", listen.String())
	if err != nil {
		return fail(CodeInvalidArgument, "could not listen on "+listen.String(), false)
	}
	bound, _ := netip.ParseAddrPort(l.Addr().String())
	started(bound)

	var wg sync.WaitGroup
	var ids atomic.Uint64
	conns := newConnSet()
	go func() {
		<-ctx.Done()
		_ = l.Close()
		conns.closeAll()
	}()
	self := os.Getuid()
	for {
		c, err := l.Accept()
		if err != nil {
			break
		}
		id := ids.Add(1)
		client := c.RemoteAddr().String()
		if uid, ok := loopbackOwner(c); !ok || uid != self {
			_ = c.Close()
			refuse(id, client, fail(CodeInvalidArgument, "connection from another local user refused", false))
			continue
		}
		if !conns.add(c) {
			_ = c.Close()
			if ctx.Err() == nil {
				refuse(id, client, fail(CodeBusy, "too many concurrent connections", true))
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conns.remove(c)
			handle(ctx, c, id)
		}()
	}
	cancel()
	wg.Wait()
	if s.isShuttingDown() {
		return fail(CodeShuttingDown, "daemon is shutting down", true)
	}
	return nil
}

func (s *Service) serveForward(ctx context.Context, req ForwardRequest, c net.Conn, id uint64, emit func(ForwardEvent)) {
	defer c.Close()
	client := c.RemoteAddr().String()
	route, err := s.Route(ctx, req.Destination, req.Network, 0)
	if err == nil {
		var up net.Conn
		if up, err = s.Dial(ctx, route); err == nil {
			emit(ForwardEvent{Kind: ForwardOpened, ConnID: id, Client: client, Route: route})
			sent, received := relay.Pipe(c, up)
			emit(ForwardEvent{Kind: ForwardClosed, ConnID: id, Client: client, Route: route, BytesSent: sent, BytesReceived: received})
			return
		}
	}
	emit(ForwardEvent{Kind: ForwardRefused, ConnID: id, Client: client, Route: route, Err: err})
}

type connSet struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func newConnSet() *connSet { return &connSet{conns: make(map[net.Conn]struct{})} }

func (s *connSet) add(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns == nil || len(s.conns) >= MaxConnectionsPerForward {
		return false
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *connSet) remove(c net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c)
}

func (s *connSet) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

type slots struct {
	ch chan struct{}
}

func newSlots(n int) *slots { return &slots{ch: make(chan struct{}, n)} }

func (s *slots) TryAcquire() bool {
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *slots) Release() { <-s.ch }

func (s *Service) trackForward(cancel context.CancelFunc) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextForward++
	id := s.nextForward
	s.forwards[id] = cancel
	return func() {
		s.mu.Lock()
		delete(s.forwards, id)
		s.mu.Unlock()
	}
}

func (s *Service) isShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
}

func unmapPort(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

package transport

import (
	"context"
	"net"
	"net/http"
	"os"
)

type peerKey struct{}

type PeerConn struct {
	*net.UnixConn
	UID int
}

type peerListener struct {
	*net.UnixListener
	uid int
}

func Listen(path string) (net.Listener, error) {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return &peerListener{UnixListener: l, uid: os.Getuid()}, nil
}

func (l *peerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(c)
		if err != nil || uid != l.uid {
			_ = c.Close()
			continue
		}
		return &PeerConn{UnixConn: c, UID: uid}, nil
	}
}

func ConnContext(ctx context.Context, c net.Conn) context.Context {
	if pc, ok := c.(*PeerConn); ok {
		return context.WithValue(ctx, peerKey{}, pc.UID)
	}
	return ctx
}

func PeerUID(ctx context.Context) (int, bool) {
	uid, ok := ctx.Value(peerKey{}).(int)
	return uid, ok
}

func RequirePeer(next http.Handler) http.Handler {
	self := os.Getuid()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := PeerUID(r.Context()); !ok || uid != self {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

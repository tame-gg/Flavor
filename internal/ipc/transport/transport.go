package transport

import (
	"context"
	"net"
	"net/http"
	"os"
)

type peerKey struct{}

type PeerConn struct {
	net.Conn
	UID int
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

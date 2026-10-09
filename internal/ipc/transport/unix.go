//go:build !windows

package transport

import (
	"context"
	"net"
	"os"
)

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
		return &PeerConn{Conn: c, UID: uid}, nil
	}
}

func Dial(ctx context.Context, path string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", path)
}

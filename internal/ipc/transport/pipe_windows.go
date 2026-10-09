package transport

import (
	"context"
	"errors"
	"net"
	"os"

	"git.lunarlabs.dev/flavor/flavor/internal/winsid"
	"github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

var ErrForeignPipe = errors.New("the flavord pipe belongs to another user")

type handleConn interface {
	net.Conn
	Fd() uintptr
}

type pipeListener struct {
	net.Listener
}

func Listen(path string) (net.Listener, error) {
	self, err := winsid.Current()
	if err != nil {
		return nil, err
	}
	l, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + self.String() + ")"})
	if err != nil {
		return nil, err
	}
	return &pipeListener{Listener: l}, nil
}

func (l *pipeListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		var pid uint32
		if h, ok := c.(handleConn); ok && windows.GetNamedPipeClientProcessId(windows.Handle(h.Fd()), &pid) == nil && winsid.IsCurrentUser(pid) {
			return &PeerConn{Conn: c, UID: os.Getuid()}, nil
		}
		_ = c.Close()
	}
}

func Dial(ctx context.Context, path string) (net.Conn, error) {
	c, err := winio.DialPipeContext(ctx, path)
	if err != nil {
		return nil, err
	}
	var pid uint32
	if h, ok := c.(handleConn); ok && windows.GetNamedPipeServerProcessId(windows.Handle(h.Fd()), &pid) == nil && winsid.IsCurrentUser(pid) {
		return c, nil
	}
	_ = c.Close()
	return nil, ErrForeignPipe
}

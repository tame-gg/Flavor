package netd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const listenFD = 3

func ActivatedListener() (*net.UnixListener, error) {
	if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) || os.Getenv("LISTEN_FDS") != "1" {
		return nil, errors.New("lattice-netd must be started by lattice-netd.socket")
	}
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")
	unix.CloseOnExec(listenFD)
	typ, err := unix.GetsockoptInt(listenFD, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil || typ != unix.SOCK_SEQPACKET {
		return nil, fmt.Errorf("activated socket is not SOCK_SEQPACKET (type %d, err %v)", typ, err)
	}
	f := os.NewFile(listenFD, "lattice-netd.socket")
	defer f.Close()
	l, err := net.FileListener(f)
	if err != nil {
		return nil, err
	}
	ul, ok := l.(*net.UnixListener)
	if !ok {
		l.Close()
		return nil, errors.New("activated socket is not a unix socket")
	}
	return ul, nil
}

func Notify(state string) error {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return nil
	}
	c, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Write([]byte(state))
	return err
}

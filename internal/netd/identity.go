package netd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

var ErrUnidentified = errors.New("peer identity cannot be established reliably")

type Peer struct {
	UID       uint32
	PID       int32
	PIDFD     *os.File
	StartTime uint64
}

func (p Peer) Close() {
	if p.PIDFD != nil {
		p.PIDFD.Close()
	}
}

func Identify(c *net.UnixConn) (Peer, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return Peer{}, err
	}
	var p Peer
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			credErr = err
			return
		}
		p.UID, p.PID = cred.Uid, cred.Pid
		if pidfd, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PEERPIDFD); err == nil {
			unix.CloseOnExec(pidfd)
			p.PIDFD = os.NewFile(uintptr(pidfd), "peer-pidfd")
		}
	}); err != nil {
		return Peer{}, err
	}
	if credErr != nil {
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, credErr)
	}
	if p.PID <= 0 {
		p.Close()
		return Peer{}, ErrUnidentified
	}
	start, err := startTime(p.PID)
	if err != nil {
		p.Close()
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	p.StartTime = start
	if p.PIDFD != nil && !alive(p.PIDFD) {
		p.Close()
		return Peer{}, fmt.Errorf("%w: peer exited during identification", ErrUnidentified)
	}
	return p, nil
}

func alive(pidfd *os.File) bool {
	raw, err := pidfd.SyscallConn()
	if err != nil {
		return false
	}
	ready := 1
	if err := raw.Control(func(fd uintptr) {
		ready, err = unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 0)
	}); err != nil || ready != 0 {
		return false
	}
	return err == nil
}

func startTime(pid int32) (uint64, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(int(pid)) + "/stat")
	if err != nil {
		return 0, err
	}
	return parseStartTime(string(b))
}

func parseStartTime(stat string) (uint64, error) {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, errors.New("malformed stat")
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 20 {
		return 0, errors.New("malformed stat")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

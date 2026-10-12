package netd

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

var ErrUnidentified = errors.New("peer identity cannot be established reliably")

type Peer struct {
	UID     string
	PID     uint32
	Session uint32
	Process windows.Handle
	Token   windows.Token
}

func (p Peer) Close() {
	if p.Token != 0 {
		p.Token.Close()
	}
	if p.Process != 0 {
		windows.CloseHandle(p.Process)
	}
}

type handleConn interface {
	Fd() uintptr
}

func Identify(c wireConn) (Peer, error) {
	h, ok := c.(handleConn)
	if !ok {
		return Peer{}, ErrUnidentified
	}
	var p Peer
	if err := windows.GetNamedPipeClientProcessId(windows.Handle(h.Fd()), &p.PID); err != nil || p.PID == 0 {
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, p.PID)
	if err != nil {
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	p.Process = proc
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &p.Token); err != nil {
		p.Close()
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	user, err := p.Token.GetTokenUser()
	if err != nil {
		p.Close()
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	p.UID = user.User.Sid.String()
	if err := windows.ProcessIdToSessionId(p.PID, &p.Session); err != nil {
		p.Close()
		return Peer{}, fmt.Errorf("%w: %v", ErrUnidentified, err)
	}
	if ev, err := windows.WaitForSingleObject(proc, 0); err != nil || ev != uint32(windows.WAIT_TIMEOUT) {
		p.Close()
		return Peer{}, fmt.Errorf("%w: peer exited during identification", ErrUnidentified)
	}
	return p, nil
}

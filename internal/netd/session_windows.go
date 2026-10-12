package netd

import (
	"context"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Interactive struct{}

func (Interactive) Authorize(_ context.Context, p Peer) (bool, error) {
	interactive, err := windows.CreateWellKnownSid(windows.WinInteractiveSid)
	if err != nil {
		return false, err
	}
	groups, err := p.Token.GetTokenGroups()
	if err != nil {
		return false, err
	}
	for _, g := range groups.AllGroups() {
		if g.Attributes&windows.SE_GROUP_ENABLED != 0 && g.Sid.Equals(interactive) {
			return true, nil
		}
	}
	return false, nil
}

type Terminal struct{}

func (Terminal) Session(_ context.Context, p Peer) (string, error) {
	return strconv.FormatUint(uint64(p.Session), 10), nil
}

func (Terminal) Active(_ context.Context, session string) (bool, error) {
	id, err := strconv.ParseUint(session, 10, 32)
	if err != nil {
		return false, err
	}
	var infos *windows.WTS_SESSION_INFO
	var n uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &infos, &n); err != nil {
		return false, err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(infos)))
	for _, s := range unsafe.Slice(infos, n) {
		if s.SessionID == uint32(id) {
			return s.State == windows.WTSActive, nil
		}
	}
	return false, nil
}

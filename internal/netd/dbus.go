package netd

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"

	"github.com/godbus/dbus/v5"
)

const ManageAction = "dev.lunarlabs.lattice.netd.manage-interface"

type polkitSubject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type polkitResult struct {
	Authorized bool
	Challenge  bool
	Details    map[string]string
}

func pidfdSubject(p Peer) polkitSubject {
	return polkitSubject{Kind: "unix-process", Details: map[string]dbus.Variant{
		"pidfd": dbus.MakeVariant(dbus.UnixFD(p.PIDFD.Fd())),
		"uid":   dbus.MakeVariant(int32(p.UID)),
	}}
}

func processSubject(p Peer) polkitSubject {
	return polkitSubject{Kind: "unix-process", Details: map[string]dbus.Variant{
		"pid":        dbus.MakeVariant(uint32(p.PID)),
		"start-time": dbus.MakeVariant(p.StartTime),
		"uid":        dbus.MakeVariant(int32(p.UID)),
	}}
}

type Polkit struct {
	Conn *dbus.Conn
	Log  *slog.Logger
}

func (a Polkit) Authorize(ctx context.Context, p Peer) (bool, error) {
	if p.PIDFD != nil {
		ok, err := a.check(ctx, pidfdSubject(p))
		var rejected dbus.Error
		if !errors.As(err, &rejected) {
			return ok, err
		}
		a.Log.Info("polkit rejected the pidfd subject; using pid and start time", "err", rejected.Error())
	}
	return a.check(ctx, processSubject(p))
}

func (a Polkit) check(ctx context.Context, s polkitSubject) (bool, error) {
	var r polkitResult
	err := a.Conn.Object("org.freedesktop.PolicyKit1", "/org/freedesktop/PolicyKit1/Authority").
		CallWithContext(ctx, "org.freedesktop.PolicyKit1.Authority.CheckAuthorization", 0, s, ManageAction, map[string]string{}, uint32(0), "").
		Store(&r)
	return err == nil && r.Authorized, err
}

type resolvedAddress struct {
	Family  int32
	Address []byte
}

type resolvedDomain struct {
	Domain      string
	RoutingOnly bool
}

type Resolved struct{ Conn *dbus.Conn }

func (r Resolved) manager() dbus.BusObject {
	return r.Conn.Object("org.freedesktop.resolve1", "/org/freedesktop/resolve1")
}

func (r Resolved) Apply(ctx context.Context, ifindex uint32, servers []netip.Addr, domains []string) error {
	var addrs []resolvedAddress
	for _, s := range servers {
		family := int32(10)
		if s.Is4() {
			family = 2
		}
		addrs = append(addrs, resolvedAddress{Family: family, Address: s.AsSlice()})
	}
	var doms []resolvedDomain
	for _, d := range domains {
		doms = append(doms, resolvedDomain{Domain: d, RoutingOnly: true})
	}
	m, idx := r.manager(), int32(ifindex)
	for _, call := range []struct {
		method string
		arg    any
	}{
		{"SetLinkDNS", addrs},
		{"SetLinkDomains", doms},
		{"SetLinkDefaultRoute", false},
	} {
		if err := m.CallWithContext(ctx, "org.freedesktop.resolve1.Manager."+call.method, 0, idx, call.arg).Err; err != nil {
			_ = r.Revert(ctx, ifindex)
			return err
		}
	}
	return nil
}

func (r Resolved) Revert(ctx context.Context, ifindex uint32) error {
	return r.manager().CallWithContext(ctx, "org.freedesktop.resolve1.Manager.RevertLink", 0, int32(ifindex)).Err
}

func (r Resolved) Generation(ctx context.Context) (string, error) {
	var owner string
	err := r.Conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, "org.freedesktop.resolve1").Store(&owner)
	return owner, err
}

type Logind struct{ Conn *dbus.Conn }

func (l Logind) Session(ctx context.Context, p Peer) (string, error) {
	m := l.Conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")
	var path dbus.ObjectPath
	if err := m.CallWithContext(ctx, "org.freedesktop.login1.Manager.GetSessionByPID", 0, uint32(p.PID)).Store(&path); err == nil {
		return string(path), nil
	}
	var user dbus.ObjectPath
	if err := m.CallWithContext(ctx, "org.freedesktop.login1.Manager.GetUser", 0, p.UID).Store(&user); err != nil {
		return "", err
	}
	v, err := l.Conn.Object("org.freedesktop.login1", user).GetProperty("org.freedesktop.login1.User.Display")
	if err != nil {
		return "", err
	}
	var display struct {
		ID   string
		Path dbus.ObjectPath
	}
	if err := v.Store(&display); err != nil || display.ID == "" {
		return "", errors.New("user has no graphical session")
	}
	return string(display.Path), nil
}

func (l Logind) Active(ctx context.Context, session string) (bool, error) {
	obj := l.Conn.Object("org.freedesktop.login1", dbus.ObjectPath(session))
	active, err := obj.GetProperty("org.freedesktop.login1.Session.Active")
	if err != nil {
		return false, err
	}
	remote, err := obj.GetProperty("org.freedesktop.login1.Session.Remote")
	if err != nil {
		return false, err
	}
	a, _ := active.Value().(bool)
	r, ok := remote.Value().(bool)
	return a && ok && !r, nil
}

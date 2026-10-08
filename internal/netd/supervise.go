package netd

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"time"
)

const (
	DefaultBackoff     = time.Second
	MaxBackoff         = time.Minute
	DefaultProbe       = 30 * time.Second
	WarnUnavailable    = "synthetic_unavailable"
	WarnDNSUnavailable = "synthetic_dns_unavailable"
)

type Attach func(ctx context.Context, tun *os.File) (done <-chan struct{}, stop func(), err error)

type Supervisor struct {
	Socket  string
	V6, V4  netip.Prefix
	MTU     uint32
	Attach  Attach
	Log     *slog.Logger
	Warn    func(code, message string)
	Backoff time.Duration
	Probe   time.Duration

	dial func(ctx context.Context, path string) (*Client, error)
}

func (s Supervisor) Run(ctx context.Context) {
	if s.Backoff == 0 {
		s.Backoff = DefaultBackoff
	}
	if s.Probe == 0 {
		s.Probe = DefaultProbe
	}
	if s.dial == nil {
		s.dial = Dial
	}
	wait, reported := s.Backoff, ""
	for {
		err := s.once(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			wait, reported = s.Backoff, ""
			s.Log.Info("synthetic interface lost; recreating")
		} else if msg := err.Error(); msg != reported {
			reported = msg
			s.Log.Warn("synthetic networking unavailable", "err", msg)
			s.Warn(WarnUnavailable, "Transparent Lattice names and addresses are unavailable: "+msg)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, MaxBackoff)
	}
}

func (s Supervisor) once(ctx context.Context) error {
	c, err := s.dial(ctx, s.Socket)
	if err != nil {
		return fmt.Errorf("lattice-netd is not reachable: %w", err)
	}
	defer c.Close()
	created, tun, err := c.Create(s.V6, s.V4, s.MTU)
	if err != nil {
		return err
	}
	done, stop, err := s.Attach(ctx, tun)
	if err != nil {
		_ = c.Destroy()
		return err
	}
	defer stop()
	s.Log.Info("synthetic interface active", "link", created.InterfaceName)
	if _, err := c.ConfigureDNS(nil); err != nil {
		s.Log.Warn("system dns not configured for lattice names", "err", err.Error())
		s.Warn(WarnDNSUnavailable, "Lattice names will not resolve system-wide: "+err.Error())
	}
	probe := time.NewTicker(s.Probe)
	defer probe.Stop()
	for {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			_ = c.Destroy()
			return nil
		case <-probe.C:
			if _, err := c.Status(); err != nil {
				s.Log.Info("lost the connection to lattice-netd", "err", err.Error())
				return nil
			}
		}
	}
}

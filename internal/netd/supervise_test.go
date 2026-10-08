//go:build linux

package netd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

type attachments struct {
	mu       sync.Mutex
	attached int
	warnings []string
}

func (a *attachments) attach(_ context.Context, tun *os.File) (<-chan struct{}, func(), error) {
	a.mu.Lock()
	a.attached++
	a.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		for {
			if _, err := tun.Read(buf); err != nil {
				return
			}
		}
	}()
	return done, func() { tun.Close(); <-done }, nil
}

func (a *attachments) warn(code, _ string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.warnings = append(a.warnings, code)
}

func (a *attachments) counts() (int, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.attached, append([]string(nil), a.warnings...)
}

func supervise(t *testing.T, h *harness, a *attachments) (context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		Supervisor{
			Socket: h.path, V6: ula, V4: pool, MTU: 1280,
			Attach: a.attach, Warn: a.warn, Log: slog.New(slog.DiscardHandler),
			Backoff: 20 * time.Millisecond, Probe: 20 * time.Millisecond,
			dial: func(ctx context.Context, path string) (*Client, error) {
				c, err := Dial(ctx, path)
				if c != nil {
					c.verify = func(*os.File, string) error { return nil }
				}
				return c, err
			},
		}.Run(ctx)
	}()
	t.Cleanup(func() { cancel(); <-stopped })
	return cancel, stopped
}

func TestSupervisorCreatesConfiguresAndCleansUp(t *testing.T) {
	h := newHarness(t, nil)
	a := &attachments{}
	cancel, stopped := supervise(t, h, a)
	eventually(t, "interface and dns", func() bool {
		var dns int
		h.with(func() { dns = len(h.dns.applied) })
		return h.linkCount() == 1 && dns == 1
	})
	cancel()
	<-stopped
	if h.linkCount() != 0 {
		t.Fatal("shutdown must destroy the interface")
	}
}

func TestSupervisorRecreatesAfterLossAndWarnsOncePerCause(t *testing.T) {
	h := newHarness(t, nil)
	a := &attachments{}
	supervise(t, h, a)
	eventually(t, "first instance", func() bool { return h.linkCount() == 1 })

	h.with(func() { h.sessions.inactive, h.auth.deny = true, true })
	eventually(t, "teardown", func() bool { return h.linkCount() == 0 })
	time.Sleep(200 * time.Millisecond)
	if _, warnings := a.counts(); len(warnings) != 1 || warnings[0] != WarnUnavailable {
		t.Fatalf("repeated refusals must warn once: %v", warnings)
	}

	h.with(func() { h.sessions.inactive, h.auth.deny = false, false })
	eventually(t, "recreated", func() bool {
		n, _ := a.counts()
		return n == 2 && h.linkCount() == 1
	})
}

func TestSupervisorNoticesAHelperThatDied(t *testing.T) {
	h := newHarness(t, nil)
	a := &attachments{}
	supervise(t, h, a)
	eventually(t, "first instance", func() bool { return h.linkCount() == 1 })
	h.srv.mu.Lock()
	h.srv.owner = nil
	for cn := range h.srv.conns {
		cn.c.Close()
	}
	h.srv.mu.Unlock()
	eventually(t, "reconnect after probe failure", func() bool {
		n, _ := a.counts()
		return n == 2
	})
}

func TestSupervisorWarnsWhenTheHelperIsMissing(t *testing.T) {
	a := &attachments{}
	h := &harness{path: "/nonexistent/netd.sock"}
	supervise(t, h, a)
	eventually(t, "warning", func() bool {
		_, w := a.counts()
		return len(w) == 1
	})
}

func TestSupervisorDestroysBeforeStoppingThePlane(t *testing.T) {
	h := newHarness(t, nil)
	var order []string
	var mu sync.Mutex
	note := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}
	a := &attachments{}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		Supervisor{
			Socket: h.path, V6: ula, V4: pool, MTU: 1280, Warn: a.warn, Log: slog.New(slog.DiscardHandler),
			Attach: func(actx context.Context, tun *os.File) (<-chan struct{}, func(), error) {
				done, stop, err := a.attach(actx, tun)
				go func() {
					<-actx.Done()
					note("plane context cancelled")
				}()
				return done, func() {
					h.with(func() { note(fmt.Sprintf("plane stopped, link deleted=%v", len(h.kernel.deleted) == 1)) })
					stop()
				}, err
			},
			dial: func(ctx context.Context, path string) (*Client, error) {
				c, err := Dial(ctx, path)
				if c != nil {
					c.verify = func(*os.File, string) error { return nil }
				}
				return c, err
			},
		}.Run(ctx)
	}()
	eventually(t, "instance", func() bool { return h.linkCount() == 1 })
	cancel()
	<-stopped
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 1 || order[0] != "plane stopped, link deleted=true" {
		t.Fatalf("the plane must outlive Destroy and never see the daemon context cancelled: %v", order)
	}
}

func TestSupervisorBacksOffWhenInstancesDieImmediately(t *testing.T) {
	h := newHarness(t, nil)
	var mu sync.Mutex
	attached := 0
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		Supervisor{
			Socket: h.path, V6: ula, V4: pool, MTU: 1280, Warn: func(string, string) {}, Log: slog.New(slog.DiscardHandler),
			Backoff: 20 * time.Millisecond, Probe: time.Hour,
			Attach: func(_ context.Context, tun *os.File) (<-chan struct{}, func(), error) {
				mu.Lock()
				attached++
				mu.Unlock()
				done := make(chan struct{})
				close(done)
				return done, func() { tun.Close() }, nil
			},
			dial: func(ctx context.Context, path string) (*Client, error) {
				c, err := Dial(ctx, path)
				if c != nil {
					c.verify = func(*os.File, string) error { return nil }
				}
				return c, err
			},
		}.Run(ctx)
	}()
	time.Sleep(700 * time.Millisecond)
	cancel()
	<-stopped
	mu.Lock()
	defer mu.Unlock()
	if attached < 2 || attached > 7 {
		t.Fatalf("instances that die at once must not be recreated in a tight loop: %d attempts", attached)
	}
}

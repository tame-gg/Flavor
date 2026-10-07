package app_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/config"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/ipc/client"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

type daemon struct {
	paths  config.Paths
	client *client.Client
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
}

func testEnv(t *testing.T) config.Env {
	t.Helper()
	root := t.TempDir()
	run := filepath.Join(root, "run")
	if err := os.Mkdir(run, 0o700); err != nil {
		t.Fatal(err)
	}
	return config.Env{
		Home:       root,
		DataHome:   filepath.Join(root, "data"),
		ConfigHome: filepath.Join(root, "config"),
		RuntimeDir: run,
	}
}

func start(t *testing.T, env config.Env, opts app.Options) *daemon {
	t.Helper()
	ready := make(chan config.Paths, 1)
	ctx, cancel := context.WithCancel(context.Background())
	opts.Env = &env
	opts.SecretMode = secret.ModeMemory
	opts.OnReady = func(p config.Paths) { ready <- p }
	d := &daemon{cancel: cancel, done: make(chan error, 1)}
	go func() { d.done <- app.Run(ctx, opts) }()
	select {
	case d.paths = <-ready:
	case err := <-d.done:
		t.Fatalf("daemon exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon not ready")
	}
	d.client = client.New(d.paths.Socket)
	t.Cleanup(d.stop)
	return d
}

func (d *daemon) stop() {
	d.once.Do(func() {
		d.cancel()
		<-d.done
	})
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func latticeCode(err error) v1.LatticeErrorCode {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return v1.LatticeErrorCode_LATTICE_ERROR_CODE_UNSPECIFIED
	}
	for _, d := range ce.Details() {
		if m, err := d.Value(); err == nil {
			if detail, ok := m.(*v1.LatticeErrorDetail); ok {
				return detail.Code
			}
		}
	}
	return v1.LatticeErrorCode_LATTICE_ERROR_CODE_UNSPECIFIED
}

func snapshot(t *testing.T, c *client.Client) *v1.GetStateSnapshotResponse {
	t.Helper()
	res, err := c.Daemon.GetStateSnapshot(context.Background(), connect.NewRequest(&v1.GetStateSnapshotRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg
}

func networkState(t *testing.T, c *client.Client, id string) v1.NetworkConnectionState {
	res, err := c.Networks.GetNetwork(context.Background(), connect.NewRequest(&v1.GetNetworkRequest{NetworkId: id}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg.Network.State
}

func addHeadscale(t *testing.T, c *client.Client, name, url string, auto bool) *v1.Network {
	t.Helper()
	res, err := c.Networks.AddNetwork(context.Background(), connect.NewRequest(&v1.AddNetworkRequest{
		DisplayName: name, Provider: v1.ProviderType_PROVIDER_TYPE_HEADSCALE, ControlUrl: url, AutoConnect: auto,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg.Network
}

type collector struct {
	mu     sync.Mutex
	events []*v1.DaemonEvent
	err    error
	done   chan struct{}
}

func watch(t *testing.T, c *client.Client, instance string, after uint64) (*collector, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.Events.WatchEvents(ctx, connect.NewRequest(&v1.WatchEventsRequest{DaemonInstanceId: instance, AfterSequence: after}))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	col := &collector{done: make(chan struct{})}
	go func() {
		defer close(col.done)
		for stream.Receive() {
			col.mu.Lock()
			col.events = append(col.events, stream.Msg())
			col.mu.Unlock()
		}
		col.mu.Lock()
		col.err = stream.Err()
		col.mu.Unlock()
	}()
	return col, cancel
}

func (c *collector) snapshot() []*v1.DaemonEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*v1.DaemonEvent(nil), c.events...)
}

func TestDaemonEndToEnd(t *testing.T) {
	seq := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		e.SetStatus(sessiontest.StatusSelf("node-"+string(cfg.NetworkID), "100.64.0.1"))
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
	ctx := context.Background()
	c := d.client

	info, err := c.Daemon.GetDaemonInfo(ctx, connect.NewRequest(&v1.GetDaemonInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if info.Msg.ProtocolMajor != 1 || info.Msg.DaemonInstanceId == "" {
		t.Fatalf("%+v", info.Msg)
	}
	instance := info.Msg.DaemonInstanceId

	base := snapshot(t, c)
	if base.DaemonInstanceId != instance || len(base.Networks) != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", base)
	}
	col, stopWatch := watch(t, c, instance, base.SnapshotSequence)
	defer stopWatch()

	_, err = c.Networks.AddNetwork(ctx, connect.NewRequest(&v1.AddNetworkRequest{
		DisplayName: "Bad", Provider: v1.ProviderType_PROVIDER_TYPE_HEADSCALE, ControlUrl: "ftp://nope",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_INVALID_CONTROL_URL {
		t.Fatalf("invalid control url: %v", err)
	}

	a := addHeadscale(t, c, "Office", "https://a.example.com/", false)
	b := addHeadscale(t, c, "Office", "https://b.example.com", false)
	if a.ControlUrl != "https://a.example.com" || a.NodeHostname == "" {
		t.Fatalf("network not normalized: %+v", a)
	}
	for _, id := range []string{a.Id, b.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{a.Id, b.Id} {
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}
	_, err = c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: a.Id}))
	if latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_NETWORK_ALREADY_CONNECTED {
		t.Fatalf("connect while active: %v", err)
	}

	snap := snapshot(t, c)
	byNetwork := map[string]*v1.Device{}
	for _, dev := range snap.Devices {
		byNetwork[dev.Id.NetworkId] = dev
	}
	da, db := byNetwork[a.Id], byNetwork[b.Id]
	if da == nil || db == nil {
		t.Fatalf("devices missing from snapshot: %+v", snap.Devices)
	}
	if da.Addresses[0] != "100.64.0.1" || db.Addresses[0] != "100.64.0.1" {
		t.Fatal("expected duplicate tailnet address across networks")
	}
	if da.Id.NetworkId == db.Id.NetworkId || da.Id.NodeId == db.Id.NodeId {
		t.Fatal("duplicate-IP devices collapsed into one identity")
	}

	eventually(t, "peer events for both networks", func() bool {
		seen := map[string]bool{}
		for _, ev := range col.snapshot() {
			if p := ev.GetPeerAdded(); p != nil {
				seen[p.Device.Id.NetworkId] = true
			}
		}
		return seen[a.Id] && seen[b.Id]
	})
	evs := col.snapshot()
	for i, ev := range evs {
		if ev.DaemonInstanceId != instance {
			t.Fatal("event from wrong instance")
		}
		if want := base.SnapshotSequence + uint64(i) + 1; ev.SequenceId != want {
			t.Fatalf("sequence gap: got %d want %d", ev.SequenceId, want)
		}
	}

	for _, req := range []*v1.WatchEventsRequest{
		{DaemonInstanceId: "someone-else", AfterSequence: 0},
		{DaemonInstanceId: instance, AfterSequence: 1 << 40},
	} {
		stream, err := c.Events.WatchEvents(ctx, connect.NewRequest(req))
		if err == nil {
			for stream.Receive() {
			}
			err = stream.Err()
		}
		if latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_RESYNC_REQUIRED {
			t.Fatalf("expected resync for %+v, got %v", req, err)
		}
	}

	renamed := "Office (A)"
	upd, err := c.Networks.UpdateNetwork(ctx, connect.NewRequest(&v1.UpdateNetworkRequest{NetworkId: a.Id, DisplayName: &renamed}))
	if err != nil {
		t.Fatal(err)
	}
	if upd.Msg.Network.DisplayName != renamed || upd.Msg.Network.NodeHostname != a.NodeHostname {
		t.Fatalf("rename changed identity: %+v", upd.Msg.Network)
	}
	if upd.Msg.Network.State != v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED {
		t.Fatal("rename disturbed the session")
	}

	if _, err := c.Networks.DisconnectNetwork(ctx, connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: b.Id})); err != nil {
		t.Fatal(err)
	}
	if st := networkState(t, c, b.Id); st != v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISCONNECTED {
		t.Fatalf("after disconnect: %s", st)
	}
	if st := networkState(t, c, a.Id); st != v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED {
		t.Fatalf("disconnecting B affected A: %s", st)
	}

	dirA, err := d.paths.NetworkDir(domain.NetworkID(a.Id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Networks.RemoveNetwork(ctx, connect.NewRequest(&v1.RemoveNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Networks.GetNetwork(ctx, connect.NewRequest(&v1.GetNetworkRequest{NetworkId: a.Id})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("removed network still visible: %v", err)
	}
	if _, err := os.Stat(dirA); err != nil {
		t.Fatalf("soft remove must keep identity dir: %v", err)
	}
	if _, err := c.Networks.DeleteNetworkIdentity(ctx, connect.NewRequest(&v1.DeleteNetworkIdentityRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dirA); !os.IsNotExist(err) {
		t.Fatal("hard delete left identity dir")
	}
	if _, err := c.Networks.DeleteNetworkIdentity(ctx, connect.NewRequest(&v1.DeleteNetworkIdentityRequest{NetworkId: b.Id})); err != nil {
		t.Fatal(err)
	}
	if n := len(snapshot(t, c).Networks); n != 0 {
		t.Fatalf("networks left: %d", n)
	}
	if peak := seq.Peak(); peak != 2 {
		t.Fatalf("expected exactly two concurrent engines, peak=%d", peak)
	}

	d.stop()
	if _, err := os.Stat(d.paths.Socket); !os.IsNotExist(err) {
		t.Fatal("socket not removed on shutdown")
	}
	select {
	case <-col.done:
	case <-time.After(3 * time.Second):
		t.Fatal("event stream not terminated on shutdown")
	}
}

func TestSecondInstanceRefused(t *testing.T) {
	env := testEnv(t)
	start(t, env, app.Options{EngineFactory: (&sessiontest.Sequence{}).Factory})
	err := app.Run(context.Background(), app.Options{Env: &env, SecretMode: secret.ModeMemory})
	if !errors.Is(err, app.ErrAlreadyRunning) {
		t.Fatalf("got %v want ErrAlreadyRunning", err)
	}
}

func TestIPCReadyBeforeAutoConnect(t *testing.T) {
	env := testEnv(t)
	first := start(t, env, app.Options{EngineFactory: (&sessiontest.Sequence{}).Factory})
	n := addHeadscale(t, first.client, "Auto", "https://auto.example.com", true)
	first.stop()

	var release func()
	entered := make(chan struct{})
	seq := &sessiontest.Sequence{Prepare: func(_ int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		var in <-chan struct{}
		in, release = e.GateStart()
		go func() { <-in; close(entered) }()
	}}
	d := start(t, env, app.Options{EngineFactory: seq.Factory})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("auto-connect did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := d.client.Daemon.GetDaemonInfo(ctx, connect.NewRequest(&v1.GetDaemonInfoRequest{})); err != nil {
		t.Fatalf("ipc blocked by auto-connect: %v", err)
	}
	release()
	eventually(t, "auto-connected", func() bool {
		return networkState(t, d.client, n.Id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
	})
}

func TestEnrollmentKeyNeverPersistedOrExposed(t *testing.T) {
	const canary = "tskey-auth-LATTICECANARY0001"
	var logs bytes.Buffer
	var mu sync.Mutex
	var received []string
	seq := &sessiontest.Sequence{Prepare: func(_ int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		mu.Lock()
		received = append(received, e.AuthKey())
		mu.Unlock()
	}}
	env := testEnv(t)
	d := start(t, env, app.Options{EngineFactory: seq.Factory, Log: logging.New(&logs, slog.LevelDebug)})
	c := d.client
	ctx := context.Background()
	base := snapshot(t, c)
	col, stopWatch := watch(t, c, base.DaemonInstanceId, base.SnapshotSequence)
	defer stopWatch()

	n := addHeadscale(t, c, "Enroll", "https://hs.example.com", false)
	if _, err := c.Networks.EnrollNetwork(ctx, connect.NewRequest(&v1.EnrollNetworkRequest{
		NetworkId:  n.Id,
		Enrollment: &v1.EnrollmentCredential{Credential: &v1.EnrollmentCredential_PreAuthKey{PreAuthKey: canary}},
	})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "connected", func() bool {
		return networkState(t, c, n.Id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
	})
	mu.Lock()
	got := append([]string(nil), received...)
	mu.Unlock()
	if len(got) != 1 || got[0] != canary {
		t.Fatalf("engine did not receive enrollment key: %q", got)
	}
	if !seq.Engine(0).AuthKeyCleared() {
		t.Fatal("auth key not cleared after start")
	}

	diag, err := c.Diagnostics.RunDiagnostics(ctx, connect.NewRequest(&v1.RunDiagnosticsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	var dump strings.Builder
	dump.WriteString(logs.String())
	dump.WriteString(snapshot(t, c).String())
	dump.WriteString(diag.Msg.String())
	for _, ev := range col.snapshot() {
		dump.WriteString(ev.String())
	}
	d.stop()
	for _, root := range []string{d.paths.Data, d.paths.Config} {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.Mode().IsRegular() {
				if b, err := os.ReadFile(path); err == nil {
					dump.Write(b)
				}
			}
			return nil
		})
	}
	if strings.Contains(dump.String(), "LATTICECANARY") {
		t.Fatal("enrollment key leaked into logs, IPC responses, events, diagnostics or disk")
	}
}

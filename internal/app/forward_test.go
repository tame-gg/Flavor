package app_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/ipc/client"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

func identifyingEngines() *sessiontest.Sequence {
	return &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("pg-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "postgres"
		e.SetStatus(st)
		network := cfg.NetworkID
		e.SetDial(func(_ context.Context, _, address string) (net.Conn, error) {
			ours, theirs := net.Pipe()
			go func() {
				defer theirs.Close()
				fmt.Fprintf(theirs, "network=%s target=%s\n", network, address)
				_, _ = io.Copy(theirs, theirs)
			}()
			return ours, nil
		})
	}}
}

type forward struct {
	listen string
	events chan *v1.ForwardResponse
	errs   chan error
	cancel context.CancelFunc
}

func startForward(t *testing.T, c *client.Client, req *v1.ForwardRequest) (*forward, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.Forwards.Forward(ctx, connect.NewRequest(req))
	if err != nil {
		cancel()
		return nil, err
	}
	if !stream.Receive() {
		cancel()
		return nil, stream.Err()
	}
	f := &forward{listen: stream.Msg().GetStarted().GetListenAddress(), events: make(chan *v1.ForwardResponse, 64), errs: make(chan error, 1), cancel: cancel}
	go func() {
		for stream.Receive() {
			f.events <- stream.Msg()
		}
		f.errs <- stream.Err()
	}()
	t.Cleanup(cancel)
	return f, nil
}

func banner(t *testing.T, addr string) (string, net.Conn) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line), conn
}

func TestForwardThroughTheChosenNetwork(t *testing.T) {
	d := start(t, testEnv(t), app.Options{EngineFactory: identifyingEngines().Factory})
	c := d.client
	ctx := context.Background()
	a := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, c, "Home", "https://home.example.com", false)
	for _, id := range []string{a.Id, b.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}

	f, err := startForward(t, c, &v1.ForwardRequest{Destination: "postgres.home.lattice.internal:5432"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.listen, "127.0.0.1:") {
		t.Fatalf("default bind must be loopback: %q", f.listen)
	}
	got, conn := banner(t, f.listen)
	if got != "network="+b.Id+" target=100.64.0.9:5432" {
		t.Fatalf("forward went through the wrong network: %q", got)
	}
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	echo, _ := bufio.NewReader(conn).ReadString('\n')
	if echo != "ping\n" {
		t.Fatalf("echo %q", echo)
	}
	opened := <-f.events
	if opened.GetOpened().GetRoute().GetNetwork().GetId() != b.Id || opened.GetOpened().GetRoute().GetDecision().GetReason() != v1.DecisionReason_DECISION_REASON_NETWORK_QUALIFIED_NAME {
		t.Fatalf("%+v", opened)
	}

	f.cancel()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("open connection survived cancellation")
	}
	conn.Close()
	eventually(t, "listener closed", func() bool {
		c, err := net.DialTimeout("tcp", f.listen, 100*time.Millisecond)
		if err == nil {
			c.Close()
		}
		return err != nil
	})

	_, err = startForward(t, c, &v1.ForwardRequest{Destination: "100.64.0.9:5432"})
	if latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_DESTINATION_AMBIGUOUS {
		t.Fatalf("ambiguous raw address must not be forwarded: %v", err)
	}

	byName, err := startForward(t, c, &v1.ForwardRequest{Destination: "100.64.0.9:5432", Network: "LunarLabs"})
	if err != nil {
		t.Fatal(err)
	}
	if got, conn := banner(t, byName.listen); got != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("explicit network: %q", got)
	} else {
		conn.Close()
	}
	byName.cancel()

	if _, err := c.Preferences.SetDestinationPreference(ctx, connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: "100.64.0.9", NetworkId: b.Id})); err != nil {
		t.Fatal(err)
	}
	preferred, err := startForward(t, c, &v1.ForwardRequest{Destination: "100.64.0.9:5432", Listen: "[::1]:0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(preferred.listen, "[::1]:") {
		t.Fatalf("%q", preferred.listen)
	}
	if got, conn := banner(t, preferred.listen); got != "network="+b.Id+" target=100.64.0.9:5432" {
		t.Fatalf("preference: %q", got)
	} else {
		conn.Close()
	}

	for _, listen := range []string{"0.0.0.0:0", "192.168.1.10:8080", "[::]:0"} {
		if _, err := startForward(t, c, &v1.ForwardRequest{Destination: "postgres.home.lattice.internal:5432", Listen: listen}); latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_INVALID_ARGUMENT {
			t.Fatalf("%s accepted: %v", listen, err)
		}
	}
	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: "postgres.home.lattice.internal"}); latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("missing port: %v", err)
	}
	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: "10.1.2.3:80"}); latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_DESTINATION_NOT_FOUND {
		t.Fatalf("unknown destination: %v", err)
	}

	d.stop()
	select {
	case <-preferred.errs:
	case <-time.After(5 * time.Second):
		t.Fatal("forward stream not ended by daemon shutdown")
	}
}

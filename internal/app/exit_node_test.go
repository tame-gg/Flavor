package app_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/client"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
	"golang.org/x/net/proxy"
)

const exitNodeDestination = "203.0.113.7"

func exitNodeID(network string) string { return "gw-" + network }

func exitNodeEngines() *sessiontest.Sequence {
	return &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer(exitNodeID(string(cfg.NetworkID)), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "gateway"
		st.Peers[0].ExitNodeOption = true
		st.Peers = append(st.Peers, sessiontest.StatusWithPeer("plain-"+string(cfg.NetworkID), "100.64.0.10").Peers[0])
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

func exitNodeDaemon(t *testing.T) (*client.Client, *sessiontest.Sequence, *v1.Network, *v1.Network) {
	t.Helper()
	seq := exitNodeEngines()
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
	a := addHeadscale(t, d.client, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, d.client, "Home", "https://home.example.com", false)
	connectNetworks(t, d.client, a, b)
	return d.client, seq, a, b
}

func setExitNode(t *testing.T, c *client.Client, network *v1.Network) *v1.Device {
	t.Helper()
	res, err := c.ExitNodes.SetExitNode(context.Background(), connect.NewRequest(&v1.SetExitNodeRequest{NetworkId: network.Id, NodeId: exitNodeID(network.Id)}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg.Device
}

func clearExitNode(t *testing.T, c *client.Client, network *v1.Network) {
	t.Helper()
	if _, err := c.ExitNodes.ClearExitNode(context.Background(), connect.NewRequest(&v1.ClearExitNodeRequest{NetworkId: network.Id})); err != nil {
		t.Fatal(err)
	}
}

func startProxy(t *testing.T, c *client.Client) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := c.Forwards.Proxy(ctx, connect.NewRequest(&v1.ProxyRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() {
		t.Fatal(stream.Err())
	}
	go func() {
		for stream.Receive() {
		}
	}()
	return stream.Msg().GetStarted().GetListenAddress()
}

func socksBanner(t *testing.T, addr, dest string) string {
	t.Helper()
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := dialer.Dial("tcp", dest)
	if err != nil {
		t.Fatalf("%s: %v", dest, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return line[:len(line)-1]
}

func TestExitNodeSelectionOverIPC(t *testing.T) {
	c, seq, a, _ := exitNodeDaemon(t)

	if r := inspectDestination(t, c, exitNodeDestination); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH {
		t.Fatalf("no exit node selected yet: %+v", r)
	}

	dev := setExitNode(t, c, a)
	if !dev.ExitNode || !dev.ExitNodeOption || dev.Hostname != "gateway" {
		t.Fatalf("%+v", dev)
	}
	if got := seq.Engine(0).ExitNode(); string(got) != exitNodeID(a.Id) {
		t.Fatalf("engine selection %q", got)
	}
	devices, err := c.Devices.ListDevices(context.Background(), connect.NewRequest(&v1.ListDevicesRequest{NetworkId: a.Id}))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devices.Msg.Devices {
		if want := d.Id.NodeId == exitNodeID(a.Id); d.ExitNode != want || d.ExitNodeOption != want {
			t.Fatalf("%+v", d)
		}
	}

	for _, dest := range []string{exitNodeDestination, exitNodeDestination + ":443", "example.com:443"} {
		r := inspectDestination(t, c, dest)
		if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_EXIT_NODE || r.DecidedBy != v1.MatchKind_MATCH_KIND_EXIT_NODE || len(r.Candidates) != 1 {
			t.Fatalf("%s: %+v", dest, r)
		}
		cand := r.Candidates[0]
		if cand.Network.Id != a.Id || cand.Match != v1.MatchKind_MATCH_KIND_EXIT_NODE || cand.Device.Hostname != "gateway" || cand.Status != v1.CandidateStatus_CANDIDATE_STATUS_SELECTED {
			t.Fatalf("%s: %+v", dest, cand)
		}
	}

	for _, dest := range []string{"100.64.200.1", "fd7a:115c:a1e0::5", "ghost.home.flavor.internal"} {
		if r := inspectDestination(t, c, dest); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH {
			t.Fatalf("%s must not use the exit node: %+v", dest, r)
		}
	}

	clearExitNode(t, c, a)
	if got := seq.Engine(0).ExitNode(); got != "" {
		t.Fatalf("engine selection %q", got)
	}
	if r := inspectDestination(t, c, exitNodeDestination); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH || len(r.Candidates) != 0 {
		t.Fatalf("cleared exit node still resolves: %+v", r)
	}
}

func TestExitNodeRejectsInvalidSelections(t *testing.T) {
	c, seq, a, _ := exitNodeDaemon(t)
	set := func(network, node string) error {
		_, err := c.ExitNodes.SetExitNode(context.Background(), connect.NewRequest(&v1.SetExitNodeRequest{NetworkId: network, NodeId: node}))
		return err
	}

	if err := set(a.Id, "plain-"+a.Id); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NOT_AN_EXIT_NODE {
		t.Fatalf("device without the exit node option: %v", err)
	}
	if err := set(a.Id, "self-"+a.Id); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NOT_AN_EXIT_NODE {
		t.Fatalf("local device: %v", err)
	}
	if err := set(a.Id, "missing"); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DEVICE_NOT_FOUND {
		t.Fatalf("unknown device: %v", err)
	}
	if err := set(a.Id, ""); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("empty device: %v", err)
	}
	if err := set(string(domain.NewNetworkID()), "x"); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("unknown network: %v", err)
	}
	if calls := seq.Engine(0).ExitNodeCalls(); len(calls) != 0 {
		t.Fatalf("rejected selections reached the engine: %v", calls)
	}

	if _, err := c.Networks.DisconnectNetwork(context.Background(), connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if err := set(a.Id, exitNodeID(a.Id)); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_UNREACHABLE {
		t.Fatalf("disconnected network: %v", err)
	}
}

func TestExitNodeForwardDialsTheOriginalAddress(t *testing.T) {
	c, _, a, _ := exitNodeDaemon(t)
	setExitNode(t, c, a)

	for _, dest := range []string{exitNodeDestination + ":443", "example.com:443"} {
		f, err := startForward(t, c, &v1.ForwardRequest{Destination: dest})
		if err != nil {
			t.Fatalf("%s: %v", dest, err)
		}
		got, conn := banner(t, f.listen)
		if want := "network=" + a.Id + " target=" + dest; got != want {
			t.Fatalf("got %q want %q", got, want)
		}
		if _, err := conn.Write([]byte("ping\n")); err != nil {
			t.Fatal(err)
		}
		if echo, _ := bufio.NewReader(conn).ReadString('\n'); echo != "ping\n" {
			t.Fatalf("echo %q", echo)
		}
		conn.Close()
		opened := <-f.events
		route := opened.GetOpened().GetRoute()
		if route.GetDecision().GetReason() != v1.DecisionReason_DECISION_REASON_EXIT_NODE || route.GetTarget() != dest || route.GetNetwork().GetId() != a.Id {
			t.Fatalf("%s: %+v", dest, opened)
		}
	}

	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: exitNodeDestination}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("forward without a port: %v", err)
	}
}

func TestExitNodeSocksDialsTheOriginalAddress(t *testing.T) {
	c, _, a, _ := exitNodeDaemon(t)
	setExitNode(t, c, a)
	addr := startProxy(t, c)

	for _, dest := range []string{exitNodeDestination + ":443", "example.com:443"} {
		if got, want := socksBanner(t, addr, dest), "network="+a.Id+" target="+dest; got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}

	clearExitNode(t, c, a)
	if code := socksReplyCode(t, addr, domainRequest(1, "example.com", 443)); code != 0x04 {
		t.Fatalf("without an exit node the destination is unreachable, got reply %#x", code)
	}
}

func TestExitNodeOnTwoNetworksIsAmbiguous(t *testing.T) {
	c, _, a, b := exitNodeDaemon(t)
	setExitNode(t, c, a)
	setExitNode(t, c, b)

	r := inspectDestination(t, c, exitNodeDestination)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS || r.Reason != v1.DecisionReason_DECISION_REASON_MULTIPLE_MATCHES || r.DecidedBy != v1.MatchKind_MATCH_KIND_EXIT_NODE || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: exitNodeDestination + ":443"}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_AMBIGUOUS {
		t.Fatalf("ambiguous exit nodes must not be forwarded: %v", err)
	}
	addr := startProxy(t, c)
	if code := socksReplyCode(t, addr, domainRequest(1, "example.com", 443)); code != 0x02 {
		t.Fatalf("ambiguous exit nodes must be refused by policy, got reply %#x", code)
	}

	f, err := startForward(t, c, &v1.ForwardRequest{Destination: exitNodeDestination + ":443", Network: "Home"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn := banner(t, f.listen)
	conn.Close()
	if want := "network=" + b.Id + " target=" + exitNodeDestination + ":443"; got != want {
		t.Fatalf("explicit network: got %q want %q", got, want)
	}

	if _, err := c.Preferences.SetDestinationPreference(context.Background(), connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: exitNodeDestination, NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	r = inspectDestination(t, c, exitNodeDestination)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_DESTINATION_PREFERENCE {
		t.Fatalf("%+v", r)
	}
	pref, err := startForward(t, c, &v1.ForwardRequest{Destination: exitNodeDestination + ":443"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn = banner(t, pref.listen)
	conn.Close()
	if want := "network=" + a.Id + " target=" + exitNodeDestination + ":443"; got != want {
		t.Fatalf("preference: got %q want %q", got, want)
	}

	clearExitNode(t, c, a)
	r = inspectDestination(t, c, exitNodeDestination)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Candidates[0].Network.Id != b.Id {
		t.Fatalf("%+v", r)
	}
}

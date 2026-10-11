package app_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
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

const dnsRecordName = "git.intra.example.internal"

func dnsRecord(name string, addrs ...string) domain.DNSRecord {
	r := domain.DNSRecord{Name: name}
	for _, a := range addrs {
		r.Addresses = append(r.Addresses, netip.MustParseAddr(a))
	}
	return r
}

func dnsRecordEngines(records map[int][]domain.DNSRecord) *sessiontest.Sequence {
	return &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("edge-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "edge"
		st.DNSRecords = records[i]
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

func connectNetworks(t *testing.T, c *client.Client, networks ...*v1.Network) {
	t.Helper()
	for _, n := range networks {
		if _, err := c.Networks.ConnectNetwork(context.Background(), connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: n.Id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, n.Id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}
}

func inspectDestination(t *testing.T, c *client.Client, dest string) *v1.InspectDestinationResponse {
	t.Helper()
	res, err := c.Inspector.InspectDestination(context.Background(), connect.NewRequest(&v1.InspectDestinationRequest{Destination: dest}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg
}

func twoNetworkDaemon(t *testing.T, records map[int][]domain.DNSRecord) (*client.Client, *v1.Network, *v1.Network) {
	t.Helper()
	d := start(t, testEnv(t), app.Options{EngineFactory: dnsRecordEngines(records).Factory})
	a := addHeadscale(t, d.client, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, d.client, "Home", "https://home.example.com", false)
	connectNetworks(t, d.client, a, b)
	return d.client, a, b
}

func TestDNSRecordInspectOverIPC(t *testing.T) {
	c, a, _ := twoNetworkDaemon(t, map[int][]domain.DNSRecord{0: {dnsRecord(dnsRecordName, "100.64.0.9")}})

	r := inspectDestination(t, c, dnsRecordName)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_DNS_RECORD || r.DecidedBy != v1.MatchKind_MATCH_KIND_DNS_RECORD {
		t.Fatalf("%+v", r)
	}
	if len(r.Candidates) != 1 {
		t.Fatalf("%+v", r.Candidates)
	}
	cand := r.Candidates[0]
	if cand.Network.Id != a.Id || cand.MatchedValue != "100.64.0.9" || cand.Match != v1.MatchKind_MATCH_KIND_DNS_RECORD || cand.Device.Hostname != "edge" {
		t.Fatalf("%+v", cand)
	}

	for _, dest := range []string{"git", "git.intra", "git.lunarlabs.flavor.internal", "git.home.flavor.internal"} {
		if r := inspectDestination(t, c, dest); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH {
			t.Fatalf("%s must not match the record: %+v", dest, r)
		}
	}
}

func TestDNSRecordAmbiguousAndPreference(t *testing.T) {
	c, a, _ := twoNetworkDaemon(t, map[int][]domain.DNSRecord{
		0: {dnsRecord(dnsRecordName, "100.64.0.9")},
		1: {dnsRecord(dnsRecordName, "192.0.2.10")},
	})

	r := inspectDestination(t, c, dnsRecordName)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS || r.Reason != v1.DecisionReason_DECISION_REASON_MULTIPLE_MATCHES || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}

	if _, err := c.Preferences.SetDestinationPreference(context.Background(), connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: dnsRecordName, NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	r = inspectDestination(t, c, dnsRecordName)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_DESTINATION_PREFERENCE {
		t.Fatalf("%+v", r)
	}
	for _, cand := range r.Candidates {
		if (cand.Status == v1.CandidateStatus_CANDIDATE_STATUS_SELECTED) != (cand.Network.Id == a.Id) {
			t.Fatalf("preferred network must be the selected candidate: %+v", cand)
		}
	}
}

func TestDNSRecordForwardDialsRecordAddress(t *testing.T) {
	c, a, b := twoNetworkDaemon(t, map[int][]domain.DNSRecord{
		0: {dnsRecord(dnsRecordName, "100.64.0.9"), dnsRecord("db.intra.example.internal", "192.0.2.10")},
		1: {dnsRecord("shared.intra.example.internal", "192.0.2.20")},
	})

	f, err := startForward(t, c, &v1.ForwardRequest{Destination: dnsRecordName + ":5432"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn := banner(t, f.listen)
	defer conn.Close()
	if got != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("record with owner device: %q", got)
	}
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	if echo, _ := bufio.NewReader(conn).ReadString('\n'); echo != "ping\n" {
		t.Fatalf("echo %q", echo)
	}
	if opened := <-f.events; opened.GetOpened().GetRoute().GetDecision().GetReason() != v1.DecisionReason_DECISION_REASON_DNS_RECORD {
		t.Fatalf("%+v", opened)
	}

	orphan, err := startForward(t, c, &v1.ForwardRequest{Destination: "db.intra.example.internal:5432"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn2 := banner(t, orphan.listen)
	conn2.Close()
	if got != "network="+a.Id+" target=192.0.2.10:5432" {
		t.Fatalf("record without owner device: %q", got)
	}

	other, err := startForward(t, c, &v1.ForwardRequest{Destination: "shared.intra.example.internal:80"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn3 := banner(t, other.listen)
	conn3.Close()
	if got != "network="+b.Id+" target=192.0.2.20:80" {
		t.Fatalf("record on the second network: %q", got)
	}
}

func TestDNSRecordForwardAmbiguousNeedsNetwork(t *testing.T) {
	c, a, b := twoNetworkDaemon(t, map[int][]domain.DNSRecord{
		0: {dnsRecord(dnsRecordName, "100.64.0.9")},
		1: {dnsRecord(dnsRecordName, "192.0.2.10")},
	})

	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: dnsRecordName + ":5432"}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_AMBIGUOUS {
		t.Fatalf("ambiguous record must not be forwarded: %v", err)
	}

	f, err := startForward(t, c, &v1.ForwardRequest{Destination: dnsRecordName + ":5432", Network: "Home"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn := banner(t, f.listen)
	conn.Close()
	if got != "network="+b.Id+" target=192.0.2.10:5432" {
		t.Fatalf("explicit network: %q", got)
	}

	if _, err := c.Preferences.SetDestinationPreference(context.Background(), connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: dnsRecordName, NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	pref, err := startForward(t, c, &v1.ForwardRequest{Destination: dnsRecordName + ":5432"})
	if err != nil {
		t.Fatal(err)
	}
	got, conn = banner(t, pref.listen)
	conn.Close()
	if got != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("preference: %q", got)
	}
}

func TestDNSRecordSocks(t *testing.T) {
	c, a, _ := twoNetworkDaemon(t, map[int][]domain.DNSRecord{
		0: {dnsRecord(dnsRecordName, "100.64.0.9"), dnsRecord("db.intra.example.internal", "192.0.2.10")},
	})

	sctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := c.Forwards.Proxy(sctx, connect.NewRequest(&v1.ProxyRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() {
		t.Fatal(stream.Err())
	}
	addr := stream.Msg().GetStarted().GetListenAddress()
	go func() {
		for stream.Receive() {
		}
	}()
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	read := func(dest string) string {
		t.Helper()
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

	if got := read(dnsRecordName + ":5432"); got != "network="+a.Id+" target=100.64.0.9:5432" {
		t.Fatalf("record with owner device: %q", got)
	}
	if got := read("db.intra.example.internal:5432"); got != "network="+a.Id+" target=192.0.2.10:5432" {
		t.Fatalf("record without owner device: %q", got)
	}
}

func TestDNSRecordSocksRefusesAmbiguousName(t *testing.T) {
	c, _, _ := twoNetworkDaemon(t, map[int][]domain.DNSRecord{
		0: {dnsRecord(dnsRecordName, "100.64.0.9")},
		1: {dnsRecord(dnsRecordName, "192.0.2.10")},
	})
	sctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := c.Forwards.Proxy(sctx, connect.NewRequest(&v1.ProxyRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() {
		t.Fatal(stream.Err())
	}
	addr := stream.Msg().GetStarted().GetListenAddress()
	go func() {
		for stream.Receive() {
		}
	}()
	if code := socksReplyCode(t, addr, domainRequest(1, dnsRecordName, 5432)); code != 0x02 {
		t.Fatalf("ambiguous record must be refused by policy, got reply %#x", code)
	}
}

func TestDNSRecordDisappearsOnDisconnect(t *testing.T) {
	c, a, _ := twoNetworkDaemon(t, map[int][]domain.DNSRecord{0: {dnsRecord(dnsRecordName, "100.64.0.9")}})

	if r := inspectDestination(t, c, dnsRecordName); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE {
		t.Fatalf("%+v", r)
	}
	if _, err := c.Networks.DisconnectNetwork(context.Background(), connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	r := inspectDestination(t, c, dnsRecordName)
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH || len(r.Candidates) != 0 {
		t.Fatalf("record outlived its network: %+v", r)
	}
	if _, err := startForward(t, c, &v1.ForwardRequest{Destination: dnsRecordName + ":5432"}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_NOT_FOUND {
		t.Fatalf("forward to a vanished record: %v", err)
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"

	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
)

func device(network, node, host, dns string, option, selected bool) *v1.Device {
	return &v1.Device{
		Id:             &v1.DeviceIdentity{NetworkId: network, NodeId: node},
		Hostname:       host,
		DnsName:        dns,
		Addresses:      []string{"100.64.0.9"},
		Online:         true,
		ExitNodeOption: option,
		ExitNode:       selected,
	}
}

func TestFindNetwork(t *testing.T) {
	nets := []*v1.Network{
		{Id: "n1", DisplayName: "Home"},
		{Id: "n2", DisplayName: "Work"},
		{Id: "n3", DisplayName: "Work"},
	}
	if n, err := findNetwork(nets, "n1"); err != nil || n.Id != "n1" {
		t.Fatalf("by id: %v %v", n, err)
	}
	if n, err := findNetwork(nets, "Home"); err != nil || n.Id != "n1" {
		t.Fatalf("by name: %v %v", n, err)
	}
	if n, err := findNetwork(nets, "n3"); err != nil || n.Id != "n3" {
		t.Fatalf("duplicate name by id: %v %v", n, err)
	}
	if _, err := findNetwork(nets, "Work"); err == nil || !strings.Contains(err.Error(), "n2, n3") {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := findNetwork(nets, "nope"); err == nil {
		t.Fatal("unknown network accepted")
	}
}

func TestFindExitNode(t *testing.T) {
	options := exitNodeOptions([]*v1.Device{
		device("n1", "a", "gateway", "gateway.home.ts.net.", true, false),
		device("n1", "b", "laptop", "laptop.home.ts.net.", false, false),
		device("n1", "c", "relay", "relay.home.ts.net.", true, true),
		device("n1", "d", "relay", "relay2.home.ts.net.", true, false),
	})
	if len(options) != 3 {
		t.Fatalf("options: %d", len(options))
	}
	for _, ref := range []string{"gateway", "GATEWAY", "gateway.home.ts.net", "gateway.home.ts.net.", "a"} {
		if d, err := findExitNode(options, ref, "Home"); err != nil || d.Id.NodeId != "a" {
			t.Fatalf("%q: %v %v", ref, d, err)
		}
	}
	if _, err := findExitNode(options, "laptop", "Home"); err == nil {
		t.Fatal("device without the option accepted")
	}
	_, err := findExitNode(options, "relay", "Home")
	if err == nil || !strings.Contains(err.Error(), "c  relay") || !strings.Contains(err.Error(), "d  relay") {
		t.Fatalf("ambiguous hostname: %v", err)
	}
	if d, err := findExitNode(options, "relay2.home.ts.net", "Home"); err != nil || d.Id.NodeId != "d" {
		t.Fatalf("dns name: %v %v", d, err)
	}
}

func TestExitNodeState(t *testing.T) {
	if got := exitNodeState(device("n", "a", "h", "", false, false)); got != "" {
		t.Fatalf("plain device: %q", got)
	}
	if got := exitNodeState(device("n", "a", "h", "", true, false)); got != "yes" {
		t.Fatalf("option: %q", got)
	}
	if got := exitNodeState(device("n", "a", "h", "", true, true)); got != "selected" {
		t.Fatalf("selected: %q", got)
	}
}

func TestPrintExitNodes(t *testing.T) {
	var out bytes.Buffer
	if err := printExitNodes(&out, nil, nil); err != nil || !strings.Contains(out.String(), "no exit nodes") {
		t.Fatalf("empty: %q %v", out.String(), err)
	}
	out.Reset()
	devices := []*v1.Device{
		device("n1", "a", "gateway", "", true, true),
		device("n2", "b", "relay", "", true, false),
	}
	if err := printExitNodes(&out, devices, map[string]string{"n1": "Home", "n2": "Work"}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || strings.Fields(lines[0])[0] != "NETWORK" {
		t.Fatalf("table: %q", out.String())
	}
	if f := strings.Fields(lines[1]); f[0] != "Home" || f[1] != "gateway" || f[len(f)-1] != "yes" {
		t.Fatalf("selected row: %q", lines[1])
	}
	if f := strings.Fields(lines[2]); f[0] != "Work" || f[len(f)-1] != "true" {
		t.Fatalf("other row: %q", lines[2])
	}
}

func TestExplainExitNode(t *testing.T) {
	r := &v1.InspectDestinationResponse{
		Normalized: "203.0.113.7",
		Kind:       v1.DestinationKind_DESTINATION_KIND_ADDRESS,
		Decision:   v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE,
		Reason:     v1.DecisionReason_DECISION_REASON_EXIT_NODE,
		Candidates: []*v1.ResolutionCandidate{{
			Network: &v1.NetworkRef{Id: "n1", DisplayName: "Home"},
			Device:  device("n1", "a", "gateway", "", true, true),
			Match:   v1.MatchKind_MATCH_KIND_EXIT_NODE,
			Status:  v1.CandidateStatus_CANDIDATE_STATUS_SELECTED,
		}},
	}
	var out bytes.Buffer
	if err := printExplain(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exit node", "gateway on Home"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestParseSwitch(t *testing.T) {
	if on, err := parseSwitch("on"); err != nil || !on {
		t.Fatalf("on: %v %v", on, err)
	}
	if on, err := parseSwitch("off"); err != nil || on {
		t.Fatalf("off: %v %v", on, err)
	}
	if _, err := parseSwitch("yes"); err == nil {
		t.Fatal("yes accepted")
	}
}

func TestConnectedNetworks(t *testing.T) {
	nets := []*v1.Network{
		{Id: "n1", State: v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED},
		{Id: "n2", State: v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISCONNECTED},
		{Id: "n3", State: v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DEGRADED},
	}
	got := connectedNetworks(nets)
	if len(got) != 2 || got[0].Id != "n1" || got[1].Id != "n3" {
		t.Fatalf("connected: %v", got)
	}
}

func TestApprovalText(t *testing.T) {
	if got := approvalText(true); got != "approved" {
		t.Fatalf("approved: %q", got)
	}
	if got := approvalText(false); got != "waiting for approval" {
		t.Fatalf("pending: %q", got)
	}
}

func TestPrintRoutes(t *testing.T) {
	var out bytes.Buffer
	if err := printRoutes(&out, nil); err != nil || !strings.Contains(out.String(), "no routes advertised") {
		t.Fatalf("empty: %q %v", out.String(), err)
	}
	out.Reset()
	rows := routeRows("Home", []*v1.AdvertisedRoute{{Prefix: "192.168.1.0/24", Approved: true}, {Prefix: "10.0.0.0/8"}}, true, false)
	if err := printRoutes(&out, rows); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || strings.Fields(lines[0])[0] != "NETWORK" {
		t.Fatalf("table: %q", out.String())
	}
	if f := strings.Fields(lines[1]); f[0] != "Home" || f[1] != "192.168.1.0/24" || f[2] != "approved" {
		t.Fatalf("approved row: %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], "waiting for approval") {
		t.Fatalf("pending row: %q", lines[2])
	}
	if f := strings.Fields(lines[3]); f[1] != "exit" || f[2] != "node" || !strings.HasSuffix(lines[3], "waiting for approval") {
		t.Fatalf("exit node row: %q", lines[3])
	}
}

func TestAdvertiseSummary(t *testing.T) {
	routes := []*v1.AdvertisedRoute{{Prefix: "192.168.1.0/24"}, {Prefix: "10.0.0.0/8", Approved: true}}
	if got := advertiseSummary("192.168.1.7/24", "Home", routes); !strings.Contains(got, "192.168.1.7/24 on Home") || !strings.Contains(got, "waiting for approval on the control server") {
		t.Fatalf("masked pending: %q", got)
	}
	if got := advertiseSummary("10.0.0.0/8", "Home", routes); !strings.Contains(got, "already approved") {
		t.Fatalf("approved: %q", got)
	}
}

func TestExitNodeSummary(t *testing.T) {
	if got := exitNodeSummary("Home", true, false); !strings.Contains(got, "waiting for approval on the control server") {
		t.Fatalf("pending: %q", got)
	}
	if got := exitNodeSummary("Home", true, true); !strings.Contains(got, "approved exit node on Home") {
		t.Fatalf("approved: %q", got)
	}
	if got := exitNodeSummary("Home", false, false); !strings.Contains(got, "no longer offers") {
		t.Fatalf("off: %q", got)
	}
}

package app_test

import (
	"context"
	"net/netip"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

func TestSubnetRoutesAndQualifiedNamesOverIPC(t *testing.T) {
	routes := map[int][]string{0: {"10.0.0.0/8"}, 1: {"10.20.0.0/16"}}
	seq := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("router-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "edge"
		for _, r := range routes[i] {
			st.Peers[0].Routes = append(st.Peers[0].Routes, netip.MustParsePrefix(r))
		}
		e.SetStatus(st)
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
	c := d.client
	ctx := context.Background()
	wide := addHeadscale(t, c, "Company HQ", "https://hq.example.com", false)
	narrow := addHeadscale(t, c, "Customer", "https://cust.example.com", false)
	if wide.Label != "company-hq" {
		t.Fatalf("label %q", wide.Label)
	}
	for _, id := range []string{wide.Id, narrow.Id} {
		if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: id})); err != nil {
			t.Fatal(err)
		}
		eventually(t, "connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}
	inspect := func(dest string) *v1.InspectDestinationResponse {
		t.Helper()
		res, err := c.Inspector.InspectDestination(ctx, connect.NewRequest(&v1.InspectDestinationRequest{Destination: dest}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}

	r := inspect("10.20.5.12")
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_LONGEST_PREFIX {
		t.Fatalf("%+v", r)
	}
	if r.Candidates[0].Network.Id != narrow.Id || r.Candidates[0].MatchedValue != "10.20.0.0/16" || r.Candidates[0].Match != v1.MatchKind_MATCH_KIND_SUBNET_ROUTE {
		t.Fatalf("%+v", r.Candidates[0])
	}
	if r.Candidates[0].QualifiedName != "edge.customer.lattice.internal" {
		t.Fatalf("qualified name %q", r.Candidates[0].QualifiedName)
	}

	q := inspect("edge.company-hq.lattice.internal")
	if q.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || q.Reason != v1.DecisionReason_DECISION_REASON_NETWORK_QUALIFIED_NAME || q.Candidates[0].Network.Id != wide.Id {
		t.Fatalf("%+v", q)
	}
	desc, err := c.Inspector.DescribeDevice(ctx, connect.NewRequest(&v1.DescribeDeviceRequest{NetworkId: narrow.Id, NodeId: r.Candidates[0].Device.Id.NodeId}))
	if err != nil {
		t.Fatal(err)
	}
	if desc.Msg.Name != "edge.customer.lattice.internal" || desc.Msg.StableName != r.Candidates[0].StableName || desc.Msg.StableName == "" {
		t.Fatalf("%+v", desc.Msg)
	}
	if _, err := c.Inspector.DescribeDevice(ctx, connect.NewRequest(&v1.DescribeDeviceRequest{NetworkId: narrow.Id, NodeId: "nope"})); latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_DEVICE_NOT_FOUND {
		t.Fatalf("missing device: %v", err)
	}
	stable := inspect(r.Candidates[0].StableName)
	if stable.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || stable.Candidates[0].Network.Id != narrow.Id {
		t.Fatalf("stable name must resolve: %+v", stable)
	}
	if amb := inspect("edge"); amb.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS {
		t.Fatalf("bare name on two networks: %+v", amb)
	}

	res, err := c.Conflicts.ListConflicts(ctx, connect.NewRequest(&v1.ListConflictsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	var overlap *v1.Conflict
	for _, cf := range res.Msg.Conflicts {
		if cf.Id == "overlap:10.20.0.0/16" {
			overlap = cf
		}
	}
	if overlap == nil || overlap.Type != v1.ConflictType_CONFLICT_TYPE_SUBNET_OVERLAP || overlap.SampleAddress != "10.20.0.1" || overlap.Severity != v1.ConflictSeverity_CONFLICT_SEVERITY_EXPECTED {
		t.Fatalf("%+v", res.Msg.Conflicts)
	}
	for _, m := range overlap.Members {
		if m.Route == "" || len(m.Device.Routes) == 0 {
			t.Fatalf("member without route: %+v", m)
		}
	}
}

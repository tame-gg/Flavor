package app_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/provider"
	"git.lunarlabs.dev/lattice/lattice/internal/session/sessiontest"
)

func TestInspectDestinationAcrossNetworks(t *testing.T) {
	seq := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusSelf("self-"+string(cfg.NetworkID), "100.64.0.1")
		st.Self.Hostname = []string{"prod-api", "desktop"}[i]
		e.SetStatus(st)
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
	c := d.client
	ctx := context.Background()

	info, err := c.Daemon.GetDaemonInfo(ctx, connect.NewRequest(&v1.GetDaemonInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range info.Msg.Capabilities {
		found = found || k == v1.Capability_CAPABILITY_CONNECTION_INSPECTOR
	}
	if !found {
		t.Fatal("inspector capability not advertised")
	}

	a := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	b := addHeadscale(t, c, "Home", "https://home.example.com", false)
	off := addHeadscale(t, c, "Work", "https://work.example.com", false)
	for _, id := range []string{a.Id, b.Id} {
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

	amb := inspect("100.64.0.1")
	if amb.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS || amb.Reason != v1.DecisionReason_DECISION_REASON_MULTIPLE_MATCHES || len(amb.Candidates) != 2 {
		t.Fatalf("%+v", amb)
	}
	if amb.Candidates[0].Network.DisplayName != "Home" || amb.Candidates[1].Network.DisplayName != "LunarLabs" {
		t.Fatal("candidates not ordered by network name")
	}
	if amb.Candidates[0].Device.Id.NetworkId == amb.Candidates[1].Device.Id.NetworkId {
		t.Fatal("candidates collapsed into one network")
	}
	if len(amb.NotInspected) != 1 || amb.NotInspected[0].Id != off.Id {
		t.Fatalf("disconnected network not reported: %+v", amb.NotInspected)
	}

	uniq := inspect("prod-api:443")
	if uniq.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || uniq.Reason != v1.DecisionReason_DECISION_REASON_DEVICE_HOSTNAME || uniq.Port != 443 {
		t.Fatalf("%+v", uniq)
	}
	if uniq.Candidates[0].Status != v1.CandidateStatus_CANDIDATE_STATUS_SELECTED || uniq.Candidates[0].Network.Id != a.Id {
		t.Fatalf("%+v", uniq.Candidates[0])
	}

	if none := inspect("10.10.20.15"); none.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH || len(none.Candidates) != 0 {
		t.Fatalf("%+v", none)
	}

	_, err = c.Inspector.InspectDestination(ctx, connect.NewRequest(&v1.InspectDestinationRequest{Destination: "https://nope/"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || latticeCode(err) != v1.LatticeErrorCode_LATTICE_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("malformed destination: %v", err)
	}
}

func TestListConflictsOverIPC(t *testing.T) {
	seq := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("peer-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Self.DNSName = "self." + string(cfg.NetworkID) + ".test"
		st.Peers[0].Hostname = "postgres"
		st.Peers[0].DNSName = "postgres." + string(cfg.NetworkID) + ".test"
		e.SetStatus(st)
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
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
	res, err := c.Conflicts.ListConflicts(ctx, connect.NewRequest(&v1.ListConflictsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*v1.Conflict{}
	for _, cf := range res.Msg.Conflicts {
		got[cf.Id] = cf
	}
	addr := got["address:100.64.0.9"]
	if addr == nil || addr.Severity != v1.ConflictSeverity_CONFLICT_SEVERITY_EXPECTED || !addr.NetworkContextResolves || len(addr.Members) != 2 {
		t.Fatalf("peer address overlap: %+v", res.Msg.Conflicts)
	}
	if got["name:postgres"] == nil {
		t.Fatal("hostname collision missing")
	}
	if got["address:100.64.0.1"] != nil {
		t.Fatal("this machine's own address on two networks reported as a conflict")
	}
	for _, m := range addr.Members {
		if m.Device.Local || m.UniqueName == "" {
			t.Fatalf("%+v", m)
		}
	}
}

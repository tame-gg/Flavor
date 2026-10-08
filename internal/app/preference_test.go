package app_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
)

func TestDestinationPreferencesEndToEnd(t *testing.T) {
	seq := &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("peer-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].DNSName = "peer." + string(cfg.NetworkID) + ".test"
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
	inspect := func() *v1.InspectDestinationResponse {
		t.Helper()
		res, err := c.Inspector.InspectDestination(ctx, connect.NewRequest(&v1.InspectDestinationRequest{Destination: "100.64.0.9"}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	set := func(dest, net string) error {
		_, err := c.Preferences.SetDestinationPreference(ctx, connect.NewRequest(&v1.SetDestinationPreferenceRequest{Destination: dest, NetworkId: net}))
		return err
	}

	if r := inspect(); r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS || r.Preference != nil {
		t.Fatalf("%+v", r)
	}
	if err := set("100.64.0.9", "01NOSUCHNETWORK000000000000"); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("invalid network: %v", err)
	}
	if err := set("10.0.0.0/8", a.Id); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("invalid destination: %v", err)
	}
	if err := set("100.64.0.9:443", a.Id); err != nil {
		t.Fatal(err)
	}

	r := inspect()
	if r.Decision != v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE || r.Reason != v1.DecisionReason_DECISION_REASON_DESTINATION_PREFERENCE {
		t.Fatalf("%+v", r)
	}
	if r.Preference.GetState() != v1.PreferenceState_PREFERENCE_STATE_APPLIED || r.Preference.Network.GetDisplayName() != "LunarLabs" || r.Preference.Destination != "100.64.0.9" {
		t.Fatalf("%+v", r.Preference)
	}
	conflicts, err := c.Conflicts.ListConflicts(ctx, connect.NewRequest(&v1.ListConflictsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	resolved := false
	for _, cf := range conflicts.Msg.Conflicts {
		resolved = resolved || (cf.Id == "address:100.64.0.9" && cf.PreferredNetworkId == a.Id)
	}
	if !resolved {
		t.Fatalf("conflict does not show the preference: %+v", conflicts.Msg.Conflicts)
	}
	if snap := snapshot(t, c); len(snap.DestinationPreferences) != 1 || snap.DestinationPreferences[0].NetworkId != a.Id {
		t.Fatalf("%+v", snap.DestinationPreferences)
	}

	if _, err := c.Networks.DisconnectNetwork(ctx, connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	r = inspect()
	if r.Preference.GetState() != v1.PreferenceState_PREFERENCE_STATE_NETWORK_NOT_CONNECTED || r.Reason != v1.DecisionReason_DECISION_REASON_EXACT_DEVICE_ADDRESS {
		t.Fatalf("preference for a disconnected network must not apply: %+v", r)
	}

	base := snapshot(t, c)
	col, stopWatch := watch(t, c, base.DaemonInstanceId, base.SnapshotSequence)
	defer stopWatch()
	if _, err := c.Networks.RemoveNetwork(ctx, connect.NewRequest(&v1.RemoveNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "preference removal announced", func() bool {
		for _, ev := range col.snapshot() {
			if p := ev.GetDestinationPreferenceRemoved(); p != nil && p.Destination == "100.64.0.9" {
				return true
			}
		}
		return false
	})
	if snap := snapshot(t, c); len(snap.DestinationPreferences) != 0 {
		t.Fatal("preference outlived its network")
	}
	for range 2 {
		if _, err := c.Preferences.DeleteDestinationPreference(ctx, connect.NewRequest(&v1.DeleteDestinationPreferenceRequest{Destination: "100.64.0.9"})); err != nil {
			t.Fatal("delete must be idempotent:", err)
		}
	}
}

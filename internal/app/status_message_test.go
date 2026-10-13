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

func TestDegradedNetworksCarryTheirReason(t *testing.T) {
	seq := &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("peer-"+string(cfg.NetworkID), "100.64.0.9")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Health = []string{"DERP region fra is unreachable"}
		e.SetStatus(st)
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: seq.Factory})
	n := addHeadscale(t, d.client, "Lab", "https://lab.example.com", false)
	if _, err := d.client.Networks.ConnectNetwork(context.Background(), connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: n.Id})); err != nil {
		t.Fatal(err)
	}
	get := func() *v1.Network {
		res, err := d.client.Networks.GetNetwork(context.Background(), connect.NewRequest(&v1.GetNetworkRequest{NetworkId: n.Id}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg.Network
	}
	eventually(t, "degraded with a reason", func() bool {
		got := get()
		return got.State == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DEGRADED && got.StatusMessage == "DERP region fra is unreachable"
	})

	e := seq.Engine(0)
	st := sessiontest.StatusWithPeer("peer-"+n.Id, "100.64.0.9")
	st.Self.NodeID = domain.NodeID("self-" + n.Id)
	st.Health = []string{"Unable to reach the control server"}
	e.SetStatus(st)
	e.PushNetMap()
	eventually(t, "a new reason while still degraded", func() bool { return get().StatusMessage == "Unable to reach the control server" })

	st.Health = nil
	e.SetStatus(st)
	e.PushNetMap()
	eventually(t, "no reason once connected", func() bool {
		got := get()
		return got.State == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED && got.StatusMessage == ""
	})
}

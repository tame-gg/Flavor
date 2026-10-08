package app_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
)

func selfEngines() *sessiontest.Sequence {
	return &sessiontest.Sequence{Prepare: func(_ int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		e.SetStatus(sessiontest.StatusSelf("self-"+string(cfg.NetworkID), "100.64.0.1"))
	}}
}

func TestWorkspacesEndToEnd(t *testing.T) {
	env := testEnv(t)
	d := start(t, env, app.Options{EngineFactory: selfEngines().Factory})
	c := d.client
	ctx := context.Background()

	lunar := addHeadscale(t, c, "LunarLabs", "https://lunar.example.com", false)
	mon := addHeadscale(t, c, "Monitoring", "https://mon.example.com", false)
	home := addHeadscale(t, c, "Home", "https://home.example.com", false)
	if _, err := c.Networks.ConnectNetwork(ctx, connect.NewRequest(&v1.ConnectNetworkRequest{NetworkId: home.Id})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "home connected", func() bool {
		return networkState(t, c, home.Id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
	})

	base := snapshot(t, c)
	col, stopWatch := watch(t, c, base.DaemonInstanceId, base.SnapshotSequence)
	defer stopWatch()

	_, err := c.Workspaces.CreateWorkspace(ctx, connect.NewRequest(&v1.CreateWorkspaceRequest{Name: "Ghost", NetworkIds: []string{"01NOSUCHNETWORK000000000000"}}))
	if flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("unknown network: %v", err)
	}
	_, err = c.Workspaces.CreateWorkspace(ctx, connect.NewRequest(&v1.CreateWorkspaceRequest{Name: "  "}))
	if flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("blank name: %v", err)
	}
	created, err := c.Workspaces.CreateWorkspace(ctx, connect.NewRequest(&v1.CreateWorkspaceRequest{
		Name: "On Call", Description: "pager", NetworkIds: []string{lunar.Id, mon.Id},
	}))
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Msg.Workspace
	if len(ws.NetworkIds) != 2 {
		t.Fatalf("%+v", ws)
	}
	if snap := snapshot(t, c); len(snap.Workspaces) != 1 || snap.ActiveWorkspaceId != "" {
		t.Fatalf("snapshot workspaces: %+v", snap.Workspaces)
	}

	act, err := c.Workspaces.ActivateWorkspace(ctx, connect.NewRequest(&v1.ActivateWorkspaceRequest{WorkspaceId: ws.Id}))
	if err != nil {
		t.Fatal(err)
	}
	if len(act.Msg.Results) != 2 {
		t.Fatalf("without disconnect_others only members are touched: %+v", act.Msg.Results)
	}
	for _, r := range act.Msg.Results {
		if r.Outcome != v1.ActivationOutcome_ACTIVATION_OUTCOME_CONNECTING {
			t.Fatalf("%+v", r)
		}
	}
	for _, id := range []string{lunar.Id, mon.Id, home.Id} {
		eventually(t, "all connected", func() bool {
			return networkState(t, c, id) == v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_CONNECTED
		})
	}

	again, err := c.Workspaces.ActivateWorkspace(ctx, connect.NewRequest(&v1.ActivateWorkspaceRequest{WorkspaceId: ws.Id, DisconnectOthers: true}))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]v1.ActivationOutcome{}
	for _, r := range again.Msg.Results {
		got[r.Network.Id] = r.Outcome
	}
	if got[lunar.Id] != v1.ActivationOutcome_ACTIVATION_OUTCOME_ALREADY_ACTIVE || got[home.Id] != v1.ActivationOutcome_ACTIVATION_OUTCOME_DISCONNECTED {
		t.Fatalf("%+v", again.Msg.Results)
	}
	if st := networkState(t, c, home.Id); st != v1.NetworkConnectionState_NETWORK_CONNECTION_STATE_DISCONNECTED {
		t.Fatalf("home not disconnected: %s", st)
	}
	if snapshot(t, c).ActiveWorkspaceId != ws.Id {
		t.Fatal("active workspace missing from snapshot")
	}

	if _, err := c.Networks.RemoveNetwork(ctx, connect.NewRequest(&v1.RemoveNetworkRequest{NetworkId: mon.Id})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "membership cleanup announced", func() bool {
		for _, ev := range col.snapshot() {
			if w := ev.GetWorkspaceChanged(); w != nil && w.Workspace.Id == ws.Id && len(w.Workspace.NetworkIds) == 1 {
				return true
			}
		}
		return false
	})
	sawActive := false
	for _, ev := range col.snapshot() {
		if a := ev.GetActiveWorkspaceChanged(); a != nil && a.WorkspaceId == ws.Id {
			sawActive = true
		}
	}
	if !sawActive {
		t.Fatal("activation not announced")
	}

	d.stop()
	d = start(t, env, app.Options{EngineFactory: selfEngines().Factory})
	c = d.client
	list, err := c.Workspaces.ListWorkspaces(ctx, connect.NewRequest(&v1.ListWorkspacesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Msg.Workspaces) != 1 || list.Msg.ActiveWorkspaceId != ws.Id || len(list.Msg.Workspaces[0].NetworkIds) != 1 {
		t.Fatalf("not persisted across restart: %+v", list.Msg)
	}

	name := "Night shift"
	upd, err := c.Workspaces.UpdateWorkspace(ctx, connect.NewRequest(&v1.UpdateWorkspaceRequest{
		WorkspaceId: ws.Id, Name: &name, Networks: &v1.NetworkIDList{NetworkIds: []string{home.Id}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if upd.Msg.Workspace.Name != name || len(upd.Msg.Workspace.NetworkIds) != 1 || upd.Msg.Workspace.NetworkIds[0] != home.Id {
		t.Fatalf("%+v", upd.Msg.Workspace)
	}

	if _, err := c.Workspaces.DeleteWorkspace(ctx, connect.NewRequest(&v1.DeleteWorkspaceRequest{WorkspaceId: ws.Id})); err != nil {
		t.Fatal(err)
	}
	if snap := snapshot(t, c); len(snap.Workspaces) != 0 || snap.ActiveWorkspaceId != "" {
		t.Fatalf("%+v", snap)
	}
	_, err = c.Workspaces.ActivateWorkspace(ctx, connect.NewRequest(&v1.ActivateWorkspaceRequest{WorkspaceId: ws.Id}))
	if flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_WORKSPACE_NOT_FOUND || connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing workspace: %v", err)
	}
}

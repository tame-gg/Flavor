package app_test

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
)

func TestChoosingSubnetRoutes(t *testing.T) {
	sub := netip.MustParsePrefix("10.200.0.0/22")
	engines := &sessiontest.Sequence{Prepare: func(i int, cfg provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		st := sessiontest.StatusWithPeer("router-"+string(cfg.NetworkID), "100.64.0.20")
		st.Self.NodeID = domain.NodeID("self-" + string(cfg.NetworkID))
		st.Peers[0].Hostname = "router"
		st.Peers[0].Routes = []netip.Prefix{sub}
		e.SetStatus(st)
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: engines.Factory})
	a := addHeadscale(t, d.client, "Lab", "https://lab.example.com", false)
	b := addHeadscale(t, d.client, "Office", "https://office.example.com", false)
	connectNetworks(t, d.client, a, b)
	ctx := context.Background()

	list, err := d.client.Routes.ListSubnetRoutes(ctx, connect.NewRequest(&v1.ListSubnetRoutesRequest{NetworkId: a.Id}))
	if err != nil {
		t.Fatal(err)
	}
	if r := list.Msg.Routes; len(r) != 1 || r[0].Prefix != sub.String() || r[0].Use != v1.SubnetRouteUse_SUBNET_ROUTE_USE_NOT_USED || r[0].Routers[0].Hostname != "router" || len(r[0].Overlaps) != 1 {
		t.Fatalf("%+v", r)
	}

	use := func(n *v1.Network, req *v1.UseSubnetRoutesRequest) (*v1.UseSubnetRoutesResponse, error) {
		req.NetworkId = n.Id
		res, err := d.client.Routes.UseSubnetRoutes(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := use(a, &v1.UseSubnetRoutesRequest{Add: []string{"10.200.0.0/22"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Routes[0]; r.Use != v1.SubnetRouteUse_SUBNET_ROUTE_USE_PENDING || !strings.Contains(r.Detail, "not active") {
		t.Fatalf("%+v", r)
	}
	if _, err := use(b, &v1.UseSubnetRoutesRequest{All: true}); err == nil || !strings.Contains(err.Error(), "already used from Lab") {
		t.Fatalf("the same route from two networks: %v", err)
	}
	for _, bad := range []string{"10.9.0.0/16", "0.0.0.0/0", "100.64.0.0/10"} {
		if _, err := use(a, &v1.UseSubnetRoutesRequest{Add: []string{bad}}); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
	res, err = use(a, &v1.UseSubnetRoutesRequest{None: true})
	if err != nil || res.Routes[0].Use != v1.SubnetRouteUse_SUBNET_ROUTE_USE_NOT_USED {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := use(b, &v1.UseSubnetRoutesRequest{All: true}); err != nil {
		t.Fatal(err)
	}
}

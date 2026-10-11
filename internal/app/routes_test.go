package app_test

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/client"
)

type routesView struct {
	routes   map[string]bool
	offered  bool
	approved bool
}

func viewOf(routes []*v1.AdvertisedRoute, offered, approved bool) routesView {
	v := routesView{routes: map[string]bool{}, offered: offered, approved: approved}
	for _, r := range routes {
		v.routes[r.Prefix] = r.Approved
	}
	return v
}

func getRoutes(t *testing.T, c *client.Client, network string) routesView {
	t.Helper()
	res, err := c.Routes.GetRoutes(context.Background(), connect.NewRequest(&v1.GetRoutesRequest{NetworkId: network}))
	if err != nil {
		t.Fatal(err)
	}
	return viewOf(res.Msg.Routes, res.Msg.ExitNodeOffered, res.Msg.ExitNodeApproved)
}

func updateRoutes(c *client.Client, network string, add, remove []string, exitNode *bool) (*v1.UpdateRoutesResponse, error) {
	res, err := c.Routes.UpdateRoutes(context.Background(), connect.NewRequest(&v1.UpdateRoutesRequest{NetworkId: network, Add: add, Remove: remove, ExitNode: exitNode}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func mustUpdateRoutes(t *testing.T, c *client.Client, network string, add, remove []string, exitNode *bool) routesView {
	t.Helper()
	res, err := updateRoutes(c, network, add, remove, exitNode)
	if err != nil {
		t.Fatal(err)
	}
	return viewOf(res.Routes, res.ExitNodeOffered, res.ExitNodeApproved)
}

func waitRoutes(t *testing.T, c *client.Client, network string, ok func(routesView) bool) routesView {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		v := getRoutes(t, c, network)
		if ok(v) {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("routes never reached the expected state: %+v", v)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAdvertiseRoutesOverIPC(t *testing.T) {
	c, seq, a, b := exitNodeDaemon(t)
	engine := seq.Engine(0)

	if v := getRoutes(t, c, a.Id); len(v.routes) != 0 || v.offered || v.approved {
		t.Fatalf("nothing advertised yet: %+v", v)
	}

	v := mustUpdateRoutes(t, c, a.Id, []string{"192.168.50.0/24"}, nil, nil)
	if approved, ok := v.routes["192.168.50.0/24"]; !ok || approved || len(v.routes) != 1 {
		t.Fatalf("%+v", v)
	}
	if got := engine.AdvertisedRoutes(); !slices.Equal(got, []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}) {
		t.Fatalf("engine routes %v", got)
	}
	if v := getRoutes(t, c, b.Id); len(v.routes) != 0 {
		t.Fatalf("other network affected: %+v", v)
	}

	engine.ApproveRoutes(netip.MustParsePrefix("192.168.50.0/24"))
	engine.PushNetMap()
	waitRoutes(t, c, a.Id, func(v routesView) bool { return v.routes["192.168.50.0/24"] })

	v = mustUpdateRoutes(t, c, a.Id, []string{"10.9.0.0/16", "192.168.50.7/24"}, nil, nil)
	if len(v.routes) != 2 || !v.routes["192.168.50.0/24"] || v.routes["10.9.0.0/16"] {
		t.Fatalf("%+v", v)
	}
	calls := len(engine.AdvertisedRoutesCalls())
	mustUpdateRoutes(t, c, a.Id, []string{"10.9.0.0/16"}, []string{"172.16.0.0/12"}, nil)
	if got := len(engine.AdvertisedRoutesCalls()); got != calls {
		t.Fatalf("repeating an add reached the engine: %d calls, want %d", got, calls)
	}

	v = mustUpdateRoutes(t, c, a.Id, nil, []string{"10.9.0.0/16"}, nil)
	if _, ok := v.routes["10.9.0.0/16"]; ok || len(v.routes) != 1 {
		t.Fatalf("%+v", v)
	}

	on, off := true, false
	v = mustUpdateRoutes(t, c, a.Id, nil, nil, &on)
	if !v.offered || v.approved || len(v.routes) != 1 {
		t.Fatalf("%+v", v)
	}
	if got := engine.AdvertisedRoutes(); !slices.Contains(got, netip.MustParsePrefix("0.0.0.0/0")) || !slices.Contains(got, netip.MustParsePrefix("::/0")) || len(got) != 3 {
		t.Fatalf("engine routes %v", got)
	}
	engine.ApproveExitNode(true)
	engine.PushNetMap()
	waitRoutes(t, c, a.Id, func(v routesView) bool { return v.offered && v.approved })

	v = mustUpdateRoutes(t, c, a.Id, []string{"10.9.0.0/16"}, nil, nil)
	if !v.offered || len(v.routes) != 2 {
		t.Fatalf("subnet changes must keep the exit node offer: %+v", v)
	}

	v = mustUpdateRoutes(t, c, a.Id, nil, []string{"10.9.0.0/16", "192.168.50.0/24"}, &off)
	if v.offered || v.approved || len(v.routes) != 0 {
		t.Fatalf("%+v", v)
	}
	if got := engine.AdvertisedRoutes(); len(got) != 0 {
		t.Fatalf("engine routes %v", got)
	}
}

func TestAdvertiseRoutesRejectsInvalidInput(t *testing.T) {
	c, seq, a, _ := exitNodeDaemon(t)
	cases := map[string][]string{
		"not a prefix":      {"192.168.1.1"},
		"garbage":           {"nonsense"},
		"empty":             {""},
		"default v4":        {"0.0.0.0/0"},
		"default v6":        {"::/0"},
		"tailnet v4":        {"100.64.0.0/10"},
		"inside tailnet v4": {"100.100.0.0/16"},
		"covers tailnet v4": {"100.0.0.0/8"},
		"tailnet v6":        {"fd7a:115c:a1e0::/48"},
		"valid then bad":    {"10.1.0.0/16", "bad"},
	}
	for name, add := range cases {
		if _, err := updateRoutes(c, a.Id, add, nil, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := updateRoutes(c, a.Id, nil, []string{"0.0.0.0/0"}, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("removing a default route: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, nil, []string{"bad"}, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("removing garbage: %v", err)
	}
	if _, err := updateRoutes(c, string(domain.NewNetworkID()), []string{"10.0.0.0/8"}, nil, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("unknown network: %v", err)
	}

	many := make([]string, 0, 65)
	for i := range 65 {
		many = append(many, fmt.Sprintf("10.%d.0.0/16", i))
	}
	if _, err := updateRoutes(c, a.Id, many, nil, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("65 routes: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, many[:64], nil, nil); err != nil {
		t.Fatalf("64 routes: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, []string{"172.16.0.0/16"}, nil, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("65th route: %v", err)
	}

	if calls := seq.Engine(0).AdvertisedRoutesCalls(); len(calls) != 1 || len(calls[0]) != 64 {
		t.Fatalf("rejected input reached the engine: %d calls", len(calls))
	}
}

func TestAdvertiseRoutesNeedsAConnectedNetwork(t *testing.T) {
	c, seq, a, _ := exitNodeDaemon(t)
	if _, err := c.Routes.GetRoutes(context.Background(), connect.NewRequest(&v1.GetRoutesRequest{NetworkId: string(domain.NewNetworkID())})); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("unknown network: %v", err)
	}
	if _, err := c.Networks.DisconnectNetwork(context.Background(), connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if _, err := updateRoutes(c, a.Id, []string{"10.0.0.0/8"}, nil, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_UNREACHABLE {
		t.Fatalf("disconnected network: %v", err)
	}
	if _, err := c.Routes.GetRoutes(context.Background(), connect.NewRequest(&v1.GetRoutesRequest{NetworkId: a.Id})); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_UNREACHABLE {
		t.Fatalf("disconnected network: %v", err)
	}
	if calls := seq.Engine(0).AdvertisedRoutesCalls(); len(calls) != 0 {
		t.Fatalf("%v", calls)
	}
}

func TestExitNodeOfferAndSelectionAreExclusive(t *testing.T) {
	c, _, a, _ := exitNodeDaemon(t)
	on := true

	setExitNode(t, c, a)
	if _, err := updateRoutes(c, a.Id, nil, nil, &on); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("offering while using an exit node: %v", err)
	}
	clearExitNode(t, c, a)

	mustUpdateRoutes(t, c, a.Id, nil, nil, &on)
	_, err := c.ExitNodes.SetExitNode(context.Background(), connect.NewRequest(&v1.SetExitNodeRequest{NetworkId: a.Id, NodeId: exitNodeID(a.Id)}))
	if flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("using an exit node while offering one: %v", err)
	}
}

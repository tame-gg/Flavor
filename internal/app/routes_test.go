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
	routes map[string]bool
}

func viewOf(routes []*v1.AdvertisedRoute) routesView {
	v := routesView{routes: map[string]bool{}}
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
	return viewOf(res.Msg.Routes)
}

func updateRoutes(c *client.Client, network string, add, remove []string) (*v1.UpdateRoutesResponse, error) {
	res, err := c.Routes.UpdateRoutes(context.Background(), connect.NewRequest(&v1.UpdateRoutesRequest{NetworkId: network, Add: add, Remove: remove}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func mustUpdateRoutes(t *testing.T, c *client.Client, network string, add, remove []string) routesView {
	t.Helper()
	res, err := updateRoutes(c, network, add, remove)
	if err != nil {
		t.Fatal(err)
	}
	return viewOf(res.Routes)
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

	if v := getRoutes(t, c, a.Id); len(v.routes) != 0 {
		t.Fatalf("nothing advertised yet: %+v", v)
	}

	v := mustUpdateRoutes(t, c, a.Id, []string{"192.168.50.0/24"}, nil)
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

	v = mustUpdateRoutes(t, c, a.Id, []string{"10.9.0.0/16", "192.168.50.7/24"}, nil)
	if len(v.routes) != 2 || !v.routes["192.168.50.0/24"] || v.routes["10.9.0.0/16"] {
		t.Fatalf("%+v", v)
	}
	calls := len(engine.AdvertisedRoutesCalls())
	mustUpdateRoutes(t, c, a.Id, []string{"10.9.0.0/16"}, []string{"172.16.0.0/12"})
	if got := len(engine.AdvertisedRoutesCalls()); got != calls {
		t.Fatalf("repeating an add reached the engine: %d calls, want %d", got, calls)
	}

	v = mustUpdateRoutes(t, c, a.Id, nil, []string{"10.9.0.0/16"})
	if _, ok := v.routes["10.9.0.0/16"]; ok || len(v.routes) != 1 {
		t.Fatalf("%+v", v)
	}

	v = mustUpdateRoutes(t, c, a.Id, nil, []string{"192.168.50.0/24"})
	if len(v.routes) != 0 {
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
		if _, err := updateRoutes(c, a.Id, add, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := updateRoutes(c, a.Id, nil, []string{"0.0.0.0/0"}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("removing a default route: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, nil, []string{"bad"}); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("removing garbage: %v", err)
	}
	if _, err := updateRoutes(c, string(domain.NewNetworkID()), []string{"10.0.0.0/8"}, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND {
		t.Fatalf("unknown network: %v", err)
	}

	many := make([]string, 0, 65)
	for i := range 65 {
		many = append(many, fmt.Sprintf("10.%d.0.0/16", i))
	}
	if _, err := updateRoutes(c, a.Id, many, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("65 routes: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, many[:64], nil); err != nil {
		t.Fatalf("64 routes: %v", err)
	}
	if _, err := updateRoutes(c, a.Id, []string{"172.16.0.0/16"}, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_INVALID_ARGUMENT {
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
	if _, err := updateRoutes(c, a.Id, []string{"10.0.0.0/8"}, nil); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_UNREACHABLE {
		t.Fatalf("disconnected network: %v", err)
	}
	if _, err := c.Routes.GetRoutes(context.Background(), connect.NewRequest(&v1.GetRoutesRequest{NetworkId: a.Id})); flavorCode(err) != v1.FlavorErrorCode_FLAVOR_ERROR_CODE_DESTINATION_UNREACHABLE {
		t.Fatalf("disconnected network: %v", err)
	}
	if calls := seq.Engine(0).AdvertisedRoutesCalls(); len(calls) != 0 {
		t.Fatalf("%v", calls)
	}
}

func TestRoutesAndExitNodeSurviveReconnect(t *testing.T) {
	c, seq, a, b := exitNodeDaemon(t)
	mustUpdateRoutes(t, c, a.Id, []string{"192.168.50.0/24"}, nil)
	setExitNode(t, c, b)

	for _, n := range []*v1.Network{a, b} {
		if _, err := c.Networks.DisconnectNetwork(context.Background(), connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: n.Id})); err != nil {
			t.Fatal(err)
		}
	}
	before := seq.Count()
	connectNetworks(t, c, a, b)
	if seq.Count() != before+2 {
		t.Fatalf("expected two new engines, have %d after %d", seq.Count(), before)
	}
	var routes []netip.Prefix
	var exit domain.NodeID
	for i := before; i < seq.Count(); i++ {
		e := seq.Engine(i)
		routes = append(routes, e.AdvertisedRoutes()...)
		if id := e.ExitNode(); id != "" {
			exit = id
		}
	}
	if !slices.Equal(routes, []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}) || exit != domain.NodeID(exitNodeID(b.Id)) {
		t.Fatalf("routes %v exit %q", routes, exit)
	}
}

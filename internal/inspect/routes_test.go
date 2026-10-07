package inspect_test

import (
	"errors"
	"net/netip"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

func router(net domain.NetworkID, node, host string, routes ...string) domain.Device {
	d := dev(net, node, host, "", "100.64.9."+node)
	for _, r := range routes {
		d.Routes = append(d.Routes, netip.MustParsePrefix(r))
	}
	return d
}

func TestSubnetRouteMatch(t *testing.T) {
	a := live("A", "LunarLabs", router("A", "5", "edge", "10.10.0.0/16"))
	r := resolve(t, "10.10.20.15:5432", a, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonSubnetRoute || r.Candidates[0].MatchedValue != "10.10.0.0/16" {
		t.Fatalf("%+v", r)
	}
}

func TestLongestPrefixWinsAcrossNetworks(t *testing.T) {
	wide := live("A", "LunarLabs", router("A", "5", "edge", "10.0.0.0/8"))
	narrow := live("B", "Customer", router("B", "5", "vpn", "10.20.0.0/16"))
	r := resolve(t, "10.20.5.12", wide, narrow)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonLongestPrefix {
		t.Fatalf("%+v", r)
	}
	if r.Candidates[0].Network.ID != "B" || r.Candidates[1].Status != inspect.StatusOutranked {
		t.Fatalf("%+v", r.Candidates)
	}
	if r := resolve(t, "10.30.0.1", wide, narrow); r.Reason != inspect.ReasonSubnetRoute || r.Candidates[0].Network.ID != "A" {
		t.Fatalf("outside the narrow route only the wide one applies: %+v", r)
	}
}

func TestEqualPrefixesOnTwoNetworksAreAmbiguousUntilPreferred(t *testing.T) {
	a := live("A", "Company", router("A", "5", "edge", "10.10.0.0/16"))
	b := live("B", "Customer", router("B", "5", "vpn", "10.10.0.0/16"))
	r := resolve(t, "10.10.1.1", a, b)
	if r.Decision != inspect.DecisionAmbiguous || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
	q, _ := inspect.ParseQuery("10.10.1.1")
	p := inspect.Resolve(q, []inspect.Network{a, b}, &domain.DestinationPreference{Destination: "10.10.1.1", Kind: domain.DestinationAddress, NetworkID: "B"})
	if p.Decision != inspect.DecisionUnique || p.Reason != inspect.ReasonDestinationPreference || p.Preference.Network.ID != "B" {
		t.Fatalf("%+v", p)
	}
}

func TestExactPeerAddressBeatsSubnetRoute(t *testing.T) {
	a := live("A", "Company", router("A", "5", "edge", "100.64.0.0/24"))
	r := resolve(t, "100.64.0.1", a, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExactDeviceAddress || r.Candidates[0].Network.ID != "B" {
		t.Fatalf("%+v", r)
	}
}

func TestNetworkQualifiedNames(t *testing.T) {
	r := resolve(t, "prod-api.lunarlabs.lattice.internal", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonNetworkQualifiedName || r.Candidates[0].Network.ID != "A" {
		t.Fatalf("%+v", r)
	}
	if r := resolve(t, "postgres.home.lattice.internal:5432", lunar, home); r.Candidates[0].Network.ID != "B" || r.Query.Port != 5432 {
		t.Fatalf("qualified name picks exactly one network even when the bare name collides: %+v", r)
	}
	if r := resolve(t, "prod-api.home.lattice.internal", lunar, home); r.Decision != inspect.DecisionNoMatch {
		t.Fatalf("device on another network must not match: %+v", r)
	}
	twin := live("C", "Home", dev("C", "1", "postgres", "", "100.64.7.7"))
	if r := resolve(t, "postgres.home.lattice.internal", home, twin); r.Decision != inspect.DecisionAmbiguous {
		t.Fatalf("two networks with the same friendly label: %+v", r)
	}
	for _, bad := range []string{"x.lattice.internal", "a.b.c.lattice.internal", "-x.home.lattice.internal"} {
		if _, err := inspect.ParseQuery(bad); !errors.Is(err, inspect.ErrInvalidDestination) {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestStableQualifiedNamesSurviveRenamesAndCollisions(t *testing.T) {
	const idA, idB = "01AAAAAAAAAAAAAAAAAAAAAAAA", "01BBBBBBBBBBBBBBBBBBBBBBBB"
	a := live(idA, "Home", dev(idA, "7", "postgres", "", "100.64.0.7"), dev(idA, "8", "Postgres", "", "100.64.0.8"))
	b := live(idB, "home", dev(idB, "7", "postgres", "", "100.64.0.7"))

	r := resolve(t, "id-7.01aaaaaaaaaaaaaaaaaaaaaaaa.lattice.internal", a, b)
	if r.Decision != inspect.DecisionUnique || r.Candidates[0].Device.ID.NodeID != "7" || r.Candidates[0].Network.ID != idA {
		t.Fatalf("stable name: %+v", r)
	}
	if r.Candidates[0].StableName != "id-7.01aaaaaaaaaaaaaaaaaaaaaaaa.lattice.internal" {
		t.Fatal(r.Candidates[0].StableName)
	}
	if r.Candidates[0].Name != r.Candidates[0].StableName {
		t.Fatal("with colliding friendly labels on both levels, the published name is the stable one")
	}
	if r := resolve(t, "postgres.01aaaaaaaaaaaaaaaaaaaaaaaa.lattice.internal", a, b); r.Decision != inspect.DecisionAmbiguous || len(r.Candidates) != 2 {
		t.Fatalf("colliding device labels inside one network: %+v", r)
	}
	if r := resolve(t, "postgres.01bbbbbbbbbbbbbbbbbbbbbbbb.lattice.internal", a, b); r.Decision != inspect.DecisionUnique || r.Candidates[0].Network.ID != idB {
		t.Fatalf("%+v", r)
	}

	renamed := a
	renamed.Network.DisplayName = "Personal"
	if r := resolve(t, "id-7.01aaaaaaaaaaaaaaaaaaaaaaaa.lattice.internal", renamed, b); r.Decision != inspect.DecisionUnique {
		t.Fatal("stable name must survive a rename")
	}
	if r := resolve(t, "id-8.personal.lattice.internal", renamed, b); r.Decision != inspect.DecisionUnique || r.Candidates[0].Name != "id-8.personal.lattice.internal" {
		t.Fatalf("friendly network label after rename: %+v", r)
	}

	squatter := live("01CCCCCCCCCCCCCCCCCCCCCCCC", "01aaaaaaaaaaaaaaaaaaaaaaaa", dev("01CCCCCCCCCCCCCCCCCCCCCCCC", "7", "postgres", "", "100.64.9.9"))
	if r := resolve(t, "id-7.01aaaaaaaaaaaaaaaaaaaaaaaa.lattice.internal", a, squatter); r.Decision != inspect.DecisionUnique || r.Candidates[0].Network.ID != idA {
		t.Fatalf("a stable label must always win over a friendly label that spells it: %+v", r)
	}
}

func TestSubnetConflicts(t *testing.T) {
	company := live("A", "Company", router("A", "5", "edge", "10.10.0.0/16", "10.0.0.0/8"))
	customer := live("B", "Customer", router("B", "5", "vpn", "10.10.0.0/16"))
	lab := live("C", "Lab", router("C", "5", "lab", "10.20.30.0/24"))
	got := byID(inspect.Conflicts([]inspect.Network{company, customer, lab}, nil))

	same, ok := got["subnet:10.10.0.0/16"]
	if !ok || same.Type != inspect.ConflictSubnet || same.Severity != inspect.SeverityAmbiguous || same.ContextResolves || len(same.Members) != 2 {
		t.Fatalf("identical routes: %+v", same)
	}
	if same.SampleAddress.String() != "10.10.0.1" || same.Members[0].Route.String() != "10.10.0.0/16" {
		t.Fatalf("%+v", same)
	}

	nested, ok := got["overlap:10.20.30.0/24"]
	if !ok || nested.Severity != inspect.SeverityExpected || !nested.ContextResolves {
		t.Fatalf("nested route: %+v", nested)
	}
	routes := map[string]bool{}
	for _, m := range nested.Members {
		routes[m.Network.DisplayName+" "+m.Route.String()] = true
	}
	if !routes["Company 10.0.0.0/8"] || !routes["Lab 10.20.30.0/24"] || len(routes) != 2 {
		t.Fatalf("%v", routes)
	}

	if _, ok := got["overlap:10.10.0.0/16"]; ok {
		t.Fatal("company also advertises the identical /16, so only the ambiguous identical-route conflict applies there")
	}
	only := live("D", "Solo", router("D", "5", "x", "10.0.0.0/8", "10.1.0.0/16"))
	if rep := inspect.Conflicts([]inspect.Network{only}, nil); len(rep.Conflicts) != 0 {
		t.Fatalf("nested routes inside one network are not a cross-network conflict: %+v", rep.Conflicts)
	}
}

func TestExplicitNetworkContext(t *testing.T) {
	q, _ := inspect.ParseQuery("100.64.0.1:22")
	q.Context = "B"
	pref := &domain.DestinationPreference{Destination: "100.64.0.1", Kind: domain.DestinationAddress, NetworkID: "A"}
	r := inspect.Resolve(q, []inspect.Network{lunar, home}, pref)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExplicitNetwork || r.Candidates[0].Network.ID != "B" || len(r.Candidates) != 1 {
		t.Fatalf("explicit network must win over preferences and drop other networks: %+v", r)
	}
	if r.Preference != nil {
		t.Fatal("preference reported although an explicit network was given")
	}
	off := inspect.Network{Network: domain.Network{ID: "B", DisplayName: "Home"}, State: domain.StateDisconnected}
	if r := inspect.Resolve(q, []inspect.Network{lunar, off}, nil); r.Decision != inspect.DecisionNoMatch || len(r.NotInspected) != 1 {
		t.Fatalf("explicit network that is down: %+v", r)
	}
}

package inspect_test

import (
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
)

func withExitNode(n inspect.Network, node string) inspect.Network {
	for i := range n.Devices {
		n.Devices[i].ExitNodeOption = true
		if string(n.Devices[i].ID.NodeID) == node {
			n.Devices[i].ExitNode = true
			n.ExitNode = n.Devices[i]
		}
	}
	return n
}

func exitNetworks() (inspect.Network, inspect.Network) {
	a := withExitNode(live("A", "LunarLabs", dev("A", "1", "gateway", "gateway.lunar.ts.net", "100.64.0.1")), "1")
	b := withExitNode(live("B", "Home", dev("B", "7", "router", "router.home.ts.net", "100.64.0.7")), "7")
	return a, b
}

func TestExitNodeIsTheFallbackForUnknownAddress(t *testing.T) {
	a, _ := exitNetworks()
	r := resolve(t, "203.0.113.7:443", a, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExitNode || r.DecidedBy != inspect.MatchExitNode || len(r.Candidates) != 1 {
		t.Fatalf("%+v", r)
	}
	c := r.Candidates[0]
	if c.Status != inspect.StatusSelected || c.Match != inspect.MatchExitNode || c.Network.ID != "A" || c.Device.Hostname != "gateway" || c.MatchedValue != "203.0.113.7" {
		t.Fatalf("%+v", c)
	}
}

func TestExitNodeResolvesUnknownHostnames(t *testing.T) {
	a, _ := exitNetworks()
	r := resolve(t, "example.com:443", a, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExitNode || r.Candidates[0].MatchedValue != "example.com" {
		t.Fatalf("%+v", r)
	}
}

func TestExitNodeNeverOutranksARealMatch(t *testing.T) {
	a, _ := exitNetworks()
	a.Devices = append(a.Devices, dev("A", "2", "web", "web.lunar.ts.net", "100.64.0.2", "203.0.113.7"))
	r := resolve(t, "203.0.113.7", a)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExactDeviceAddress {
		t.Fatalf("%+v", r)
	}
}

func TestExitNodeIsAmbiguousAcrossNetworks(t *testing.T) {
	a, b := exitNetworks()
	r := resolve(t, "203.0.113.7", b, a)
	if r.Decision != inspect.DecisionAmbiguous || r.Reason != inspect.ReasonMultipleMatches || r.DecidedBy != inspect.MatchExitNode || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
	if r.Candidates[0].Network.DisplayName != "Home" || r.Candidates[1].Network.DisplayName != "LunarLabs" {
		t.Fatalf("candidates not in deterministic order: %+v", r.Candidates)
	}
	for _, c := range r.Candidates {
		if c.Status != inspect.StatusTied {
			t.Fatalf("%+v", c)
		}
	}
}

func TestExitNodePreferenceResolvesAmbiguity(t *testing.T) {
	a, b := exitNetworks()
	r := resolveWith(t, "203.0.113.7:443", prefer("203.0.113.7", domain.DestinationAddress, "B"), a, b)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDestinationPreference || r.DecidedBy != inspect.MatchExitNode {
		t.Fatalf("%+v", r)
	}
	if r.Preference == nil || r.Preference.State != inspect.PreferenceApplied {
		t.Fatalf("%+v", r.Preference)
	}
	for _, c := range r.Candidates {
		if (c.Network.ID == "B") != (c.Status == inspect.StatusSelected) {
			t.Fatalf("%+v", c)
		}
	}

	r = resolveWith(t, "example.com", prefer("example.com", domain.DestinationName, "A"), a, b)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDestinationPreference {
		t.Fatalf("%+v", r)
	}
	for _, c := range r.Candidates {
		if (c.Network.ID == "A") != (c.Status == inspect.StatusSelected) {
			t.Fatalf("%+v", c)
		}
	}
}

func TestExitNodePreferenceForANetworkWithoutOneChangesNothing(t *testing.T) {
	a, _ := exitNetworks()
	r := resolveWith(t, "203.0.113.7", prefer("203.0.113.7", domain.DestinationAddress, "B"), a, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExitNode {
		t.Fatalf("%+v", r)
	}
	if r.Preference == nil || r.Preference.State != inspect.PreferenceNoMatchOnNetwork {
		t.Fatalf("%+v", r.Preference)
	}
}

func TestExitNodeExplicitNetwork(t *testing.T) {
	a, b := exitNetworks()
	q, err := inspect.ParseQuery("203.0.113.7:443")
	if err != nil {
		t.Fatal(err)
	}
	q.Context = "B"
	r := inspect.Resolve(q, []inspect.Network{a, b}, nil)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExplicitNetwork || r.Candidates[0].Network.ID != "B" {
		t.Fatalf("%+v", r)
	}
	q.Context = "C"
	if r := inspect.Resolve(q, []inspect.Network{a, b}, nil); r.Decision != inspect.DecisionNoMatch {
		t.Fatalf("%+v", r)
	}
}

func TestTailnetDestinationsNeverFallBackToAnExitNode(t *testing.T) {
	a, b := exitNetworks()
	for _, dest := range []string{
		"100.64.9.9", "100.127.255.254:22", "fd7a:115c:a1e0::99",
		"missing.lunarlabs.flavor.internal", "api.nowhere.flavor.internal:443",
	} {
		if r := resolve(t, dest, a, b); r.Decision != inspect.DecisionNoMatch || len(r.Candidates) != 0 {
			t.Fatalf("%s: %+v", dest, r)
		}
	}
	if r := resolve(t, "100.128.0.1", a, b); r.Reason != inspect.ReasonMultipleMatches {
		t.Fatalf("addresses outside the tailnet ranges still use exit nodes: %+v", r)
	}
}

func TestNoExitNodeSelectedIsNoMatch(t *testing.T) {
	r := resolve(t, "203.0.113.7", lunar, home)
	if r.Decision != inspect.DecisionNoMatch || r.Reason != inspect.ReasonNoMatch || len(r.Candidates) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestExitNodeOnADisconnectedNetworkIsIgnored(t *testing.T) {
	a, b := exitNetworks()
	b.Live, b.State = false, domain.StateDisconnected
	r := resolve(t, "203.0.113.7", a, b)
	if r.Decision != inspect.DecisionUnique || r.Candidates[0].Network.ID != "A" || len(r.NotInspected) != 1 {
		t.Fatalf("%+v", r)
	}
}

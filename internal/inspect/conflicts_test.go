package inspect_test

import (
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
)

func byID(rep inspect.ConflictReport) map[string]inspect.Conflict {
	out := make(map[string]inspect.Conflict)
	for _, c := range rep.Conflicts {
		out[c.ID] = c
	}
	return out
}

func local(d domain.Device) domain.Device {
	d.Local = true
	return d
}

func TestAddressOverlapAcrossNetworksIsExpectedWhenNamesDisambiguate(t *testing.T) {
	rep := inspect.Conflicts([]inspect.Network{lunar, home}, nil)
	c, ok := byID(rep)["address:100.64.0.1"]
	if !ok {
		t.Fatalf("missing address conflict: %+v", rep.Conflicts)
	}
	if c.Type != inspect.ConflictAddress || c.Scope != inspect.ScopeCrossNetwork || c.Severity != inspect.SeverityExpected || !c.ContextResolves {
		t.Fatalf("%+v", c)
	}
	if len(c.Members) != 2 || c.Members[0].Network.DisplayName != "Home" || c.Members[0].UniqueName != "desktop.home.ts.net" {
		t.Fatalf("%+v", c.Members)
	}
}

func TestCollisionAcrossThreeNetworksIsOneConflict(t *testing.T) {
	third := live("C", "Customer", dev("C", "9", "app", "app.customer.ts.net", "100.64.0.1"))
	c := byID(inspect.Conflicts([]inspect.Network{lunar, home, third}, nil))["address:100.64.0.1"]
	if len(c.Members) != 3 {
		t.Fatalf("%+v", c)
	}
}

func TestSameAddressInsideOneNetworkIsAmbiguous(t *testing.T) {
	n := live("A", "LunarLabs", dev("A", "1", "a", "a.lunar.ts.net", "100.64.0.7"), dev("A", "2", "b", "b.lunar.ts.net", "100.64.0.7"))
	c := byID(inspect.Conflicts([]inspect.Network{n}, nil))["address:100.64.0.7"]
	if c.Scope != inspect.ScopeWithinNetwork || c.Severity != inspect.SeverityAmbiguous {
		t.Fatalf("%+v", c)
	}
}

func TestHostnameCollisionAcrossNetworks(t *testing.T) {
	c, ok := byID(inspect.Conflicts([]inspect.Network{lunar, home}, nil))["name:postgres"]
	if !ok || c.Type != inspect.ConflictHostname || c.Severity != inspect.SeverityExpected || len(c.Members) != 2 {
		t.Fatalf("%+v", c)
	}
}

func TestDNSNameCollisionIsAlwaysAmbiguous(t *testing.T) {
	a := live("A", "Customer A", dev("A", "1", "db", "db.example.com", "100.64.0.3"))
	b := live("B", "Customer B", dev("B", "1", "db", "db.example.com", "100.64.0.4"))
	got := byID(inspect.Conflicts([]inspect.Network{a, b}, nil))
	dns := got["dns:db.example.com"]
	if dns.Severity != inspect.SeverityAmbiguous || dns.ContextResolves {
		t.Fatalf("%+v", dns)
	}
	if name := got["name:db"]; name.Severity != inspect.SeverityAmbiguous || name.ContextResolves {
		t.Fatalf("bare name cannot be resolved by a DNS name that itself collides: %+v", name)
	}
}

func TestNoFalseConflictsForUnrelatedDevices(t *testing.T) {
	a := live("A", "LunarLabs", dev("A", "1", "api", "api.lunar.ts.net", "100.64.0.1"))
	b := live("B", "Home", dev("B", "1", "desktop", "desktop.home.ts.net", "100.64.0.2"))
	if rep := inspect.Conflicts([]inspect.Network{a, b}, nil); len(rep.Conflicts) != 0 {
		t.Fatalf("%+v", rep.Conflicts)
	}
}

func TestThisMachineOnSeveralNetworksIsNotAConflict(t *testing.T) {
	a := live("A", "LunarLabs", local(dev("A", "1", "workstation", "workstation.lunar.ts.net", "100.64.0.1")))
	b := live("B", "Home", local(dev("B", "1", "workstation", "workstation.home.ts.net", "100.64.0.1")))
	if rep := inspect.Conflicts([]inspect.Network{a, b}, nil); len(rep.Conflicts) != 0 {
		t.Fatalf("local identities reported as conflicts: %+v", rep.Conflicts)
	}
	c := live("C", "Customer", dev("C", "1", "workstation", "", "100.64.0.1"))
	got := byID(inspect.Conflicts([]inspect.Network{a, b, c}, nil))
	if len(got["address:100.64.0.1"].Members) != 3 {
		t.Fatal("a real peer sharing the address with this machine must still be reported")
	}
}

func TestConflictOrderingAndNotInspected(t *testing.T) {
	off := inspect.Network{Network: domain.Network{ID: "Z", DisplayName: "Work"}, State: domain.StateDisconnected}
	n := live("D", "Staging", dev("D", "1", "a", "", "100.64.0.7"), dev("D", "2", "b", "", "100.64.0.7"))
	rep := inspect.Conflicts([]inspect.Network{lunar, home, n, off}, nil)
	if len(rep.NotInspected) != 1 {
		t.Fatalf("%+v", rep.NotInspected)
	}
	for i := 1; i < len(rep.Conflicts); i++ {
		if rep.Conflicts[i-1].Severity > rep.Conflicts[i].Severity {
			t.Fatal("ambiguous conflicts must come first")
		}
	}
	again := inspect.Conflicts([]inspect.Network{off, n, home, lunar}, nil)
	for i := range rep.Conflicts {
		if rep.Conflicts[i].ID != again.Conflicts[i].ID {
			t.Fatal("conflict order depends on input order")
		}
	}
}

package inspect_test

import (
	"errors"
	"net/netip"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

func dev(net domain.NetworkID, node, host, dns string, addrs ...string) domain.Device {
	d := domain.Device{ID: domain.DeviceIdentity{NetworkID: net, NodeID: domain.NodeID(node)}, Hostname: host, DNSName: dns, Online: true}
	for _, a := range addrs {
		d.Addresses = append(d.Addresses, netip.MustParseAddr(a))
	}
	return d
}

func live(id, name string, devices ...domain.Device) inspect.Network {
	return inspect.Network{Network: domain.Network{ID: domain.NetworkID(id), DisplayName: name}, State: domain.StateConnected, Devices: devices, Live: true}
}

func resolve(t *testing.T, raw string, nets ...inspect.Network) inspect.Result {
	t.Helper()
	q, err := inspect.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return inspect.Resolve(q, nets, nil)
}

var (
	lunar = live("A", "LunarLabs",
		dev("A", "1", "prod-api", "prod-api.lunar.ts.net", "100.64.0.1", "fd7a:115c:a1e0::1"),
		dev("A", "2", "postgres", "postgres.lunar.ts.net", "100.64.0.2"),
	)
	home = live("B", "Home",
		dev("B", "1", "desktop", "desktop.home.ts.net", "100.64.0.1"),
		dev("B", "2", "postgres", "db.home.ts.net", "100.64.0.9"),
	)
)

func TestUniqueExactAddress(t *testing.T) {
	r := resolve(t, "100.64.0.2", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonExactDeviceAddress || len(r.Candidates) != 1 {
		t.Fatalf("%+v", r)
	}
	if c := r.Candidates[0]; c.Status != inspect.StatusSelected || c.Network.ID != "A" || c.Device.Hostname != "postgres" {
		t.Fatalf("%+v", c)
	}
}

func TestDuplicateAddressAcrossNetworksIsAmbiguous(t *testing.T) {
	r := resolve(t, "100.64.0.1", home, lunar)
	if r.Decision != inspect.DecisionAmbiguous || r.Reason != inspect.ReasonMultipleMatches || r.DecidedBy != inspect.MatchDeviceAddress {
		t.Fatalf("%+v", r)
	}
	if len(r.Candidates) != 2 || r.Candidates[0].Network.DisplayName != "Home" || r.Candidates[1].Network.DisplayName != "LunarLabs" {
		t.Fatalf("candidates not in deterministic order: %+v", r.Candidates)
	}
	for _, c := range r.Candidates {
		if c.Status != inspect.StatusTied {
			t.Fatalf("%+v", c)
		}
	}
}

func TestIPv6AndPort(t *testing.T) {
	r := resolve(t, "[fd7a:115c:a1e0::1]:443", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Query.Port != 443 || r.Query.Normalized() != "fd7a:115c:a1e0::1" {
		t.Fatalf("%+v", r)
	}
	r = resolve(t, "100.64.0.2:5432", lunar)
	if r.Query.Port != 5432 || r.Decision != inspect.DecisionUnique {
		t.Fatalf("%+v", r)
	}
}

func TestHostnameMatch(t *testing.T) {
	r := resolve(t, "Prod-API", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDeviceHostname || r.Candidates[0].Device.ID.NodeID != "1" {
		t.Fatalf("%+v", r)
	}
}

func TestShortDNSLabelCountsAsHostname(t *testing.T) {
	r := resolve(t, "db", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDeviceHostname || r.Candidates[0].Network.ID != "B" {
		t.Fatalf("%+v", r)
	}
}

func TestHostnameCollisionIsAmbiguous(t *testing.T) {
	r := resolve(t, "postgres", lunar, home)
	if r.Decision != inspect.DecisionAmbiguous || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestFullDNSNameIsUnique(t *testing.T) {
	r := resolve(t, "postgres.lunar.ts.net.", lunar, home)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDeviceDNSName || r.Candidates[0].Network.ID != "A" {
		t.Fatalf("%+v", r)
	}
}

func TestDNSNameOutranksHostname(t *testing.T) {
	byDNS := live("C", "Customer", dev("C", "1", "storage", "nas", "100.64.5.5"))
	byHost := live("B", "Home", dev("B", "1", "nas", "nas.home.ts.net", "100.64.0.9"))
	r := resolve(t, "nas", byHost, byDNS)
	if r.Decision != inspect.DecisionUnique || r.Reason != inspect.ReasonDeviceDNSName || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
	if r.Candidates[0].Network.ID != "C" || r.Candidates[1].Status != inspect.StatusOutranked {
		t.Fatalf("%+v", r.Candidates)
	}
}

func TestNoMatchAndNotInspectedNetworks(t *testing.T) {
	off := inspect.Network{Network: domain.Network{ID: "Z", DisplayName: "Work"}, State: domain.StateDisconnected}
	r := resolve(t, "10.10.20.15", lunar, off)
	if r.Decision != inspect.DecisionNoMatch || r.Reason != inspect.ReasonNoMatch || len(r.Candidates) != 0 {
		t.Fatalf("%+v", r)
	}
	if len(r.NotInspected) != 1 || r.NotInspected[0].Network.ID != "Z" {
		t.Fatalf("%+v", r.NotInspected)
	}
}

func TestIdentitySafety(t *testing.T) {
	dup := live("A", "LunarLabs",
		dev("A", "1", "prod-api", "", "100.64.0.1"),
		dev("A", "1", "prod-api", "", "100.64.0.1"),
		dev("B", "7", "smuggled", "", "100.64.0.1"),
	)
	r := resolve(t, "100.64.0.1", dup)
	if r.Decision != inspect.DecisionUnique || len(r.Candidates) != 1 {
		t.Fatalf("duplicate or foreign identities leaked into candidates: %+v", r.Candidates)
	}
	same := live("A", "LunarLabs", dev("A", "1", "a", "", "100.64.0.1"), dev("A", "2", "b", "", "100.64.0.1"))
	if r := resolve(t, "100.64.0.1", same); r.Decision != inspect.DecisionAmbiguous || len(r.Candidates) != 2 {
		t.Fatalf("two nodes sharing an address in one network must stay distinct: %+v", r)
	}
}

func TestMalformedDestinations(t *testing.T) {
	for _, raw := range []string{"", "   ", "https://example.com", "10.0.0.0/8", "a b", "host:99999", "host:0", "-bad.example", "fe80::1%eth0", "user@host", "bad!name"} {
		if _, err := inspect.ParseQuery(raw); !errors.Is(err, inspect.ErrInvalidDestination) {
			t.Fatalf("%q accepted", raw)
		}
	}
}

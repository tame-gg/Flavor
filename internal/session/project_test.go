package session

import (
	"net/netip"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/views"
)

func TestConnectedInvariant(t *testing.T) {
	addr := netip.MustParseAddr("100.64.0.1")
	ok := EngineStatus{
		BackendState: "Running",
		TailscaleIPs: []netip.Addr{addr},
		Self:         &EnginePeer{NodeID: "n1", Addresses: []netip.Addr{addr}},
	}
	if !connectedInvariant(ok) {
		t.Fatal("expected connected")
	}
	if connectedInvariant(EngineStatus{BackendState: "Running", TailscaleIPs: []netip.Addr{addr}}) {
		t.Fatal("missing self")
	}
	if connectedInvariant(EngineStatus{BackendState: "Running", Self: &EnginePeer{NodeID: "n1"}}) {
		t.Fatal("missing ips")
	}
	if mapConnectionState(ok, false) != domain.StateConnected {
		t.Fatal(mapConnectionState(ok, false))
	}
	degraded := ok
	degraded.Health = []string{"dns"}
	if mapConnectionState(degraded, false) != domain.StateDegraded {
		t.Fatal("degraded")
	}
	if mapConnectionState(EngineStatus{BackendState: "NeedsMachineAuth"}, false) != domain.StateAwaitingApproval {
		t.Fatal("approval")
	}
}

func TestParseAuthURL(t *testing.T) {
	if _, err := parseAuthURL("javascript:alert(1)"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := parseAuthURL("https://login.tailscale.com/a/x"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseAuthURL("http://127.0.0.1:8080/a/x"); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceEqual(t *testing.T) {
	a := domain.Device{
		ID:       domain.DeviceIdentity{NetworkID: "n", NodeID: "a"},
		Hostname: "h", Addresses: []netip.Addr{netip.MustParseAddr("100.64.0.1")}, Online: true,
	}
	b := cloneDevice(a)
	if !deviceEqual(a, b) {
		t.Fatal("equal")
	}
	b.Hostname = "x"
	if deviceEqual(a, b) {
		t.Fatal("hostname")
	}
}

func TestProjectPeerCarriesOSAndTags(t *testing.T) {
	tags := views.SliceOf([]string{"tag:db", "tag:prod"})
	p := projectPeer(&ipnstate.PeerStatus{
		ID:       "n1",
		HostName: "postgres",
		DNSName:  "postgres.example.ts.net.",
		OS:       "linux",
		Tags:     &tags,
	})
	if p.OS != "linux" || len(p.Tags) != 2 || p.Tags[1] != "tag:prod" || p.DNSName != "postgres.example.ts.net" {
		t.Fatalf("%+v", p)
	}
	primary := views.SliceOf([]netip.Prefix{netip.MustParsePrefix("10.10.20.7/24"), netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("fd00:1::/64")})
	router := projectPeer(&ipnstate.PeerStatus{ID: "r", PrimaryRoutes: &primary})
	if len(router.Routes) != 2 || router.Routes[0].String() != "10.10.20.0/24" || router.Routes[1].String() != "fd00:1::/64" {
		t.Fatalf("subnet routes must be masked and exclude default routes: %v", router.Routes)
	}
	a2 := domain.Device{ID: domain.DeviceIdentity{NodeID: "r"}, Routes: router.Routes}
	b2 := a2
	b2.Routes = router.Routes[:1]
	if deviceEqual(a2, b2) {
		t.Fatal("route change not detected")
	}
	if bare := projectPeer(&ipnstate.PeerStatus{ID: "n2"}); bare.Tags != nil {
		t.Fatalf("%+v", bare)
	}
	a := domain.Device{ID: domain.DeviceIdentity{NodeID: "n1"}, Tags: []string{"tag:db"}}
	b := a
	b.Tags = []string{"tag:db", "tag:prod"}
	if deviceEqual(a, b) {
		t.Fatal("tag change not detected")
	}
	b = a
	b.OS = "windows"
	if deviceEqual(a, b) {
		t.Fatal("os change not detected")
	}
}

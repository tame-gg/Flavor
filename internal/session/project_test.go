package session

import (
	"net/netip"
	"slices"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
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

func TestProjectExitNode(t *testing.T) {
	st := projectStatus(&ipnstate.Status{
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			key.NewNode().Public(): {ID: "gw", ExitNodeOption: true},
			key.NewNode().Public(): {ID: "plain"},
		},
		ExitNodeStatus: &ipnstate.ExitNodeStatus{ID: " gw "},
	})
	if st.ExitNode != "gw" {
		t.Fatalf("%q", st.ExitNode)
	}
	options := map[domain.NodeID]bool{}
	for _, p := range st.Peers {
		options[p.NodeID] = p.ExitNodeOption
	}
	if !options["gw"] || options["plain"] {
		t.Fatalf("%v", options)
	}
	if none := projectStatus(&ipnstate.Status{}); none.ExitNode != "" {
		t.Fatalf("%q", none.ExitNode)
	}
	a := domain.Device{ID: domain.DeviceIdentity{NodeID: "gw"}}
	b := a
	b.ExitNodeOption = true
	if deviceEqual(a, b) {
		t.Fatal("exit node option change not detected")
	}
	b = a
	b.ExitNode = true
	if deviceEqual(a, b) || !cloneDevice(b).ExitNode {
		t.Fatal("exit node selection change not detected")
	}
}

func TestProjectDNSRecords(t *testing.T) {
	if projectDNSRecords(nil) != nil {
		t.Fatal("nil config")
	}
	cfg := &tailcfg.DNSConfig{ExtraRecords: []tailcfg.DNSRecord{
		{Name: "Git.Intra.Example.Internal.", Value: "100.64.0.13"},
		{Name: "git.intra.example.internal", Type: "AAAA", Value: "fd7a:115c:a1e0::d"},
		{Name: "git.intra.example.internal", Type: "A", Value: "100.64.0.13"},
		{Name: "mail.example.internal", Type: "TXT", Value: "v=spf1"},
		{Name: "broken.example.internal", Value: "not an address"},
		{Name: "", Value: "100.64.0.1"},
		{Name: "app.example.internal", Value: " 100.64.0.3 "},
	}}
	got := projectDNSRecords(cfg)
	want := []domain.DNSRecord{
		{Name: "app.example.internal", Addresses: []netip.Addr{netip.MustParseAddr("100.64.0.3")}},
		{Name: "git.intra.example.internal", Addresses: []netip.Addr{netip.MustParseAddr("100.64.0.13"), netip.MustParseAddr("fd7a:115c:a1e0::d")}},
	}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || !slices.Equal(got[i].Addresses, want[i].Addresses) {
			t.Fatalf("record %d: %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestProjectApprovedRoutes(t *testing.T) {
	allowed := []netip.Prefix{
		netip.MustParsePrefix("100.64.0.1/32"),
		netip.MustParsePrefix("fd7a:115c:a1e0::1/128"),
		netip.MustParsePrefix("192.168.1.0/24"),
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("::/0"),
		netip.MustParsePrefix("10.1.2.3/32"),
	}
	view := views.SliceOf(allowed)
	st := projectStatus(&ipnstate.Status{Self: &ipnstate.PeerStatus{
		ID:           "self",
		TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")},
		AllowedIPs:   &view,
	}})
	want := []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("10.1.2.3/32")}
	if !slices.Equal(st.ApprovedRoutes, want) {
		t.Fatalf("%v", st.ApprovedRoutes)
	}
	if none := projectStatus(&ipnstate.Status{Self: &ipnstate.PeerStatus{ID: "self"}}); len(none.ApprovedRoutes) != 0 {
		t.Fatalf("%v", none.ApprovedRoutes)
	}
}

func TestProjectAdvertised(t *testing.T) {
	lan := netip.MustParsePrefix("192.168.1.0/24")
	wide := netip.MustParsePrefix("10.0.0.0/8")
	v4, v6 := netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")
	routes, exit := projectAdvertised(EngineStatus{
		AdvertisedRoutes: []netip.Prefix{lan, v4, wide, lan, netip.MustParsePrefix("10.9.9.9/16")},
		ApprovedRoutes:   []netip.Prefix{wide, netip.MustParsePrefix("172.16.0.0/12")},
	})
	if len(routes) != 3 || routes[0].Prefix != wide || !routes[0].Approved || routes[1].Prefix != netip.MustParsePrefix("10.9.0.0/16") || routes[2].Prefix != lan || routes[2].Approved {
		t.Fatalf("%+v", routes)
	}
	if exit.Offered || exit.Approved {
		t.Fatalf("one default route is not an exit node offer: %+v", exit)
	}

	self := &EnginePeer{ExitNodeOption: true}
	_, exit = projectAdvertised(EngineStatus{AdvertisedRoutes: []netip.Prefix{v6, v4}, Self: self})
	if !exit.Offered || !exit.Approved {
		t.Fatalf("%+v", exit)
	}
	_, exit = projectAdvertised(EngineStatus{Self: self})
	if exit.Offered || exit.Approved {
		t.Fatalf("an approved option without an offer is not offered: %+v", exit)
	}
}

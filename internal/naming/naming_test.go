package naming_test

import (
	"strings"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/naming"
)

func TestStableLabels(t *testing.T) {
	if got := naming.StableNetworkLabel("01JA7Q3M0000000000000HOME0"); got != "01ja7q3m0000000000000home0" {
		t.Fatal(got)
	}
	if got := naming.StableNetworkLabel("Mixed_Case"); !strings.HasPrefix(got, "ih-") {
		t.Fatalf("non-ULID ids that lowercasing could collide must be hashed: %q", got)
	}
	if got := naming.StableDeviceLabel("12"); got != "id-12" {
		t.Fatal(got)
	}
	a, b := naming.StableDeviceLabel("nAbC1CNTRL"), naming.StableDeviceLabel("nabc1cntrl")
	if !strings.HasPrefix(a, "ih-") || a == b || b != "id-nabc1cntrl" {
		t.Fatalf("case-distinct node ids must stay distinct: %q %q", a, b)
	}
	if got := naming.StableDeviceLabel(domain.NodeID(strings.Repeat("a", 61))); !strings.HasPrefix(got, "ih-") {
		t.Fatal(got)
	}
	for _, l := range []string{a, b, naming.StableNetworkLabel("Mixed_Case")} {
		if !naming.ValidLabel(l) {
			t.Fatalf("stable label %q is not a valid DNS label", l)
		}
	}
}

func TestFriendlyDeviceCandidate(t *testing.T) {
	cases := []struct {
		host, dns, want string
	}{
		{"Prod-API", "", "prod-api"},
		{"Luna's MacBook Pro", "lunas-macbook-pro.tail.ts.net.", "lunas-macbook-pro"},
		{"db.example.com", "db.home.ts.net", "db"},
		{"Ünï", "", "n"},
		{strings.Repeat("x", 70), "", strings.Repeat("x", 63)},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := naming.FriendlyDeviceCandidate(domain.Device{Hostname: c.host, DNSName: c.dns}); got != c.want {
			t.Fatalf("%q/%q: got %q want %q", c.host, c.dns, got, c.want)
		}
	}
}

func TestDuplicateFriendlyLabelsAreNotPublished(t *testing.T) {
	nets := naming.NetworkLabels([]domain.Network{
		{ID: "01AAAAAAAAAAAAAAAAAAAAAAAA", DisplayName: "Home"},
		{ID: "01BBBBBBBBBBBBBBBBBBBBBBBB", DisplayName: "home"},
		{ID: "01CCCCCCCCCCCCCCCCCCCCCCCC", DisplayName: "LunarLabs"},
		{ID: "01DDDDDDDDDDDDDDDDDDDDDDDD", DisplayName: "01aaaaaaaaaaaaaaaaaaaaaaaa"},
	})
	if nets["01AAAAAAAAAAAAAAAAAAAAAAAA"].Friendly != "" || nets["01BBBBBBBBBBBBBBBBBBBBBBBB"].Friendly != "" {
		t.Fatal("ambiguous friendly network label published")
	}
	if nets["01CCCCCCCCCCCCCCCCCCCCCCCC"].Published() != "lunarlabs" {
		t.Fatal(nets["01CCCCCCCCCCCCCCCCCCCCCCCC"])
	}
	if nets["01DDDDDDDDDDDDDDDDDDDDDDDD"].Friendly != "" {
		t.Fatal("a friendly label equal to another network's stable label must not be published")
	}
	if nets["01AAAAAAAAAAAAAAAAAAAAAAAA"].Published() != "01aaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatal("stable label must always be available")
	}

	devs := naming.DeviceLabels([]domain.Device{
		{ID: domain.DeviceIdentity{NodeID: "1"}, Hostname: "postgres"},
		{ID: domain.DeviceIdentity{NodeID: "2"}, Hostname: "Postgres"},
		{ID: domain.DeviceIdentity{NodeID: "3"}, Hostname: "grafana"},
		{ID: domain.DeviceIdentity{NodeID: "4"}, Hostname: "id-3"},
	})
	if devs["1"].Friendly != "" || devs["2"].Friendly != "" || devs["1"].Published() != "id-1" {
		t.Fatalf("colliding device labels: %+v %+v", devs["1"], devs["2"])
	}
	if devs["3"].Published() != "grafana" {
		t.Fatal(devs["3"])
	}
	if devs["4"].Friendly != "" {
		t.Fatal("a hostname that looks like a stable label must not be published as friendly")
	}
}

func TestNameRoundTrip(t *testing.T) {
	name := naming.Name("postgres", "home")
	if name != "postgres.home.flavor.internal" {
		t.Fatal(name)
	}
	d, n, ok := naming.Split(name)
	if !ok || d != "postgres" || n != "home" {
		t.Fatal(d, n, ok)
	}
	for _, bad := range []string{"x.flavor.internal", "a.b.c.flavor.internal", "-a.home.flavor.internal", "postgres.home.example.com"} {
		if _, _, ok := naming.Split(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}

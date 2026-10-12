package netd

import (
	"net/netip"
	"testing"
)

func TestNRPTServerList(t *testing.T) {
	cases := []struct {
		name    string
		servers []netip.Addr
		want    string
	}{
		{"ipv6 and ipv4", []netip.Addr{netip.MustParseAddr("fd56:21ec:22c6::53"), netip.MustParseAddr("198.19.240.2")}, "fd56:21ec:22c6::53;198.19.240.2"},
		{"one address", []netip.Addr{netip.MustParseAddr("198.19.240.2")}, "198.19.240.2"},
		{"none", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nrptServerList(c.servers); got != c.want {
				t.Fatalf("nrptServerList = %q, want %q", got, c.want)
			}
		})
	}
}

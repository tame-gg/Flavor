package synthetic_test

import (
	"errors"
	"net/netip"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/synthetic"
)

var ula = netip.MustParsePrefix("fd12:3456:789a::/48")

func TestNewULAIsRandomRFC4193(t *testing.T) {
	a, err := synthetic.NewULA()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := synthetic.NewULA()
	if !synthetic.ValidULA(a) || !synthetic.ValidULA(b) || a == b {
		t.Fatalf("%v %v", a, b)
	}
	for _, bad := range []string{"fc00::/48", "fd12:3456:789a::/56", "2001:db8::/48", "fd12:3456:789a:1::/48"} {
		p, _ := netip.ParsePrefix(bad)
		if synthetic.ValidULA(p) {
			t.Fatalf("%s accepted", bad)
		}
	}
}

func TestNetworkPrefixesAreUniquePerIndex(t *testing.T) {
	seen := map[netip.Prefix]bool{}
	for _, i := range []uint16{1, 2, 255, 256, 65534} {
		p, err := synthetic.NetworkPrefix(ula, i)
		if err != nil || p.Bits() != 64 || !ula.Contains(p.Addr()) || seen[p] {
			t.Fatalf("%d: %v %v", i, p, err)
		}
		seen[p] = true
	}
	for _, i := range []uint16{0, 0xFFFF} {
		if _, err := synthetic.NetworkPrefix(ula, i); !errors.Is(err, synthetic.ErrInvalidIndex) {
			t.Fatalf("index %#x is reserved", i)
		}
	}
	if got := synthetic.ResolverAddress(ula).String(); got != "fd12:3456:789a::53" {
		t.Fatal(got)
	}
}

func TestEmbeddedV4RoundTrip(t *testing.T) {
	a, err := synthetic.EmbedV4(ula, 7, netip.MustParseAddr("100.64.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != "fd12:3456:789a:7::6440:1" {
		t.Fatal(a)
	}
	d, err := synthetic.Decode(ula, a)
	if err != nil || d.Kind != synthetic.KindEmbeddedV4 || d.Index != 7 || d.V4.String() != "100.64.0.1" {
		t.Fatalf("%+v %v", d, err)
	}
	other, _ := synthetic.EmbedV4(ula, 8, netip.MustParseAddr("100.64.0.1"))
	if other == a {
		t.Fatal("the same IPv4 on two networks must get two synthetic addresses")
	}
	if _, err := synthetic.EmbedV4(ula, 7, netip.MustParseAddr("fd7a::1")); !errors.Is(err, synthetic.ErrNotIPv4) {
		t.Fatal(err)
	}
}

func TestAllocatedV6RoundTripAndNamespacesAreDisjoint(t *testing.T) {
	for _, counter := range []uint64{1, 2, 1 << 32, 1<<48 - 1} {
		a, err := synthetic.AllocatedV6(ula, 3, counter)
		if err != nil {
			t.Fatal(err)
		}
		d, err := synthetic.Decode(ula, a)
		if err != nil || d.Kind != synthetic.KindAllocatedV6 || d.Counter != counter || d.Index != 3 {
			t.Fatalf("%d: %+v %v", counter, d, err)
		}
		for _, v4 := range []string{"0.0.0.0", "0.0.0.1", "255.255.255.255", "100.64.0.1"} {
			e, _ := synthetic.EmbedV4(ula, 3, netip.MustParseAddr(v4))
			if e == a {
				t.Fatalf("allocated %v collides with embedded %s", a, v4)
			}
		}
	}
	if _, err := synthetic.AllocatedV6(ula, 3, 0); !errors.Is(err, synthetic.ErrCounterSpent) {
		t.Fatal("counter 0 is not allocatable")
	}
	if _, err := synthetic.AllocatedV6(ula, 3, 1<<48); !errors.Is(err, synthetic.ErrCounterSpent) {
		t.Fatal("counter overflow must be explicit")
	}
}

func TestDecodeRejectsForeignAndReservedAddresses(t *testing.T) {
	for _, raw := range []string{
		"fd99::1",
		"100.64.0.1",
		"fd12:3456:789a:0::6440:1",
		"fd12:3456:789a:7:2::1",
		"fd12:3456:789a:7:0:1:6440:1",
		"fd12:3456:789a:7:1::",
	} {
		if _, err := synthetic.Decode(ula, netip.MustParseAddr(raw)); err == nil {
			t.Fatalf("%s decoded", raw)
		}
	}
}

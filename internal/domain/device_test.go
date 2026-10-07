package domain_test

import (
	"net/netip"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

func TestDeviceIdentityNotByIP(t *testing.T) {
	a := domain.DeviceIdentity{NetworkID: "netA", NodeID: "alpha"}
	b := domain.DeviceIdentity{NetworkID: "netB", NodeID: "beta"}
	if a == b {
		t.Fatal("identities must differ")
	}
	da := domain.Device{ID: a, Addresses: []netip.Addr{netip.MustParseAddr("100.64.0.1")}}
	db := domain.Device{ID: b, Addresses: []netip.Addr{netip.MustParseAddr("100.64.0.1")}}
	if da.ID == db.ID {
		t.Fatal("same IP must not unify identity")
	}
	if da.ID.Key() == db.ID.Key() {
		t.Fatal("device keys must differ across networks")
	}
}

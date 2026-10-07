package synthetic

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (string, *store.DB, *clock, []domain.NetworkID) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lattice.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var ids []domain.NetworkID
	for _, name := range []string{"LunarLabs", "Home"} {
		now := time.Now().UTC()
		n := domain.Network{ID: domain.NewNetworkID(), DisplayName: name, Provider: domain.ProviderTailscale, NodeHostname: "ws", CreatedAt: now, UpdatedAt: now}
		if err := db.Networks().Create(context.Background(), n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	return path, db, &clock{t: time.Unix(1_800_000_000, 0)}, ids
}

func reopen(t *testing.T, path string, db *store.DB) *store.DB {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestULAAndIndicesPersist(t *testing.T) {
	ctx := context.Background()
	path, db, c, ids := setup(t)
	a, err := Open(ctx, db, c.now)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := a.For(ctx, ids[0], netip.MustParseAddr("100.64.0.1"))
	second, _ := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1"))
	if first.V6 == second.V6 {
		t.Fatal("same real address on two networks collapsed")
	}

	db = reopen(t, path, db)
	b, err := Open(ctx, db, c.now)
	if err != nil {
		t.Fatal(err)
	}
	if b.ULA() != a.ULA() {
		t.Fatal("ULA regenerated on restart")
	}
	again, _ := b.For(ctx, ids[0], netip.MustParseAddr("100.64.0.1"))
	if again.V6 != first.V6 {
		t.Fatal("network index or embedding not stable across restart")
	}
	net, real, err := b.Resolve(ctx, again.V6)
	if err != nil || net != ids[0] || real.String() != "100.64.0.1" {
		t.Fatalf("%v %v %v", net, real, err)
	}
	if err := db.Networks().Update(ctx, renamed(t, db, ids[0], "Personal")); err != nil {
		t.Fatal(err)
	}
	if renamedAddr, _ := b.For(ctx, ids[0], netip.MustParseAddr("100.64.0.1")); renamedAddr.V6 != first.V6 {
		t.Fatal("renaming a network changed its synthetic prefix")
	}
}

func renamed(t *testing.T, db *store.DB, id domain.NetworkID, name string) domain.Network {
	n, err := db.Networks().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	n.DisplayName = name
	return n
}

func TestIPv6TargetsUseTheAllocatedNamespace(t *testing.T) {
	ctx := context.Background()
	_, db, c, ids := setup(t)
	a, _ := Open(ctx, db, c.now)
	real := netip.MustParseAddr("fd7a:115c:a1e0::1")
	x, err := a.For(ctx, ids[0], real)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(a.ULA(), x.V6)
	if err != nil || d.Kind != KindAllocatedV6 || d.Counter != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	y, _ := a.For(ctx, ids[0], real)
	z, _ := a.For(ctx, ids[0], netip.MustParseAddr("fd7a:115c:a1e0::2"))
	if y.V6 != x.V6 || z.V6 == x.V6 {
		t.Fatal("v6 allocation not stable or not unique")
	}
	if net, back, err := a.Resolve(ctx, z.V6); err != nil || net != ids[0] || back.String() != "fd7a:115c:a1e0::2" {
		t.Fatalf("%v %v %v", net, back, err)
	}
}

func TestIPv4AllocationReclaimAndQuarantine(t *testing.T) {
	ctx := context.Background()
	_, db, c, ids := setup(t)
	a, _ := Open(ctx, db, c.now)
	if err := a.SetPool(ctx, netip.MustParsePrefix("10.0.0.0/24")); !errors.Is(err, ErrPoolOutside) {
		t.Fatal("pool outside 198.18.0.0/15 accepted")
	}
	if err := a.SetPool(ctx, netip.MustParsePrefix("198.19.255.252/30")); !errors.Is(err, ErrPoolOutside) {
		t.Fatal("a pool with no mappable address accepted")
	}
	if err := a.SetPool(ctx, netip.MustParsePrefix("198.19.255.240/28")); err != nil {
		t.Fatal(err)
	}
	got := map[netip.Addr]string{}
	for i := range 12 {
		r := fmt.Sprintf("100.64.0.%d", i+1)
		c.t = c.t.Add(time.Minute)
		x, err := a.For(ctx, ids[0], netip.MustParseAddr(r))
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		if !a.Pool().Contains(x.V4) || got[x.V4] != "" {
			t.Fatalf("%s got %v (taken by %q)", r, x.V4, got[x.V4])
		}
		got[x.V4] = r
		if i == 0 && x.V4.String() != "198.19.255.243" {
			t.Fatalf("network, host and resolver addresses must be skipped: %v", x.V4)
		}
	}
	other, err := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1"))
	if !errors.Is(err, store.ErrPoolExhausted) {
		t.Fatalf("pool exhaustion must be explicit: %v %v", other, err)
	}

	oldest, _ := a.For(ctx, ids[0], netip.MustParseAddr("100.64.0.2"))
	release := a.Acquire(mustV4(t, a, ids[0], "100.64.0.3"))
	c.t = c.t.Add(48 * time.Hour)
	_, _ = a.For(ctx, ids[0], netip.MustParseAddr("100.64.0.6"))
	reclaimed, err := a.ReclaimIdle(ctx, 24*time.Hour, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 2 || reclaimed[0].String() != "198.19.255.243" {
		t.Fatalf("least recently used first, skipping active flows: %v", reclaimed)
	}
	for _, r := range reclaimed {
		if r == oldest.V4 || r == mustV4(t, a, ids[0], "100.64.0.3") {
			t.Fatalf("reclaimed a recently used or active mapping: %v", r)
		}
	}
	if err := a.Release(ctx, mustV4(t, a, ids[0], "100.64.0.3")); !errors.Is(err, ErrInUse) {
		t.Fatalf("release while flows are open: %v", err)
	}
	release()
	release()

	if _, err := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1")); !errors.Is(err, store.ErrPoolExhausted) {
		t.Fatal("quarantined addresses were reused immediately")
	}
	until, ok, _ := db.Synthetic().Quarantined(ctx, reclaimed[0])
	if !ok || !until.Equal(c.t.Add(store.V4Quarantine)) {
		t.Fatalf("quarantine deadline %v", until)
	}

	c.t = c.t.Add(store.V4Quarantine + time.Second)
	fresh, err := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1"))
	if err != nil || !a.Pool().Contains(fresh.V4) {
		t.Fatalf("address must be reusable once its quarantine has passed: %v %v", fresh, err)
	}
}

func TestNetworkRemovalQuarantinesDurably(t *testing.T) {
	ctx := context.Background()
	path, db, c, ids := setup(t)
	c.t = time.Now()
	a, _ := Open(ctx, db, c.now)
	if err := a.SetPool(ctx, netip.MustParsePrefix("198.19.255.248/29")); err != nil {
		t.Fatal(err)
	}
	gone, err := a.For(ctx, ids[0], netip.MustParseAddr("100.64.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"100.64.0.2", "100.64.0.3", "100.64.0.4"} {
		if _, err := a.For(ctx, ids[0], netip.MustParseAddr(r)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SoftRemove(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	db = reopen(t, path, db)
	a, _ = Open(ctx, db, c.now)
	if _, err := db.Synthetic().Lookup(ctx, gone.V4); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("mapping of a removed network survived")
	}
	if _, ok, _ := db.Synthetic().Quarantined(ctx, gone.V4); !ok {
		t.Fatal("network removal must quarantine its addresses, durably")
	}
	c.t = c.t.Add(store.V4Quarantine - time.Minute)
	if _, err := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1")); !errors.Is(err, store.ErrPoolExhausted) {
		t.Fatal("addresses of a removed network reused before the deadline")
	}
	c.t = c.t.Add(2 * time.Minute)
	if x, err := a.For(ctx, ids[1], netip.MustParseAddr("100.64.0.1")); err != nil || !a.Pool().Contains(x.V4) {
		t.Fatalf("reusable after the deadline: %v %v", x, err)
	}
}

func mustV4(t *testing.T, a *Allocator, id domain.NetworkID, real string) netip.Addr {
	t.Helper()
	x, err := a.For(context.Background(), id, netip.MustParseAddr(real))
	if err != nil {
		t.Fatal(err)
	}
	return x.V4
}

func TestNetworkIndexQuarantine(t *testing.T) {
	ctx := context.Background()
	_, db, c, ids := setup(t)
	syn := db.Synthetic()
	first, _ := syn.NetworkIndex(ctx, ids[0], c.now())
	second, _ := syn.NetworkIndex(ctx, ids[1], c.now())
	if first != 1 || second != 2 {
		t.Fatal(first, second)
	}
	if err := db.SoftRemove(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	n := domain.Network{ID: domain.NewNetworkID(), DisplayName: "New", Provider: domain.ProviderTailscale, NodeHostname: "ws", CreatedAt: now, UpdatedAt: now}
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if idx, _ := syn.NetworkIndex(ctx, n.ID, time.Now()); idx == first {
		t.Fatal("a removed network's index was handed out again immediately")
	}
	other := domain.Network{ID: domain.NewNetworkID(), DisplayName: "Later", Provider: domain.ProviderTailscale, NodeHostname: "ws", CreatedAt: now, UpdatedAt: now}
	if err := db.Networks().Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	if idx, _ := syn.NetworkIndex(ctx, other.ID, time.Now().Add(store.IndexQuarantine+time.Hour)); idx != first {
		t.Fatalf("index should be reusable after quarantine, got %d", idx)
	}
}

func TestEnsurePool(t *testing.T) {
	ctx := context.Background()
	_, db, c, _ := setup(t)
	a, _ := Open(ctx, db, c.now)
	if err := a.EnsurePool(ctx, []netip.Prefix{netip.MustParsePrefix("198.19.240.0/24")}); err != nil {
		t.Fatal(err)
	}
	if got := a.Pool().String(); got != "198.19.224.0/20" {
		t.Fatalf("pool must avoid local prefixes: %s", got)
	}
	if HostV4(a.Pool()).String() != "198.19.224.1" || ResolverV4(a.Pool()).String() != "198.19.224.2" {
		t.Fatal("host and resolver are the first two hosts of the pool")
	}
	if err := a.EnsurePool(ctx, nil); err != nil || a.Pool().String() != "198.19.224.0/20" {
		t.Fatalf("a stored pool is kept: %v %v", a.Pool(), err)
	}
	if err := a.EnsurePool(ctx, []netip.Prefix{netip.MustParsePrefix("198.19.230.0/24")}); !errors.Is(err, ErrPoolOverlaps) || a.Pool().IsValid() {
		t.Fatalf("a stored pool that now overlaps must disable IPv4: %v %v", a.Pool(), err)
	}
	b, _ := Open(ctx, db, c.now)
	if b.Pool().String() != "198.19.224.0/20" {
		t.Fatal("disabling IPv4 for one run must not forget the stored pool")
	}
}

func TestChoosePool(t *testing.T) {
	p, err := ChoosePool([]netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("198.19.240.10/32")})
	if err != nil || p.String() != "198.19.224.0/20" {
		t.Fatalf("%v %v", p, err)
	}
	if p, _ := ChoosePool(nil); p.String() != "198.19.240.0/20" {
		t.Fatal(p)
	}
	if _, err := ChoosePool([]netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}); !errors.Is(err, ErrPoolOverlaps) {
		t.Fatal("whole compatibility range in use must refuse, not shadow")
	}
	clash, _ := ChoosePool([]netip.Prefix{netip.MustParsePrefix("198.18.0.0/16")})
	if !CompatibilityRange.Contains(clash.Addr()) || clash.Overlaps(netip.MustParsePrefix("198.18.0.0/16")) {
		t.Fatal(clash)
	}
}

func TestParseKernelRoutes(t *testing.T) {
	v4 := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0102A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0002A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n" +
		"utun\t0000F0C6\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	got := parseRoute4(strings.NewReader(v4))
	if len(got) != 3 || got[1].String() != "192.168.2.0/24" || got[2].String() != "198.240.0.0/16" {
		t.Fatalf("%v", got)
	}
	v6 := "fd7a115ca1e000000000000000000000 30 00000000000000000000000000000000 00 00000000000000000000000000000000 00000000 00000001 00000000 00000001 tailscale0\n"
	if got := parseRoute6(strings.NewReader(v6)); len(got) != 1 || got[0].String() != "fd7a:115c:a1e0::/48" {
		t.Fatalf("%v", got)
	}
}

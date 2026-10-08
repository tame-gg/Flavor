package store_test

import (
	"context"
	"errors"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
)

func TestDestinationPreferences(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	a := sampleNetwork(domain.ProviderHeadscale, "LunarLabs", "ws", "https://a.example.com")
	b := sampleNetwork(domain.ProviderTailscale, "Home", "ws", "")
	for _, n := range []domain.Network{a, b} {
		if err := db.Networks().Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	prefs := db.Preferences()
	if err := prefs.Set(ctx, domain.DestinationPreference{Destination: "100.64.0.1", Kind: domain.DestinationAddress, NetworkID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := prefs.Set(ctx, domain.DestinationPreference{Destination: "100.64.0.1", Kind: domain.DestinationAddress, NetworkID: b.ID}); err != nil {
		t.Fatal(err)
	}
	p, err := prefs.Get(ctx, "100.64.0.1")
	if err != nil || p.NetworkID != b.ID || p.CreatedAt.IsZero() {
		t.Fatalf("upsert: %+v %v", p, err)
	}
	if err := prefs.Set(ctx, domain.DestinationPreference{Destination: "db", Kind: domain.DestinationName, NetworkID: "01NOSUCHNETWORK000000000000"}); !errors.Is(err, store.ErrInvalidInput) {
		t.Fatalf("unknown network accepted: %v", err)
	}
	if err := prefs.Set(ctx, domain.DestinationPreference{Destination: "db", Kind: "cidr", NetworkID: a.ID}); !errors.Is(err, store.ErrInvalidInput) {
		t.Fatalf("unknown kind accepted: %v", err)
	}
	if err := prefs.Set(ctx, domain.DestinationPreference{Destination: "postgres", Kind: domain.DestinationName, NetworkID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := prefs.ForNetwork(ctx, a.ID); len(got) != 1 || got[0].Destination != "postgres" {
		t.Fatalf("%+v", got)
	}
	if err := db.SoftRemove(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	all, _ := prefs.List(ctx)
	if len(all) != 1 || all[0].Destination != "100.64.0.1" {
		t.Fatalf("preferences for a removed network must go with it: %+v", all)
	}
	if ok, err := prefs.Delete(ctx, "100.64.0.1"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _ := prefs.Delete(ctx, "100.64.0.1"); ok {
		t.Fatal("second delete reported a row")
	}
}

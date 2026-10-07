package secret_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

func TestMemoryStoreCRUD(t *testing.T) {
	ctx := context.Background()
	store := secret.NewMemoryStore()
	st := store.Status(ctx)
	if st.Backend != secret.BackendMemory || st.State != secret.StateMemory {
		t.Fatalf("status=%+v", st)
	}
	if st.Persistent() {
		t.Fatal("memory must not be persistent")
	}
	idA := domain.NewNetworkID()
	idB := domain.NewNetworkID()
	refA, err := secret.NetworkRef(idA, "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	refB, err := secret.NetworkRef(idB, "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, refA, secret.New("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, refB, secret.New("beta")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, refA)
	if err != nil || got.Reveal() != "alpha" {
		t.Fatalf("get A: %v %v", got, err)
	}
	if err := store.Delete(ctx, refA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, refA); !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	gotB, err := store.Get(ctx, refB)
	if err != nil || gotB.Reveal() != "beta" {
		t.Fatalf("B must remain: %v %v", gotB, err)
	}
	if err := store.Delete(ctx, refA); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryStoreMissingGet(t *testing.T) {
	ctx := context.Background()
	store := secret.NewMemoryStore()
	ref, err := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestMemoryStoreConcurrent(t *testing.T) {
	ctx := context.Background()
	store := secret.NewMemoryStore()
	ref, err := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = store.Set(ctx, ref, secret.New("v"))
			_, _ = store.Get(ctx, ref)
			if i%2 == 0 {
				_ = store.Delete(ctx, ref)
			}
		}(i)
	}
	wg.Wait()
}

func TestMemoryStoreNotSharedAcrossInstances(t *testing.T) {
	ctx := context.Background()
	a := secret.NewMemoryStore()
	b := secret.NewMemoryStore()
	ref, err := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Set(ctx, ref, secret.New("only-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(ctx, ref); !errors.Is(err, secret.ErrNotFound) {
		t.Fatal("new instance must be empty")
	}
}

package secret_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
)

type fakeKeyring struct {
	mu     sync.Mutex
	data   map[string]string
	getErr error
	setErr error
	delErr error
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{data: make(map[string]string)}
}

func (f *fakeKeyring) key(service, user string) string { return service + "\x00" + user }

func (f *fakeKeyring) Get(service, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.data[f.key(service, user)]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Set(service, user, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.data[f.key(service, user)] = password
	return nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delErr != nil {
		return f.delErr
	}
	k := f.key(service, user)
	if _, ok := f.data[k]; !ok {
		return secret.ErrNotFound
	}
	delete(f.data, k)
	return nil
}

func TestSecretServiceViaFake(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKeyring()
	store := secret.NewSecretServiceForTest("dev.lunarlabs.lattice", fk)
	st := store.Status(ctx)
	if st.Backend != secret.BackendSecretService || st.State != secret.StateAvailable {
		t.Fatalf("status=%+v", st)
	}
	if !st.Persistent() {
		t.Fatal("available secret service should be persistent")
	}
	idA := domain.NewNetworkID()
	idB := domain.NewNetworkID()
	refA, _ := secret.NetworkRef(idA, "provider-token")
	refB, _ := secret.NetworkRef(idB, "provider-token")
	if err := store.Set(ctx, refA, secret.New("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, refB, secret.New("beta")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, refA)
	if err != nil || got.Reveal() != "alpha" {
		t.Fatalf("%v %v", got, err)
	}
	if err := store.Delete(ctx, refA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, refA); !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	gotB, err := store.Get(ctx, refB)
	if err != nil || gotB.Reveal() != "beta" {
		t.Fatalf("B must remain: %v %v", gotB, err)
	}
}

func TestSecretServiceMapsUnavailable(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKeyring()
	fk.getErr = errors.New("dbus: connection refused")
	fk.setErr = errors.New("dbus: connection refused")
	store := secret.NewSecretServiceForTest("dev.lunarlabs.lattice", fk)
	st := store.Status(ctx)
	if st.State != secret.StateUnavailable {
		t.Fatalf("status=%+v", st)
	}
	ref, _ := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	if err := store.Set(ctx, ref, secret.New("x")); !errors.Is(err, secret.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestSecretServiceUnlockTextIsUnavailableNotLocked(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKeyring()
	fk.getErr = errors.New("failed to unlock correct collection")
	store := secret.NewSecretServiceForTest("dev.lunarlabs.lattice", fk)
	st := store.Status(ctx)
	if st.State != secret.StateUnavailable {
		t.Fatalf("status=%+v want unavailable", st)
	}
}

func TestSecretServiceMapsExplicitLocked(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKeyring()
	fk.getErr = secret.ErrLocked
	store := secret.NewSecretServiceForTest("dev.lunarlabs.lattice", fk)
	st := store.Status(ctx)
	if st.State != secret.StateLocked {
		t.Fatalf("status=%+v", st)
	}
}

func TestOpenAutoFallsBackToMemory(t *testing.T) {
	ctx := context.Background()
	store, err := secret.Open(ctx, secret.Options{Mode: secret.ModeMemory})
	if err != nil {
		t.Fatal(err)
	}
	st := store.Status(ctx)
	if st.Backend != secret.BackendMemory {
		t.Fatalf("%+v", st)
	}
}

func TestNoPlaintextFallbackFiles(t *testing.T) {
	ctx := context.Background()
	store := secret.NewMemoryStore()
	ref, _ := secret.NetworkRef(domain.NewNetworkID(), "provider-token")
	canary := "LATTICE_TEST_SECRET_DO_NOT_LEAK_7f3c9a"
	if err := store.Set(ctx, ref, secret.New(canary)); err != nil {
		t.Fatal(err)
	}
	st := store.Status(ctx)
	if st.Persistent() {
		t.Fatal("memory must not claim persistence")
	}
}

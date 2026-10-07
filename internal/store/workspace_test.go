package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
)

func TestWorkspaceLifecycleAndMembership(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lattice.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	a := sampleNetwork(domain.ProviderHeadscale, "LunarLabs", "ws", "https://a.example.com")
	b := sampleNetwork(domain.ProviderTailscale, "Home", "ws", "")
	for _, n := range []domain.Network{a, b} {
		if err := db.Networks().Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	w := domain.Workspace{ID: domain.NewWorkspaceID(), Name: "On Call", Description: "pager rotation", NetworkIDs: []domain.NetworkID{a.ID, b.ID, a.ID}}
	if err := db.Workspaces().Create(ctx, w); err != nil {
		t.Fatal(err)
	}
	got, err := db.Workspaces().Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "On Call" || len(got.NetworkIDs) != 2 || got.CreatedAt.IsZero() {
		t.Fatalf("%+v", got)
	}

	bad := domain.Workspace{ID: domain.NewWorkspaceID(), Name: "Ghost", NetworkIDs: []domain.NetworkID{"01NOSUCHNETWORK000000000000"}}
	if err := db.Workspaces().Create(ctx, bad); !errors.Is(err, store.ErrInvalidInput) {
		t.Fatalf("unknown network accepted: %v", err)
	}
	if _, err := db.Workspaces().Get(ctx, bad.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("failed create left a workspace behind")
	}
	if err := db.Workspaces().Create(ctx, domain.Workspace{ID: domain.NewWorkspaceID(), Name: " "}); !errors.Is(err, store.ErrInvalidInput) {
		t.Fatalf("blank name accepted: %v", err)
	}

	w.Name, w.NetworkIDs = "Work", []domain.NetworkID{b.ID}
	if err := db.Workspaces().Update(ctx, w); err != nil {
		t.Fatal(err)
	}
	got, _ = db.Workspaces().Get(ctx, w.ID)
	if got.Name != "Work" || len(got.NetworkIDs) != 1 || got.NetworkIDs[0] != b.ID {
		t.Fatalf("%+v", got)
	}
	if err := db.Workspaces().Update(ctx, domain.Workspace{ID: domain.NewWorkspaceID(), Name: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update of missing workspace: %v", err)
	}

	if err := db.Workspaces().SetActive(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.Workspaces().Active(ctx); active != w.ID {
		t.Fatalf("active=%q", active)
	}
	if err := db.Workspaces().SetActive(ctx, domain.NewWorkspaceID()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("activating a missing workspace: %v", err)
	}

	if err := db.SoftRemove(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err = db.Workspaces().Get(ctx, w.ID)
	if err != nil || len(got.NetworkIDs) != 0 {
		t.Fatalf("removing a network must drop membership but keep the workspace: %+v %v", got, err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if active, _ := db.Workspaces().Active(ctx); active != w.ID {
		t.Fatal("active workspace not persisted")
	}
	if err := db.Workspaces().Delete(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.Workspaces().Active(ctx); active != "" {
		t.Fatal("deleting the active workspace must clear it")
	}
	if err := db.Workspaces().Delete(ctx, w.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

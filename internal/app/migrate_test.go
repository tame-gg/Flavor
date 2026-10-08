package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
)

func TestLatticeDataMigrated(t *testing.T) {
	env := testEnv(t)
	first := start(t, env, app.Options{EngineFactory: (&sessiontest.Sequence{}).Factory})
	n := addHeadscale(t, first.client, "Office", "https://office.example.com", false)
	tsnet, err := first.paths.TsnetDir(domain.NetworkID(n.Id))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tsnet, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tsnet, "tailscaled.state"), []byte("node-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	first.stop()

	oldData := filepath.Join(env.DataHome, "lattice")
	if err := os.Rename(first.paths.Data, oldData); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(oldData, "database"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		rest, _ := strings.CutPrefix(e.Name(), "flavor.db")
		if err := os.Rename(filepath.Join(oldData, "database", e.Name()), filepath.Join(oldData, "database", "lattice.db"+rest)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(first.paths.Config, filepath.Join(env.ConfigHome, "lattice")); err != nil {
		t.Fatal(err)
	}

	second := start(t, env, app.Options{EngineFactory: (&sessiontest.Sequence{}).Factory})
	nets := snapshot(t, second.client).Networks
	if len(nets) != 1 || nets[0].Id != n.Id || nets[0].DisplayName != "Office" {
		t.Fatalf("networks after migration: %+v", nets)
	}
	if b, err := os.ReadFile(filepath.Join(tsnet, "tailscaled.state")); err != nil || string(b) != "node-key" {
		t.Fatalf("identity after migration: %q %v", b, err)
	}
	for _, old := range []string{oldData, filepath.Join(env.ConfigHome, "lattice")} {
		if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still present: %v", old, err)
		}
	}
}

func TestMigrationRefusedWhileLatticeRuns(t *testing.T) {
	env := testEnv(t)
	oldDB := filepath.Join(env.DataHome, "lattice", "database")
	if err := os.MkdirAll(oldDB, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDB, "lattice.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(env.RuntimeDir, "lattice"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(env.RuntimeDir, "lattice", "latticed.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = app.Run(ctx, app.Options{Env: &env, SecretMode: secret.ModeMemory})
	if !errors.Is(err, app.ErrLatticeRunning) {
		t.Fatalf("got %v want ErrLatticeRunning", err)
	}
	if _, err := os.Stat(filepath.Join(oldDB, "lattice.db")); err != nil {
		t.Fatalf("Lattice database moved while latticed ran: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(env.DataHome, "flavor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("flavor data created while latticed ran: %v", err)
	}
}

//go:build !windows

package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
)

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

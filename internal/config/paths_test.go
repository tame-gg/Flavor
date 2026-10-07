package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/config"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

func TestResolveXDGDefaults(t *testing.T) {
	home := t.TempDir()
	runtime := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := config.Resolve(config.Env{
		Home:       home,
		DataHome:   "",
		ConfigHome: "",
		RuntimeDir: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Data != filepath.Join(home, ".local", "share", "lattice") {
		t.Fatalf("data=%s", p.Data)
	}
	if p.Config != filepath.Join(home, ".config", "lattice") {
		t.Fatalf("config=%s", p.Config)
	}
	if p.Runtime != filepath.Join(runtime, "lattice") {
		t.Fatalf("runtime=%s", p.Runtime)
	}
	if p.Database != filepath.Join(p.Data, "database", "lattice.db") {
		t.Fatalf("db=%s", p.Database)
	}
	if p.Socket != filepath.Join(p.Runtime, "latticed.sock") {
		t.Fatalf("sock=%s", p.Socket)
	}
}

func TestRuntimeDirOverride(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "custom-run")
	if err := os.MkdirAll(override, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := config.Resolve(config.Env{
		Home:            home,
		RuntimeDir:      filepath.Join(t.TempDir(), "ignored"),
		RuntimeOverride: override,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Runtime != filepath.Join(override, "lattice") {
		t.Fatalf("runtime=%s", p.Runtime)
	}
}

func TestMissingRuntimeDirFails(t *testing.T) {
	_, err := config.Resolve(config.Env{
		Home:       t.TempDir(),
		RuntimeDir: "",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWrongOwnerRuntimeFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root")
	}
	runtime := t.TempDir()
	if err := os.Chown(runtime, 0, 0); err != nil {
		t.Skip("cannot chown to root in this environment")
	}
	_, err := config.Resolve(config.Env{
		Home:       t.TempDir(),
		RuntimeDir: runtime,
	})
	if err == nil {
		t.Fatal("expected ownership error")
	}
}

func TestNetworkDirRejectsTraversal(t *testing.T) {
	home := t.TempDir()
	runtime := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := config.Resolve(config.Env{Home: home, RuntimeDir: runtime})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.NetworkDir(domain.NetworkID("../escape")); err == nil {
		t.Fatal("expected traversal reject")
	}
	if _, err := p.NetworkDir(domain.NetworkID("foo/bar")); err == nil {
		t.Fatal("expected slash reject")
	}
}

func TestNetworkDirStableAcrossRename(t *testing.T) {
	home := t.TempDir()
	runtime := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := config.Resolve(config.Env{Home: home, RuntimeDir: runtime})
	if err != nil {
		t.Fatal(err)
	}
	id := domain.NewNetworkID()
	dir1, err := p.NetworkDir(id)
	if err != nil {
		t.Fatal(err)
	}
	dir2, err := p.NetworkDir(id)
	if err != nil {
		t.Fatal(err)
	}
	if dir1 != dir2 {
		t.Fatal("path must be stable")
	}
	if filepath.Base(dir1) != string(id) {
		t.Fatalf("path base must be network id, got %s", filepath.Base(dir1))
	}
	displayName := "LunarLabs"
	_ = displayName
	if filepath.Base(dir1) == displayName {
		t.Fatal("display name must not affect path")
	}
}

func TestEnsureLatticeDirsPermissions(t *testing.T) {
	home := t.TempDir()
	runtime := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := config.Resolve(config.Env{Home: home, RuntimeDir: runtime})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureLatticeDirs(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{p.Data, p.Config, p.Runtime, p.NetworksRoot, filepath.Dir(p.Database)} {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode=%o", dir, st.Mode().Perm())
		}
	}
	id := domain.NewNetworkID()
	netDir, err := p.EnsureNetworkDirs(id)
	if err != nil {
		t.Fatal(err)
	}
	tsnet := filepath.Join(netDir, "tsnet")
	st, err := os.Stat(tsnet)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("tsnet mode=%o", st.Mode().Perm())
	}
}

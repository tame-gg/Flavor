package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.lunarlabs.dev/lattice/lattice/internal/config"
)

func TestDesktopConfigDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg, err := config.LoadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || cfg.UI.Theme != config.ThemeSystem || cfg.Desktop.CloseBehavior != config.CloseTray {
		t.Fatalf("defaults=%+v", cfg)
	}
	cfg.UI.Theme = config.ThemeDark
	cfg.Desktop.CloseBehavior = config.CloseQuitGUI
	if err := config.SaveDesktopConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", st.Mode().Perm())
	}
	loaded, err := config.LoadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UI.Theme != config.ThemeDark || loaded.Desktop.CloseBehavior != config.CloseQuitGUI {
		t.Fatalf("loaded=%+v", loaded)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "auth_key") || strings.Contains(s, "pre_auth") {
		t.Fatalf("config must not contain credentials: %s", s)
	}
}

func TestDesktopConfigInvalidThemeFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("version = 1\n[ui]\ntheme = \"drak\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Theme != config.ThemeSystem {
		t.Fatalf("theme=%s", cfg.UI.Theme)
	}
}

func TestDesktopConfigMalformedTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("version = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected safe defaults on malformed toml, got %+v", cfg)
	}
}

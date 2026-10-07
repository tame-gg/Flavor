package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

type CloseBehavior string

const (
	CloseTray    CloseBehavior = "tray"
	CloseQuitGUI CloseBehavior = "quit_gui"
)

type DesktopConfig struct {
	Version int          `toml:"version"`
	Desktop DesktopPrefs `toml:"desktop"`
	UI      UIPrefs      `toml:"ui"`
}

type DesktopPrefs struct {
	CloseBehavior CloseBehavior `toml:"close_behavior"`
}

type UIPrefs struct {
	Theme Theme `toml:"theme"`
}

func DefaultDesktopConfig() DesktopConfig {
	return DesktopConfig{
		Version: 1,
		Desktop: DesktopPrefs{CloseBehavior: CloseTray},
		UI:      UIPrefs{Theme: ThemeSystem},
	}
}

func LoadDesktopConfig(path string) (DesktopConfig, error) {
	cfg := DefaultDesktopConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var parsed DesktopConfig
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		return cfg, nil
	}
	cfg = DefaultDesktopConfig()
	if parsed.Version != 0 {
		cfg.Version = parsed.Version
	}
	if cfg.Version != 1 {
		cfg.Version = 1
	}
	cfg.UI.Theme = normalizeTheme(parsed.UI.Theme)
	cfg.Desktop.CloseBehavior = normalizeClose(parsed.Desktop.CloseBehavior)
	return cfg, nil
}

func SaveDesktopConfig(path string, cfg DesktopConfig) error {
	cfg.Version = 1
	cfg.UI.Theme = normalizeTheme(cfg.UI.Theme)
	cfg.Desktop.CloseBehavior = normalizeClose(cfg.Desktop.CloseBehavior)
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return err
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

func normalizeTheme(t Theme) Theme {
	switch t {
	case ThemeSystem, ThemeLight, ThemeDark:
		return t
	default:
		return ThemeSystem
	}
}

func normalizeClose(c CloseBehavior) CloseBehavior {
	switch c {
	case CloseTray, CloseQuitGUI:
		return c
	default:
		return CloseTray
	}
}

func ValidateDesktopConfig(cfg DesktopConfig) error {
	switch cfg.UI.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("invalid theme %q", cfg.UI.Theme)
	}
	switch cfg.Desktop.CloseBehavior {
	case CloseTray, CloseQuitGUI:
	default:
		return fmt.Errorf("invalid close_behavior %q", cfg.Desktop.CloseBehavior)
	}
	if cfg.Version != 1 {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	return nil
}

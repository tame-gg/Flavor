package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type Paths struct {
	Home         string
	Data         string
	Config       string
	Runtime      string
	Database     string
	NetworksRoot string
	Socket       string
	Lock         string
}

type Env struct {
	Home            string
	DataHome        string
	ConfigHome      string
	RuntimeDir      string
	RuntimeOverride string
}

var (
	ErrRuntimeDirRequired = errors.New("runtime directory required")
	ErrInsecurePath       = errors.New("insecure path")
	ErrInvalidNetworkPath = errors.New("invalid network path")
)

func Resolve(env Env) (Paths, error) {
	home := env.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
	}
	dataHome := env.DataHome
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	configHome := env.ConfigHome
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	runtimeParent := env.RuntimeOverride
	if runtimeParent == "" {
		runtimeParent = env.RuntimeDir
	}
	if runtimeParent == "" {
		return Paths{}, ErrRuntimeDirRequired
	}
	runtimeParent = filepath.Clean(runtimeParent)
	if err := assertOwnedDir(runtimeParent); err != nil {
		return Paths{}, fmt.Errorf("%w: runtime parent: %v", ErrInsecurePath, err)
	}
	p := Paths{
		Home:         home,
		Data:         filepath.Join(dataHome, "lattice"),
		Config:       filepath.Join(configHome, "lattice"),
		Runtime:      filepath.Join(runtimeParent, "lattice"),
		Database:     filepath.Join(dataHome, "lattice", "database", "lattice.db"),
		NetworksRoot: filepath.Join(dataHome, "lattice", "networks"),
		Socket:       filepath.Join(runtimeParent, "lattice", "latticed.sock"),
		Lock:         filepath.Join(runtimeParent, "lattice", "latticed.lock"),
	}
	return p, nil
}

func ResolveFromOS(runtimeOverride string) (Paths, error) {
	return Resolve(Env{
		DataHome:        os.Getenv("XDG_DATA_HOME"),
		ConfigHome:      os.Getenv("XDG_CONFIG_HOME"),
		RuntimeDir:      os.Getenv("XDG_RUNTIME_DIR"),
		RuntimeOverride: runtimeOverride,
	})
}

func (p Paths) NetworksRootPath() string { return p.NetworksRoot }

func (p Paths) NetworkDir(id domain.NetworkID) (string, error) {
	parsed, err := domain.ParseNetworkID(string(id))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidNetworkPath, err)
	}
	dir := filepath.Join(p.NetworksRoot, string(parsed))
	clean := filepath.Clean(dir)
	root := filepath.Clean(p.NetworksRoot) + string(os.PathSeparator)
	if clean != filepath.Clean(p.NetworksRoot) && !hasPrefixDir(clean, root) {
		return "", ErrInvalidNetworkPath
	}
	if filepath.Base(clean) != string(parsed) {
		return "", ErrInvalidNetworkPath
	}
	return clean, nil
}

func (p Paths) TsnetDir(id domain.NetworkID) (string, error) {
	dir, err := p.NetworkDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tsnet"), nil
}

func (p Paths) EnsureLatticeDirs() error {
	for _, dir := range []string{p.Data, p.Config, p.Runtime, p.NetworksRoot, filepath.Dir(p.Database)} {
		if err := mkdirPrivate(dir); err != nil {
			return err
		}
		if err := assertOwnedDir(dir); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInsecurePath, dir, err)
		}
	}
	return nil
}

func (p Paths) EnsureNetworkDirs(id domain.NetworkID) (string, error) {
	if err := p.EnsureLatticeDirs(); err != nil {
		return "", err
	}
	netDir, err := p.NetworkDir(id)
	if err != nil {
		return "", err
	}
	tsnet, err := p.TsnetDir(id)
	if err != nil {
		return "", err
	}
	if err := mkdirPrivate(netDir); err != nil {
		return "", err
	}
	if err := mkdirPrivate(tsnet); err != nil {
		return "", err
	}
	return netDir, nil
}

func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

func assertOwnedDir(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(path)
			if parent == path {
				return err
			}
			return assertOwnedDir(parent)
		}
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink not allowed at %s", path)
	}
	if !st.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("unexpected owner uid=%d", stat.Uid)
	}
	return nil
}

func hasPrefixDir(path, rootWithSep string) bool {
	return len(path) >= len(rootWithSep) && path[:len(rootWithSep)] == rootWithSep
}

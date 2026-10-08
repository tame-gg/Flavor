package app

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/config"
)

var ErrLatticeRunning = errors.New("Lattice's latticed is still running for this user; stop it with `systemctl --user disable --now latticed`, then start flavord again")

func migrateFromLattice(paths config.Paths, log *slog.Logger) error {
	oldData := filepath.Join(filepath.Dir(paths.Data), "lattice")
	if _, err := os.Lstat(oldData); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(paths.Data); err == nil {
		log.Warn("Lattice data found but not migrated because Flavor data already exists", "lattice", oldData, "flavor", paths.Data)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	unlock, err := lock(filepath.Join(filepath.Dir(paths.Runtime), "lattice", "latticed.lock"))
	switch {
	case err == nil:
		defer unlock()
	case errors.Is(err, ErrAlreadyRunning):
		return ErrLatticeRunning
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	dbDir := filepath.Join(oldData, filepath.Base(filepath.Dir(paths.Database)))
	entries, err := os.ReadDir(dbDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if rest, ok := strings.CutPrefix(e.Name(), "lattice.db"); ok {
			if err := os.Rename(filepath.Join(dbDir, e.Name()), filepath.Join(dbDir, filepath.Base(paths.Database)+rest)); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(oldData, paths.Data); err != nil {
		return err
	}

	oldConfig := filepath.Join(filepath.Dir(paths.Config), "lattice")
	if _, err := os.Lstat(paths.Config); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(oldConfig, paths.Config); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	log.Info("migrated Lattice data", "from", oldData, "to", paths.Data)
	return nil
}

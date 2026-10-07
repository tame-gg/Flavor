package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type DirResolver interface {
	NetworkDir(id domain.NetworkID) (string, error)
	NetworksRoot() string
}

type PathResolver struct {
	Root string
}

func (p PathResolver) NetworksRoot() string { return p.Root }

func (p PathResolver) NetworkDir(id domain.NetworkID) (string, error) {
	parsed, err := domain.ParseNetworkID(string(id))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	dir := filepath.Join(p.Root, string(parsed))
	clean := filepath.Clean(dir)
	root := filepath.Clean(p.Root) + string(os.PathSeparator)
	if clean != filepath.Clean(p.Root) && !(len(clean) >= len(root) && clean[:len(root)] == root) {
		return "", ErrInvalidInput
	}
	if filepath.Base(clean) != string(parsed) {
		return "", ErrInvalidInput
	}
	return clean, nil
}

func (db *DB) SoftRemove(ctx context.Context, id domain.NetworkID) error {
	n, err := db.Networks().Get(ctx, id)
	if err != nil {
		return err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO retained_identities (network_id, provider, control_url, display_name_hint, node_hostname, removed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		string(n.ID), string(n.Provider), n.ControlURL, n.DisplayName, n.NodeHostname,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM networks WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

const tombstonePrefix = ".deleting-"

func (db *DB) HardDeleteIdentity(ctx context.Context, id domain.NetworkID, dirs DirResolver) error {
	if dirs == nil {
		return fmt.Errorf("%w: directory resolver required", ErrInvalidInput)
	}
	netDir, err := dirs.NetworkDir(id)
	if err != nil {
		return err
	}
	root := filepath.Clean(dirs.NetworksRoot()) + string(os.PathSeparator)
	clean := filepath.Clean(netDir)
	if clean != filepath.Clean(dirs.NetworksRoot()) && !(len(clean) >= len(root) && clean[:len(root)] == root) {
		return fmt.Errorf("%w: path escapes networks root", ErrInvalidInput)
	}

	tomb := ""
	switch err := validateNetworkDirForDelete(netDir, dirs.NetworksRoot()); {
	case err == nil:
		tomb = filepath.Join(filepath.Dir(netDir), tombstonePrefix+string(id))
		if err := os.Rename(netDir, tomb); err != nil {
			return err
		}
	case !os.IsNotExist(err):
		return err
	}
	restore := func() {
		if tomb != "" {
			_ = os.Rename(tomb, netDir)
		}
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		restore()
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM networks WHERE id = ?`, string(id)); err != nil {
		restore()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM retained_identities WHERE network_id = ?`, string(id)); err != nil {
		restore()
		return err
	}
	if err := tx.Commit(); err != nil {
		restore()
		return err
	}

	if tomb != "" {
		if err := os.RemoveAll(tomb); err != nil {
			return fmt.Errorf("%w: %v", ErrIdentityDeletePending, err)
		}
	}
	return nil
}

func SweepDeletedIdentities(networksRoot string) error {
	entries, err := os.ReadDir(networksRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), tombstonePrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(networksRoot, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func validateNetworkDirForDelete(netDir, networksRoot string) error {
	st, err := os.Lstat(netDir)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: refusing to delete symlink identity dir", ErrInvalidInput)
	}
	if !st.IsDir() {
		return fmt.Errorf("%w: identity path is not a directory", ErrInvalidInput)
	}
	parent := filepath.Clean(filepath.Dir(netDir))
	if parent != filepath.Clean(networksRoot) {
		return fmt.Errorf("%w: unexpected parent directory", ErrInvalidInput)
	}
	return nil
}

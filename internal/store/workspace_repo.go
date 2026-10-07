package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type WorkspaceRepository struct {
	db *DB
}

func (db *DB) Workspaces() *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

func validateWorkspace(w domain.Workspace) error {
	if _, err := domain.ParseWorkspaceID(string(w.ID)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := domain.ValidateWorkspaceName(w.Name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := domain.ValidateWorkspaceDescription(w.Description); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return nil
}

func (r *WorkspaceRepository) Create(ctx context.Context, w domain.Workspace) error {
	if err := validateWorkspace(w); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return r.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			string(w.ID), w.Name, w.Description, now, now); err != nil {
			return err
		}
		return insertMembers(ctx, tx, w)
	})
}

func (r *WorkspaceRepository) Update(ctx context.Context, w domain.Workspace) error {
	if err := validateWorkspace(w); err != nil {
		return err
	}
	return r.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE workspaces SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
			w.Name, w.Description, time.Now().UTC().Format(time.RFC3339Nano), string(w.ID))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_networks WHERE workspace_id = ?`, string(w.ID)); err != nil {
			return err
		}
		return insertMembers(ctx, tx, w)
	})
}

func insertMembers(ctx context.Context, tx *sql.Tx, w domain.Workspace) error {
	for _, id := range w.NetworkIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_networks (workspace_id, network_id) VALUES (?, ?)`,
			string(w.ID), string(id)); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkspaceRepository) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		if isConstraint(err) {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		return err
	}
	return tx.Commit()
}

func (r *WorkspaceRepository) Delete(ctx context.Context, id domain.WorkspaceID) error {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WorkspaceRepository) Get(ctx context.Context, id domain.WorkspaceID) (domain.Workspace, error) {
	all, err := r.list(ctx, `WHERE id = ?`, string(id))
	if err != nil {
		return domain.Workspace{}, err
	}
	if len(all) == 0 {
		return domain.Workspace{}, ErrNotFound
	}
	return all[0], nil
}

func (r *WorkspaceRepository) List(ctx context.Context) ([]domain.Workspace, error) {
	return r.list(ctx, "")
}

func (r *WorkspaceRepository) list(ctx context.Context, where string, args ...any) ([]domain.Workspace, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT id, name, description, created_at, updated_at FROM workspaces `+where+` ORDER BY name, id`, args...)
	if err != nil {
		return nil, err
	}
	var out []domain.Workspace
	index := map[domain.WorkspaceID]int{}
	for rows.Next() {
		var id, name, desc, created, updated string
		if err := rows.Scan(&id, &name, &desc, &created, &updated); err != nil {
			_ = rows.Close()
			return nil, err
		}
		w := domain.Workspace{ID: domain.WorkspaceID(id), Name: name, Description: desc}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		index[w.ID] = len(out)
		out = append(out, w)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	members, err := r.db.sql.QueryContext(ctx, `SELECT workspace_id, network_id FROM workspace_networks ORDER BY workspace_id, network_id`)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var ws, net string
		if err := members.Scan(&ws, &net); err != nil {
			return nil, err
		}
		if i, ok := index[domain.WorkspaceID(ws)]; ok {
			out[i].NetworkIDs = append(out[i].NetworkIDs, domain.NetworkID(net))
		}
	}
	return out, members.Err()
}

func (r *WorkspaceRepository) Containing(ctx context.Context, network domain.NetworkID) ([]domain.WorkspaceID, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT workspace_id FROM workspace_networks WHERE network_id = ? ORDER BY workspace_id`, string(network))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WorkspaceID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, domain.WorkspaceID(id))
	}
	return out, rows.Err()
}

func (r *WorkspaceRepository) SetActive(ctx context.Context, id domain.WorkspaceID) error {
	if id == "" {
		_, err := r.db.sql.ExecContext(ctx, `DELETE FROM active_workspace`)
		return err
	}
	_, err := r.db.sql.ExecContext(ctx, `INSERT INTO active_workspace (singleton, workspace_id) VALUES (1, ?)
ON CONFLICT(singleton) DO UPDATE SET workspace_id = excluded.workspace_id`, string(id))
	if err != nil && isConstraint(err) {
		return ErrNotFound
	}
	return err
}

func (r *WorkspaceRepository) Active(ctx context.Context) (domain.WorkspaceID, error) {
	var id string
	err := r.db.sql.QueryRowContext(ctx, `SELECT workspace_id FROM active_workspace WHERE singleton = 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return domain.WorkspaceID(id), err
}

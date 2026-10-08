package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type RetainedIdentity struct {
	NetworkID       domain.NetworkID
	Provider        domain.ProviderType
	ControlURL      string
	DisplayNameHint string
	NodeHostname    string
	RemovedAt       time.Time
}

type RetainedRepository struct {
	db *DB
}

func (db *DB) Retained() *RetainedRepository {
	return &RetainedRepository{db: db}
}

func (r *RetainedRepository) Create(ctx context.Context, ri RetainedIdentity) error {
	if _, err := domain.ParseNetworkID(string(ri.NetworkID)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if _, err := domain.ParseProvider(string(ri.Provider)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	control, err := domain.NormalizeControlURL(ri.Provider, ri.ControlURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := domain.ValidateNodeHostname(ri.NodeHostname); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if ri.DisplayNameHint == "" {
		return fmt.Errorf("%w: display name hint required", ErrInvalidInput)
	}
	if ri.RemovedAt.IsZero() {
		ri.RemovedAt = time.Now().UTC()
	}
	_, err = r.db.sql.ExecContext(ctx, `
INSERT INTO retained_identities (network_id, provider, control_url, display_name_hint, node_hostname, removed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		string(ri.NetworkID), string(ri.Provider), control, ri.DisplayNameHint, ri.NodeHostname,
		ri.RemovedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isConstraint(err) {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		return err
	}
	return nil
}

func (r *RetainedRepository) Get(ctx context.Context, id domain.NetworkID) (RetainedIdentity, error) {
	row := r.db.sql.QueryRowContext(ctx, `
SELECT network_id, provider, control_url, display_name_hint, node_hostname, removed_at
FROM retained_identities WHERE network_id = ?`, string(id))
	ri, err := scanRetained(row)
	if err == sql.ErrNoRows {
		return RetainedIdentity{}, ErrNotFound
	}
	return ri, err
}

func (r *RetainedRepository) List(ctx context.Context) ([]RetainedIdentity, error) {
	rows, err := r.db.sql.QueryContext(ctx, `
SELECT network_id, provider, control_url, display_name_hint, node_hostname, removed_at
FROM retained_identities ORDER BY removed_at ASC, network_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RetainedIdentity
	for rows.Next() {
		ri, err := scanRetained(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ri)
	}
	return out, rows.Err()
}

func (r *RetainedRepository) Delete(ctx context.Context, id domain.NetworkID) error {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM retained_identities WHERE network_id = ?`, string(id))
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
	return nil
}

func scanRetained(row rowScanner) (RetainedIdentity, error) {
	var id, provider, control, hint, hostname, removed string
	if err := row.Scan(&id, &provider, &control, &hint, &hostname, &removed); err != nil {
		return RetainedIdentity{}, err
	}
	nid, err := domain.ParseNetworkID(id)
	if err != nil {
		return RetainedIdentity{}, fmt.Errorf("%w: %v", ErrCorruptDB, err)
	}
	p, err := domain.ParseProvider(provider)
	if err != nil {
		return RetainedIdentity{}, fmt.Errorf("%w: %v", ErrCorruptDB, err)
	}
	removedAt, err := time.Parse(time.RFC3339Nano, removed)
	if err != nil {
		removedAt, err = time.Parse(time.RFC3339, removed)
		if err != nil {
			return RetainedIdentity{}, fmt.Errorf("%w: removed_at", ErrCorruptDB)
		}
	}
	return RetainedIdentity{
		NetworkID:       nid,
		Provider:        p,
		ControlURL:      control,
		DisplayNameHint: hint,
		NodeHostname:    hostname,
		RemovedAt:       removedAt.UTC(),
	}, nil
}

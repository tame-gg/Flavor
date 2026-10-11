package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type NetworkRepository struct {
	db *DB
}

func (db *DB) Networks() *NetworkRepository {
	return &NetworkRepository{db: db}
}

func (r *NetworkRepository) Create(ctx context.Context, n domain.Network) error {
	if err := prepareNetwork(&n); err != nil {
		return err
	}
	_, err := r.db.sql.ExecContext(ctx, `
INSERT INTO networks (id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(n.ID), n.DisplayName, string(n.Provider), n.ControlURL, boolToInt(n.AutoConnect),
		n.NodeHostname, n.CreatedAt.UTC().Format(time.RFC3339Nano), n.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isConstraint(err) {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		return err
	}
	return nil
}

func (r *NetworkRepository) Get(ctx context.Context, id domain.NetworkID) (domain.Network, error) {
	row := r.db.sql.QueryRowContext(ctx, `
SELECT id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at, exit_node_id, advertised_routes
FROM networks WHERE id = ?`, string(id))
	n, err := scanNetwork(row)
	if err == sql.ErrNoRows {
		return domain.Network{}, ErrNotFound
	}
	return n, err
}

func (r *NetworkRepository) List(ctx context.Context) ([]domain.Network, error) {
	rows, err := r.db.sql.QueryContext(ctx, `
SELECT id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at, exit_node_id, advertised_routes
FROM networks ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Network
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *NetworkRepository) Update(ctx context.Context, n domain.Network) error {
	if err := prepareNetwork(&n); err != nil {
		return err
	}
	n.UpdatedAt = time.Now().UTC()
	res, err := r.db.sql.ExecContext(ctx, `
UPDATE networks
SET display_name = ?, provider = ?, control_url = ?, auto_connect = ?, node_hostname = ?, updated_at = ?
WHERE id = ?`,
		n.DisplayName, string(n.Provider), n.ControlURL, boolToInt(n.AutoConnect),
		n.NodeHostname, n.UpdatedAt.Format(time.RFC3339Nano), string(n.ID),
	)
	if err != nil {
		if isConstraint(err) {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
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

func (r *NetworkRepository) SetExitNode(ctx context.Context, id domain.NetworkID, node domain.NodeID) error {
	return r.setColumn(ctx, id, "exit_node_id", string(node))
}

func (r *NetworkRepository) SetAdvertisedRoutes(ctx context.Context, id domain.NetworkID, routes []netip.Prefix) error {
	parts := make([]string, len(routes))
	for i, p := range routes {
		parts[i] = p.String()
	}
	return r.setColumn(ctx, id, "advertised_routes", strings.Join(parts, " "))
}

func (r *NetworkRepository) setColumn(ctx context.Context, id domain.NetworkID, column, value string) error {
	res, err := r.db.sql.ExecContext(ctx, `UPDATE networks SET `+column+` = ? WHERE id = ?`, value, string(id))
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

func parseRoutes(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Fields(raw) {
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *NetworkRepository) Delete(ctx context.Context, id domain.NetworkID) error {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM networks WHERE id = ?`, string(id))
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanNetwork(row rowScanner) (domain.Network, error) {
	var (
		id, display, provider, control, hostname, created, updated, exit, routes string
		auto                                                                     int
	)
	if err := row.Scan(&id, &display, &provider, &control, &auto, &hostname, &created, &updated, &exit, &routes); err != nil {
		return domain.Network{}, err
	}
	nid, err := domain.ParseNetworkID(id)
	if err != nil {
		return domain.Network{}, fmt.Errorf("%w: id %q: %v", ErrCorruptDB, id, err)
	}
	p, err := domain.ParseProvider(provider)
	if err != nil {
		return domain.Network{}, fmt.Errorf("%w: provider: %v", ErrCorruptDB, err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return domain.Network{}, fmt.Errorf("%w: created_at", ErrCorruptDB)
		}
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		updatedAt, err = time.Parse(time.RFC3339, updated)
		if err != nil {
			return domain.Network{}, fmt.Errorf("%w: updated_at", ErrCorruptDB)
		}
	}
	advertised, err := parseRoutes(routes)
	if err != nil {
		return domain.Network{}, fmt.Errorf("%w: advertised_routes", ErrCorruptDB)
	}
	n := domain.Network{
		ID:               nid,
		DisplayName:      display,
		Provider:         p,
		ControlURL:       control,
		AutoConnect:      auto != 0,
		NodeHostname:     hostname,
		CreatedAt:        createdAt.UTC(),
		UpdatedAt:        updatedAt.UTC(),
		ExitNode:         domain.NodeID(exit),
		AdvertisedRoutes: advertised,
	}
	if err := n.Validate(); err != nil {
		return domain.Network{}, fmt.Errorf("%w: %v", ErrCorruptDB, err)
	}
	return n, nil
}

func prepareNetwork(n *domain.Network) error {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = n.CreatedAt
	}
	control, err := domain.NormalizeControlURL(n.Provider, n.ControlURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	n.ControlURL = control
	if err := n.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

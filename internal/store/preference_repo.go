package store

import (
	"context"
	"fmt"
	"time"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
)

type PreferenceRepository struct {
	db *DB
}

func (db *DB) Preferences() *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

func (r *PreferenceRepository) Set(ctx context.Context, p domain.DestinationPreference) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.sql.ExecContext(ctx, `
INSERT INTO destination_preferences (destination, kind, network_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(destination) DO UPDATE SET kind = excluded.kind, network_id = excluded.network_id, updated_at = excluded.updated_at`,
		p.Destination, string(p.Kind), string(p.NetworkID), now, now)
	if err != nil && isConstraint(err) {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return err
}

func (r *PreferenceRepository) Delete(ctx context.Context, destination string) (bool, error) {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM destination_preferences WHERE destination = ?`, destination)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *PreferenceRepository) Get(ctx context.Context, destination string) (domain.DestinationPreference, error) {
	all, err := r.query(ctx, `WHERE destination = ?`, destination)
	if err != nil {
		return domain.DestinationPreference{}, err
	}
	if len(all) == 0 {
		return domain.DestinationPreference{}, ErrNotFound
	}
	return all[0], nil
}

func (r *PreferenceRepository) List(ctx context.Context) ([]domain.DestinationPreference, error) {
	return r.query(ctx, "")
}

func (r *PreferenceRepository) ForNetwork(ctx context.Context, id domain.NetworkID) ([]domain.DestinationPreference, error) {
	return r.query(ctx, `WHERE network_id = ?`, string(id))
}

func (r *PreferenceRepository) query(ctx context.Context, where string, args ...any) ([]domain.DestinationPreference, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT destination, kind, network_id, created_at, updated_at FROM destination_preferences `+where+` ORDER BY destination`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DestinationPreference
	for rows.Next() {
		var dest, kind, net, created, updated string
		if err := rows.Scan(&dest, &kind, &net, &created, &updated); err != nil {
			return nil, err
		}
		p := domain.DestinationPreference{Destination: dest, Kind: domain.DestinationKind(kind), NetworkID: domain.NetworkID(net)}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/synthetic/layout"
)

const (
	V4Quarantine    = 24 * time.Hour
	IndexQuarantine = 7 * 24 * time.Hour
)

var (
	ErrPoolExhausted    = errors.New("synthetic address pool exhausted")
	ErrIndicesExhausted = errors.New("no synthetic network index available")
)

type SyntheticRepository struct {
	db *DB
}

func (db *DB) Synthetic() *SyntheticRepository {
	return &SyntheticRepository{db: db}
}

type SyntheticMapping struct {
	Synthetic netip.Addr
	NetworkID domain.NetworkID
	Real      netip.Addr
	LastUsed  time.Time
}

func (r *SyntheticRepository) tx(ctx context.Context, fn func(*sql.Tx) error) error {
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

func (r *SyntheticRepository) EnsureULA(ctx context.Context, generate func() (netip.Prefix, error)) (netip.Prefix, error) {
	var out netip.Prefix
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT ula_prefix FROM synthetic_install WHERE singleton = 1`).Scan(&raw)
		if err == nil {
			out, err = netip.ParsePrefix(raw)
			if err != nil {
				return fmt.Errorf("%w: ula prefix", ErrCorruptDB)
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if out, err = generate(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO synthetic_install (singleton, ula_prefix) VALUES (1, ?)`, out.String())
		return err
	})
	return out, err
}

func (r *SyntheticRepository) V4Pool(ctx context.Context) (netip.Prefix, error) {
	var raw string
	err := r.db.sql.QueryRowContext(ctx, `SELECT ipv4_pool FROM synthetic_install WHERE singleton = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || raw == "" {
		return netip.Prefix{}, nil
	}
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.ParsePrefix(raw)
}

func (r *SyntheticRepository) SetV4Pool(ctx context.Context, p netip.Prefix) error {
	res, err := r.db.sql.ExecContext(ctx, `UPDATE synthetic_install SET ipv4_pool = ? WHERE singleton = 1`, p.String())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SyntheticRepository) NetworkIndex(ctx context.Context, id domain.NetworkID, now time.Time) (uint16, error) {
	var idx uint16
	err := r.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT idx FROM synthetic_network_index WHERE network_id = ?`, string(id)).Scan(&idx)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		blocked, err := intSet(ctx, tx, `SELECT idx FROM synthetic_network_index UNION SELECT idx FROM synthetic_index_quarantine WHERE reusable_after > ?`, now.Unix())
		if err != nil {
			return err
		}
		for i := 1; i <= layout.MaxNetworkIndex; i++ {
			if !blocked[int64(i)] {
				idx = uint16(i)
				break
			}
		}
		if idx == 0 {
			return ErrIndicesExhausted
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM synthetic_index_quarantine WHERE idx = ?`, idx); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO synthetic_network_index (network_id, idx) VALUES (?, ?)`, string(id), idx)
		return err
	})
	return idx, err
}

func (r *SyntheticRepository) NetworkForIndex(ctx context.Context, idx uint16) (domain.NetworkID, error) {
	var id string
	err := r.db.sql.QueryRowContext(ctx, `SELECT network_id FROM synthetic_network_index WHERE idx = ?`, idx).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return domain.NetworkID(id), err
}

func (r *SyntheticRepository) MapV6(ctx context.Context, id domain.NetworkID, real netip.Addr, encode func(counter uint64) (netip.Addr, error)) (netip.Addr, error) {
	var out netip.Addr
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT synthetic FROM synthetic_v6 WHERE network_id = ? AND real = ?`, string(id), real.String()).Scan(&raw)
		if err == nil {
			out, err = netip.ParseAddr(raw)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var counter uint64
		if err := tx.QueryRowContext(ctx, `SELECT next_v6 FROM synthetic_network_index WHERE network_id = ?`, string(id)).Scan(&counter); err != nil {
			return err
		}
		if out, err = encode(counter); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE synthetic_network_index SET next_v6 = next_v6 + 1 WHERE network_id = ?`, string(id)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO synthetic_v6 (synthetic, network_id, real) VALUES (?, ?, ?)`, out.String(), string(id), real.String())
		return err
	})
	return out, err
}

func (r *SyntheticRepository) MapV4(ctx context.Context, id domain.NetworkID, real netip.Addr, pool netip.Prefix, first netip.Addr, now time.Time) (netip.Addr, error) {
	var out netip.Addr
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT synthetic FROM synthetic_v4 WHERE network_id = ? AND real = ?`, string(id), real.String()).Scan(&raw)
		if err == nil {
			if out, err = netip.ParseAddr(raw); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE synthetic_v4 SET last_used = ? WHERE synthetic = ?`, now.Unix(), raw)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		blocked, err := stringSet(ctx, tx, `SELECT synthetic FROM synthetic_v4 UNION SELECT synthetic FROM synthetic_v4_quarantine WHERE reusable_after > ?`, now.Unix())
		if err != nil {
			return err
		}
		for a := first; pool.Contains(a) && pool.Contains(a.Next()); a = a.Next() {
			if !blocked[a.String()] {
				out = a
				break
			}
		}
		if !out.IsValid() {
			return ErrPoolExhausted
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM synthetic_v4_quarantine WHERE synthetic = ?`, out.String()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO synthetic_v4 (synthetic, network_id, real, last_used) VALUES (?, ?, ?, ?)`,
			out.String(), string(id), real.String(), now.Unix())
		return err
	})
	return out, err
}

func (r *SyntheticRepository) ReleaseV4(ctx context.Context, synthetic netip.Addr, now time.Time) error {
	return r.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM synthetic_v4 WHERE synthetic = ?`, synthetic.String())
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO synthetic_v4_quarantine (synthetic, reusable_after) VALUES (?, ?)`,
			synthetic.String(), now.Add(V4Quarantine).Unix())
		return err
	})
}

func (r *SyntheticRepository) V4ByLastUsed(ctx context.Context, before time.Time) ([]SyntheticMapping, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT synthetic, network_id, real, last_used FROM synthetic_v4 WHERE last_used < ? ORDER BY last_used, synthetic`, before.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyntheticMapping
	for rows.Next() {
		var syn, net, real string
		var last int64
		if err := rows.Scan(&syn, &net, &real, &last); err != nil {
			return nil, err
		}
		m := SyntheticMapping{NetworkID: domain.NetworkID(net), LastUsed: time.Unix(last, 0)}
		m.Synthetic, _ = netip.ParseAddr(syn)
		m.Real, _ = netip.ParseAddr(real)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SyntheticRepository) Lookup(ctx context.Context, synthetic netip.Addr) (SyntheticMapping, error) {
	m := SyntheticMapping{Synthetic: synthetic}
	var net, real string
	err := r.db.sql.QueryRowContext(ctx, `SELECT network_id, real FROM synthetic_v4 WHERE synthetic = ? UNION ALL SELECT network_id, real FROM synthetic_v6 WHERE synthetic = ?`,
		synthetic.String(), synthetic.String()).Scan(&net, &real)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	m.NetworkID = domain.NetworkID(net)
	m.Real, err = netip.ParseAddr(real)
	return m, err
}

func (r *SyntheticRepository) Quarantined(ctx context.Context, synthetic netip.Addr) (time.Time, bool, error) {
	var after int64
	err := r.db.sql.QueryRowContext(ctx, `SELECT reusable_after FROM synthetic_v4_quarantine WHERE synthetic = ?`, synthetic.String()).Scan(&after)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return time.Unix(after, 0), err == nil, err
}

func intSet(ctx context.Context, tx *sql.Tx, q string, args ...any) (map[int64]bool, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func stringSet(ctx context.Context, tx *sql.Tx, q string, args ...any) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

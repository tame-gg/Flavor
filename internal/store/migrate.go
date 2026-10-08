package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func (db *DB) migrate(ctx context.Context) error {
	sub, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		return err
	}
	return db.migrateFrom(ctx, sub)
}

func (db *DB) migrateFrom(ctx context.Context, migrations fs.FS) error {
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return err
	}
	var names []string
	known := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		ver, err := migrationVersion(e.Name())
		if err != nil {
			return err
		}
		names = append(names, e.Name())
		known = max(known, ver)
	}
	sort.Strings(names)

	if _, err := db.sql.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`); err != nil {
		return err
	}
	applied, err := db.appliedMigrations(ctx)
	if err != nil {
		return err
	}
	current := 0
	for v := range applied {
		current = max(current, v)
	}
	if current > known {
		return fmt.Errorf("%w: schema %d, this build knows up to %d", ErrNewerSchema, current, known)
	}

	var pending []string
	for _, name := range names {
		if ver, _ := migrationVersion(name); !applied[ver] {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if len(applied) > 0 {
		if err := db.backup(ctx, fmt.Sprintf("%s.schema-%d.bak", db.path, current)); err != nil {
			return fmt.Errorf("backup before migrating: %w", err)
		}
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, name := range pending {
		ver, _ := migrationVersion(name)
		body, err := fs.ReadFile(migrations, name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			ver, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) appliedMigrations(ctx context.Context) (map[int]bool, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (db *DB) backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := db.sql.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func migrationVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 1 {
		return 0, fmt.Errorf("bad migration name %q", name)
	}
	return strconv.Atoi(parts[0])
}

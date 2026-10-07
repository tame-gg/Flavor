package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql  *sql.DB
	path string
}

var (
	ErrNotFound     = errors.New("not found")
	ErrCorruptDB    = errors.New("database corrupt or unreadable")
	ErrInvalidInput = errors.New("invalid input")
)

func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty database path", ErrInvalidInput)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if st, err := os.Stat(path); err == nil {
		if st.IsDir() {
			return nil, fmt.Errorf("%w: path is directory", ErrCorruptDB)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_time_format=sqlite"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		if fileLooksPresent(path) {
			return nil, fmt.Errorf("%w: %v", ErrCorruptDB, err)
		}
		return nil, err
	}
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	db := &DB{sql: sqlDB, path: path}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		if fileLooksPresent(path) {
			return nil, fmt.Errorf("%w: migrate: %v", ErrCorruptDB, err)
		}
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return db, nil
}

func fileLooksPresent(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}

func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func (db *DB) Path() string { return db.path }

func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func isConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint") || strings.Contains(msg, "check")
}

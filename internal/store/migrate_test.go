package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestNewerSchemaIsRefusedAndLeftUntouched(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flavor.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (999, 'future')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, _ := os.ReadFile(path)
	if _, err := Open(ctx, path); !errors.Is(err, ErrNewerSchema) || errors.Is(err, ErrCorruptDB) {
		t.Fatalf("a newer schema must be refused, not treated as corrupt: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("refusing a newer schema must not modify the database")
	}
}

func TestUpgradeBacksUpTheOldSchemaFirst(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	v1 := fstest.MapFS{"001_init.sql": {Data: []byte(`CREATE TABLE notes (body TEXT); INSERT INTO notes VALUES ('kept');`)}}
	v2 := fstest.MapFS{
		"001_init.sql": v1["001_init.sql"],
		"002_more.sql": {Data: []byte(`ALTER TABLE notes ADD COLUMN extra TEXT;`)},
	}
	open := func(m fstest.MapFS) *DB {
		t.Helper()
		sqlDB, err := sql.Open("sqlite", "file:"+path)
		if err != nil {
			t.Fatal(err)
		}
		db := &DB{sql: sqlDB, path: path}
		if err := db.migrateFrom(ctx, m); err != nil {
			t.Fatal(err)
		}
		return db
	}
	first := open(v1)
	first.Close()
	if _, err := os.Stat(path + ".schema-0.bak"); !os.IsNotExist(err) {
		t.Fatal("a fresh database has nothing to back up")
	}
	second := open(v2)
	defer second.Close()
	backup := path + ".schema-1.bak"
	st, err := os.Stat(backup)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("backup missing or not private: %v %v", st, err)
	}
	old, err := sql.Open("sqlite", "file:"+backup)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var body string
	var cols int
	if err := old.QueryRowContext(ctx, `SELECT body FROM notes`).Scan(&body); err != nil || body != "kept" {
		t.Fatalf("backup lost data: %q %v", body, err)
	}
	if err := old.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('notes')`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("backup must hold the pre-upgrade schema: %d columns %v", cols, err)
	}
}

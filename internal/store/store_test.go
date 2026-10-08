package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/store"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flavor.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sampleNetwork(provider domain.ProviderType, name, host, control string) domain.Network {
	now := time.Now().UTC()
	return domain.Network{
		ID:           domain.NewNetworkID(),
		DisplayName:  name,
		Provider:     provider,
		ControlURL:   control,
		AutoConnect:  true,
		NodeHostname: host,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestMigrateFreshDB(t *testing.T) {
	db := openTestDB(t)
	ver, err := db.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ver != 4 {
		t.Fatalf("version=%d", ver)
	}
}

func TestCreateGetListUpdateNetwork(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	n := sampleNetwork(domain.ProviderHeadscale, "LunarLabs", "luna-desktop", "https://headscale.example.com/")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := db.Networks().Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ControlURL != "https://headscale.example.com" {
		t.Fatalf("control url not normalized: %q", got.ControlURL)
	}
	if !got.AutoConnect || got.NodeHostname != "luna-desktop" {
		t.Fatalf("got %+v", got)
	}
	list, err := db.Networks().List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	got.DisplayName = "Work"
	if err := db.Networks().Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, err := db.Networks().Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != n.ID {
		t.Fatal("rename changed id")
	}
	if again.NodeHostname != "luna-desktop" {
		t.Fatal("rename changed hostname")
	}
	if again.DisplayName != "Work" {
		t.Fatalf("display=%q", again.DisplayName)
	}
}

func TestDuplicateDisplayNamesAllowed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	a := sampleNetwork(domain.ProviderTailscale, "Home", "host-a", "")
	b := sampleNetwork(domain.ProviderHeadscale, "Home", "host-b", "https://hs.example")
	if err := db.Networks().Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.Networks().Create(ctx, b); err != nil {
		t.Fatal(err)
	}
}

func TestProviderAndAutoConnectConstraints(t *testing.T) {
	db := openTestDB(t)
	raw, err := sql.Open("sqlite", "file:"+db.Path()+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_, err = raw.Exec(`INSERT INTO networks (id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at)
VALUES ('01TESTID000000000000000000', 'X', 'tailscale', '', 2, 'host-x', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected auto_connect check failure")
	}
	_, err = raw.Exec(`INSERT INTO networks (id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at)
VALUES ('01TESTID000000000000000001', 'X', 'wireguard', '', 0, 'host-x', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected provider check failure")
	}
	_, err = raw.Exec(`INSERT INTO networks (id, display_name, provider, control_url, auto_connect, node_hostname, created_at, updated_at)
VALUES ('01TESTID000000000000000002', 'X', 'tailscale', 'https://nope.example', 0, 'host-x', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected tailscale empty control_url check failure")
	}
}

func TestTailscaleEmptyControlURL(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	n := sampleNetwork(domain.ProviderTailscale, "Personal", "luna-desktop", "https://ignored.example")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := db.Networks().Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ControlURL != "" {
		t.Fatalf("want empty control url, got %q", got.ControlURL)
	}
}

func TestReopenPreservesRegistry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flavor.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	n := sampleNetwork(domain.ProviderHeadscale, "Lab", "lab-host", "https://hs.lab")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	got, err := db2.Networks().Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Lab" {
		t.Fatalf("got %+v", got)
	}
}

func TestNoRuntimePeerOrSecretColumns(t *testing.T) {
	db := openTestDB(t)
	raw, err := sql.Open("sqlite", "file:"+db.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	rows, err := raw.Query(`PRAGMA table_info(networks)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, name)
	}
	forbidden := map[string]bool{
		"state": true, "connection_state": true, "auth_key": true, "secret": true,
		"peer": true, "peers": true, "auth_url": true,
	}
	for _, c := range cols {
		if forbidden[c] {
			t.Fatalf("forbidden column %q", c)
		}
	}
	var peerTables int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '%peer%'`).Scan(&peerTables); err != nil {
		t.Fatal(err)
	}
	if peerTables != 0 {
		t.Fatal("peer tables must not exist")
	}
}

func TestSoftRemoveRetainsIdentity(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	root := filepath.Join(t.TempDir(), "networks")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	n := sampleNetwork(domain.ProviderHeadscale, "LunarLabs", "luna-desktop", "https://hs.example")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	dirs := store.PathResolver{Root: root}
	netDir, err := dirs.NetworkDir(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	tsnet := filepath.Join(netDir, "tsnet")
	if err := os.MkdirAll(tsnet, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(tsnet, "marker")
	if err := os.WriteFile(marker, []byte("identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.SoftRemove(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Networks().Get(ctx, n.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("configured network should be gone: %v", err)
	}
	list, err := db.Networks().List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	ri, err := db.Retained().Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ri.Provider != domain.ProviderHeadscale || ri.ControlURL != "https://hs.example" {
		t.Fatalf("retained=%+v", ri)
	}
	if ri.DisplayNameHint != "LunarLabs" || ri.NodeHostname != "luna-desktop" {
		t.Fatalf("retained=%+v", ri)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("tsnet directory must remain after soft remove")
	}
}

func TestHardDeleteRemovesFSLast(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	root := filepath.Join(t.TempDir(), "networks")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	n := sampleNetwork(domain.ProviderTailscale, "Personal", "host-p", "")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	dirs := store.PathResolver{Root: root}
	netDir, err := dirs.NetworkDir(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(netDir, "tsnet"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := db.SoftRemove(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.HardDeleteIdentity(ctx, n.ID, dirs); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Retained().Get(ctx, n.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("retained should be gone: %v", err)
	}
	if _, err := os.Stat(netDir); !os.IsNotExist(err) {
		t.Fatal("network dir should be deleted")
	}
}

func TestHardDeleteRejectsSymlinkDir(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	root := filepath.Join(t.TempDir(), "networks")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	id := domain.NewNetworkID()
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, string(id))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	n := sampleNetwork(domain.ProviderTailscale, "X", "host-x", "")
	n.ID = id
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	err := db.HardDeleteIdentity(ctx, id, store.PathResolver{Root: root})
	if err == nil {
		t.Fatal("expected symlink reject")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("outside target must remain")
	}
}

func TestCorruptDBNotSilentlyRecreated(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flavor.db")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := store.Open(ctx, path)
	if err == nil {
		t.Fatal("expected corrupt error")
	}
	if !errors.Is(err, store.ErrCorruptDB) {
		t.Fatalf("want ErrCorruptDB, got %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "not a sqlite database" {
		t.Fatal("corrupt file must be preserved")
	}
}

func TestPathResolverRejectsTraversal(t *testing.T) {
	dirs := store.PathResolver{Root: t.TempDir()}
	if _, err := dirs.NetworkDir(domain.NetworkID("../x")); err == nil {
		t.Fatal("expected error")
	}
}

func TestHardDeleteFSFailureLeavesSweepableTombstone(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	ctx := context.Background()
	db := openTestDB(t)
	root := filepath.Join(t.TempDir(), "networks")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	n := sampleNetwork(domain.ProviderTailscale, "Personal", "host-p", "")
	if err := db.Networks().Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	dirs := store.PathResolver{Root: root}
	netDir, _ := dirs.NetworkDir(n.ID)
	locked := filepath.Join(netDir, "tsnet")
	if err := os.MkdirAll(filepath.Join(locked, "state"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	tomb := filepath.Join(root, ".deleting-"+string(n.ID))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(tomb, "tsnet"), 0o700) })

	err := db.HardDeleteIdentity(ctx, n.ID, dirs)
	if !errors.Is(err, store.ErrIdentityDeletePending) {
		t.Fatalf("got %v want ErrIdentityDeletePending", err)
	}
	if _, err := os.Stat(netDir); !os.IsNotExist(err) {
		t.Fatal("identity dir must not remain under its network id")
	}
	if _, err := os.Stat(tomb); err != nil {
		t.Fatalf("tombstone missing: %v", err)
	}
	if _, err := db.Networks().Get(ctx, n.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("metadata should be gone")
	}

	_ = os.Chmod(filepath.Join(tomb, "tsnet"), 0o700)
	if err := store.SweepDeletedIdentities(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tomb); !os.IsNotExist(err) {
		t.Fatal("tombstone not swept")
	}
}

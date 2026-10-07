CREATE TABLE networks (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    provider TEXT NOT NULL
        CHECK (provider IN ('tailscale', 'headscale')),
    control_url TEXT NOT NULL DEFAULT '',
    auto_connect INTEGER NOT NULL DEFAULT 0
        CHECK (auto_connect IN (0, 1)),
    node_hostname TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (provider = 'tailscale' AND control_url = '')
        OR
        (provider = 'headscale' AND control_url <> '')
    )
);

CREATE TABLE retained_identities (
    network_id TEXT PRIMARY KEY,
    provider TEXT NOT NULL
        CHECK (provider IN ('tailscale', 'headscale')),
    control_url TEXT NOT NULL DEFAULT '',
    display_name_hint TEXT NOT NULL,
    node_hostname TEXT NOT NULL,
    removed_at TEXT NOT NULL,
    CHECK (
        (provider = 'tailscale' AND control_url = '')
        OR
        (provider = 'headscale' AND control_url <> '')
    )
);

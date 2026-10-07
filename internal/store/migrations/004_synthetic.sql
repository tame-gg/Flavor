CREATE TABLE synthetic_install (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    ula_prefix TEXT NOT NULL,
    ipv4_pool TEXT NOT NULL DEFAULT ''
);

CREATE TABLE synthetic_network_index (
    network_id TEXT PRIMARY KEY REFERENCES networks(id),
    idx INTEGER NOT NULL UNIQUE CHECK (idx BETWEEN 1 AND 65535),
    next_v6 INTEGER NOT NULL DEFAULT 1 CHECK (next_v6 >= 1)
);

CREATE TABLE synthetic_index_quarantine (
    idx INTEGER PRIMARY KEY,
    reusable_after INTEGER NOT NULL
);

CREATE TABLE synthetic_v6 (
    synthetic TEXT PRIMARY KEY,
    network_id TEXT NOT NULL REFERENCES networks(id),
    real TEXT NOT NULL,
    UNIQUE (network_id, real)
);

CREATE TABLE synthetic_v4 (
    synthetic TEXT PRIMARY KEY,
    network_id TEXT NOT NULL REFERENCES networks(id),
    real TEXT NOT NULL,
    last_used INTEGER NOT NULL,
    UNIQUE (network_id, real)
);

CREATE TABLE synthetic_v4_quarantine (
    synthetic TEXT PRIMARY KEY,
    reusable_after INTEGER NOT NULL
);

CREATE TRIGGER synthetic_release_network BEFORE DELETE ON networks
BEGIN
    INSERT OR REPLACE INTO synthetic_v4_quarantine (synthetic, reusable_after)
        SELECT synthetic, CAST(strftime('%s', 'now') AS INTEGER) + 86400 FROM synthetic_v4 WHERE network_id = OLD.id;
    DELETE FROM synthetic_v4 WHERE network_id = OLD.id;
    DELETE FROM synthetic_v6 WHERE network_id = OLD.id;
    INSERT OR REPLACE INTO synthetic_index_quarantine (idx, reusable_after)
        SELECT idx, CAST(strftime('%s', 'now') AS INTEGER) + 604800 FROM synthetic_network_index WHERE network_id = OLD.id;
    DELETE FROM synthetic_network_index WHERE network_id = OLD.id;
END;

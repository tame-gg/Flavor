CREATE TABLE destination_preferences (
    destination TEXT PRIMARY KEY CHECK (length(destination) BETWEEN 1 AND 253),
    kind TEXT NOT NULL CHECK (kind IN ('address', 'name')),
    network_id TEXT NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

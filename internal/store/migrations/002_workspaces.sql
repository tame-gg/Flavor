CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 256),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE workspace_networks (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    network_id TEXT NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    PRIMARY KEY (workspace_id, network_id)
);

CREATE TABLE active_workspace (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE
);

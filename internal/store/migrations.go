// Package store provides SQLite-based metadata storage and content-addressed
// file I/O for the obsidian-sync server.
package store

import (
	"database/sql"
	"fmt"
)

// migrations defines the ordered list of schema migrations.
// Each migration is applied once and tracked in the _migrations table.
var migrations = []struct {
	version int
	name    string
	up      string
}{
	{
		version: 1,
		name:    "initial_schema",
		up: `
CREATE TABLE IF NOT EXISTS files (
    vault_id    TEXT    NOT NULL,
    path        TEXT    NOT NULL,
    hash        TEXT    NOT NULL,
    size        INTEGER NOT NULL,
    modified_at TEXT    NOT NULL,
    revision    INTEGER NOT NULL,
    deleted_at  TEXT,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (vault_id, path)
);

CREATE INDEX IF NOT EXISTS idx_files_vault_hash ON files (vault_id, hash);
CREATE INDEX IF NOT EXISTS idx_files_vault_deleted ON files (vault_id, deleted_at);

CREATE TABLE IF NOT EXISTS revisions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    vault_id   TEXT NOT NULL,
    file_path  TEXT NOT NULL,
    operation  TEXT NOT NULL,
    hash       TEXT NOT NULL,
    size       INTEGER NOT NULL,
    timestamp  TEXT NOT NULL DEFAULT (datetime('now')),
    client_id  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_revisions_vault_id ON revisions (vault_id, id);

CREATE TABLE IF NOT EXISTS client_revisions (
    client_id  TEXT NOT NULL,
    vault_id   TEXT NOT NULL,
    revision   INTEGER NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (vault_id, client_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    key_hash    TEXT    PRIMARY KEY,
    vault_id    TEXT    NOT NULL,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT,
    name        TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS _migrations (
    version INTEGER PRIMARY KEY,
    name    TEXT    NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`,
	},
}

// RunMigrations applies all pending database migrations.
// It creates the _migrations table if needed and applies each migration
// that has not yet been recorded, in order.
func RunMigrations(db *sql.DB) error {
	// Ensure _migrations table exists so we can check applied state.
	// (It's also in migration #1, but we need it first to track anything.)
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS _migrations (
    version INTEGER PRIMARY KEY,
    name    TEXT    NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`); err != nil {
		return fmt.Errorf("creating _migrations table: %w", err)
	}

	for _, m := range migrations {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE version = ?`, m.version).Scan(&count)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		if _, err := db.Exec(m.up); err != nil {
			return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := db.Exec(`INSERT INTO _migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
	}

	return nil
}

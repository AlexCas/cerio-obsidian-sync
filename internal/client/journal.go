// Package client provides vault synchronization client functionality
// including local change journaling, manifest building, conflict resolution,
// and sync session orchestration.
package client

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Journal operations.
const (
	JournalOpCreate = "create"
	JournalOpUpdate = "update"
	JournalOpDelete = "delete"
)

// PendingChange represents a single recorded change in the journal.
type PendingChange struct {
	ID        int64
	Path      string
	Operation string
	Hash      string
	Size      int64
	Timestamp string
}

// Journal manages a local SQLite database of pending file changes
// that have not yet been synced to the server.
type Journal struct {
	db   *sql.DB
	path string
}

// NewJournal creates a Journal pointing to the given database file path.
// Call Open to connect and initialize.
func NewJournal(dbPath string) *Journal {
	return &Journal{path: dbPath}
}

// Open connects to the SQLite database and ensures the schema exists.
func (j *Journal) Open() error {
	var err error
	j.db, err = sql.Open("sqlite", j.path)
	if err != nil {
		return fmt.Errorf("opening journal database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := j.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("setting WAL mode: %w", err)
	}

	if err := j.createSchema(); err != nil {
		return fmt.Errorf("creating journal schema: %w", err)
	}

	return nil
}

// OpenInMemory opens an in-memory SQLite database for testing.
func (j *Journal) OpenInMemory() error {
	var err error
	j.db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		return fmt.Errorf("opening in-memory journal: %w", err)
	}

	if err := j.createSchema(); err != nil {
		return fmt.Errorf("creating journal schema: %w", err)
	}

	return nil
}

// createSchema creates the pending_changes and sync_metadata tables if they do not exist.
func (j *Journal) createSchema() error {
	_, err := j.db.Exec(`
CREATE TABLE IF NOT EXISTS pending_changes (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    path      TEXT    NOT NULL,
    operation TEXT    NOT NULL,
    hash      TEXT    NOT NULL DEFAULT '',
    size      INTEGER NOT NULL DEFAULT 0,
    timestamp TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pending_changes_path ON pending_changes (path);
CREATE TABLE IF NOT EXISTS sync_metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`)
	return err
}

// Close closes the database connection.
func (j *Journal) Close() error {
	if j.db != nil {
		return j.db.Close()
	}
	return nil
}

// DB returns the underlying database connection for advanced use.
func (j *Journal) DB() *sql.DB {
	return j.db
}

// RecordChange records a file change in the journal. If a pending change for
// the same path already exists, it is updated with the new operation, hash,
// and size. If the existing operation is "create" and the new operation is
// "delete", the entry is removed (net effect: nothing to sync).
func (j *Journal) RecordChange(path, operation, hash string, size int64) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Check for existing entry for this path.
	var existingID int64
	var existingOp string
	err := j.db.QueryRow(
		`SELECT id, operation FROM pending_changes WHERE path = ?`, path,
	).Scan(&existingID, &existingOp)

	if err == sql.ErrNoRows {
		// No existing entry — insert new.
		_, err := j.db.Exec(`
INSERT INTO pending_changes (path, operation, hash, size, timestamp)
VALUES (?, ?, ?, ?, ?)`, path, operation, hash, size, now)
		if err != nil {
			return fmt.Errorf("inserting pending change: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("querying existing change: %w", err)
	}

	// If existing is "create" and new is "delete", remove (net zero).
	if existingOp == JournalOpCreate && operation == JournalOpDelete {
		_, err := j.db.Exec(`DELETE FROM pending_changes WHERE id = ?`, existingID)
		if err != nil {
			return fmt.Errorf("removing net-zero change: %w", err)
		}
		return nil
	}

	// Update existing entry.
	_, err = j.db.Exec(`
UPDATE pending_changes
SET operation = ?, hash = ?, size = ?, timestamp = ?
WHERE id = ?`, operation, hash, size, now, existingID)
	if err != nil {
		return fmt.Errorf("updating pending change: %w", err)
	}

	return nil
}

// GetPendingChanges returns all pending changes ordered by timestamp.
func (j *Journal) GetPendingChanges() ([]PendingChange, error) {
	rows, err := j.db.Query(`
SELECT id, path, operation, hash, size, timestamp
FROM pending_changes
ORDER BY timestamp ASC`)
	if err != nil {
		return nil, fmt.Errorf("querying pending changes: %w", err)
	}
	defer rows.Close()

	var changes []PendingChange
	for rows.Next() {
		var c PendingChange
		if err := rows.Scan(&c.ID, &c.Path, &c.Operation, &c.Hash, &c.Size, &c.Timestamp); err != nil {
			return nil, fmt.Errorf("scanning pending change: %w", err)
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// MarkSynced removes a pending change by path, indicating it has been
// successfully synced to the server.
func (j *Journal) MarkSynced(path string) error {
	_, err := j.db.Exec(`DELETE FROM pending_changes WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("marking change synced: %w", err)
	}
	return nil
}

// ClearPending removes all pending changes from the journal.
func (j *Journal) ClearPending() error {
	_, err := j.db.Exec(`DELETE FROM pending_changes`)
	if err != nil {
		return fmt.Errorf("clearing pending changes: %w", err)
	}
	return nil
}

// PendingCount returns the number of pending changes.
func (j *Journal) PendingCount() (int, error) {
	var count int
	err := j.db.QueryRow(`SELECT COUNT(*) FROM pending_changes`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting pending changes: %w", err)
	}
	return count, nil
}

// GetMetadata retrieves a value from the sync_metadata table by key.
// Returns ("", nil) if the key does not exist.
func (j *Journal) GetMetadata(key string) (string, error) {
	var value string
	err := j.db.QueryRow(`SELECT value FROM sync_metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying metadata for key %q: %w", key, err)
	}
	return value, nil
}

// SetMetadata upserts a key-value pair into the sync_metadata table.
func (j *Journal) SetMetadata(key, value string) error {
	_, err := j.db.Exec(`
INSERT INTO sync_metadata (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	if err != nil {
		return fmt.Errorf("setting metadata key %q: %w", key, err)
	}
	return nil
}

// LastSync returns the timestamp of the last successful sync as reported
// by the syncer. Returns ("", nil) if no sync has been recorded.
func (j *Journal) LastSync() (string, error) {
	return j.GetMetadata("last_sync")
}

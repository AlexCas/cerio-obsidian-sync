package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
	_ "modernc.org/sqlite"
)

// Store implements content-addressed file storage backed by SQLite for metadata
// and a sharded directory layout for file blobs.
type Store struct {
	db      *sql.DB
	dataDir string
}

// New creates a new Store. Call Open to connect and initialize.
func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// Open connects to the SQLite database at dbPath and runs pending migrations.
// The data directory is created if it does not exist.
func (s *Store) Open(dbPath string) error {
	var err error
	s.db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("setting WAL mode: %w", err)
	}

	if err := RunMigrations(s.db); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Ensure data directory exists.
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying database connection for use by other packages
// (e.g., middleware that needs to validate API keys).
func (s *Store) DB() *sql.DB {
	return s.db
}

// PutFile stores a file using content-addressed storage. The content is written
// to a sharded path under {dataDir}/{vaultID}/objects/{hash[:2]}/{hash}.
// If content with the same hash already exists on disk, the blob is not
// rewritten (dedup). The metadata entry in the files table is upserted.
func (s *Store) PutFile(vaultID, path, hash string, content io.Reader) error {
	// Read content and compute hash to verify.
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading file content: %w", err)
	}

	// Verify hash matches.
	computed := fmt.Sprintf("%x", sha256.Sum256(data))
	if computed != hash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", hash, computed)
	}

	// Write content blob (dedup: skip if already exists).
	objDir := filepath.Join(s.dataDir, vaultID, "objects", hash[:2])
	objPath := filepath.Join(objDir, hash)

	if _, err := os.Stat(objPath); os.IsNotExist(err) {
		if err := os.MkdirAll(objDir, 0o755); err != nil {
			return fmt.Errorf("creating object directory: %w", err)
		}
		if err := os.WriteFile(objPath, data, 0o644); err != nil {
			return fmt.Errorf("writing object file: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Upsert metadata.
	_, err = s.db.Exec(`
INSERT INTO files (vault_id, path, hash, size, modified_at, revision, deleted_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 0, NULL, ?, ?)
ON CONFLICT(vault_id, path) DO UPDATE SET
    hash = excluded.hash,
    size = excluded.size,
    modified_at = excluded.modified_at,
    deleted_at = NULL,
    updated_at = excluded.updated_at`,
		vaultID, path, hash, len(data), now, now, now,
	)
	if err != nil {
		return fmt.Errorf("upserting file metadata: %w", err)
	}

	return nil
}

// GetFile retrieves a file's content from content-addressed storage.
func (s *Store) GetFile(vaultID, hash string) (io.ReadCloser, error) {
	objPath := filepath.Join(s.dataDir, vaultID, "objects", hash[:2], hash)
	f, err := os.Open(objPath)
	if err != nil {
		return nil, fmt.Errorf("opening object file: %w", err)
	}
	return f, nil
}

// DeleteFile soft-deletes a file by setting the deleted_at timestamp.
func (s *Store) DeleteFile(vaultID, path string) error {
	return MarkFileDeleted(s, vaultID, path)
}

// GetManifest returns all non-deleted files for a vault.
func (s *Store) GetManifest(vaultID string) ([]protocol.FileEntry, error) {
	rows, err := s.db.Query(`
SELECT path, hash, size, modified_at
FROM files
WHERE vault_id = ? AND deleted_at IS NULL
ORDER BY path`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("querying manifest: %w", err)
	}
	defer rows.Close()

	var entries []protocol.FileEntry
	for rows.Next() {
		var e protocol.FileEntry
		if err := rows.Scan(&e.Path, &e.Hash, &e.Size, &e.ModifiedAt); err != nil {
			return nil, fmt.Errorf("scanning manifest entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetChangesSince returns all file changes for a vault since the given revision ID.
// It collapses multiple operations on the same file to only the latest one,
// and filters out create/update operations for files that are currently soft-deleted
// (the client should only see the "delete" operation for those).
func (s *Store) GetChangesSince(vaultID string, revision int64) ([]protocol.FileChange, error) {
	rows, err := s.db.Query(`
SELECT r.file_path, r.hash, r.size, r.timestamp, r.operation
FROM revisions r
WHERE r.vault_id = ? AND r.id > ?
ORDER BY r.id`, vaultID, revision)
	if err != nil {
		return nil, fmt.Errorf("querying changes: %w", err)
	}
	defer rows.Close()

	// Collect all changes, keeping only the latest per file path.
	latestByPath := make(map[string]protocol.FileChange)
	for rows.Next() {
		var c protocol.FileChange
		if err := rows.Scan(&c.Path, &c.Hash, &c.Size, &c.ModifiedAt, &c.Operation); err != nil {
			return nil, fmt.Errorf("scanning change: %w", err)
		}
		latestByPath[c.Path] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating changes: %w", err)
	}

	// For files whose latest operation is create/update, check if they are
	// currently soft-deleted on the server. If so, skip them — the client
	// shouldn't download a file that was deleted.
	var changes []protocol.FileChange
	for _, c := range latestByPath {
		if c.Operation != protocol.OperationDelete {
			// Check if this file is currently soft-deleted.
			// If the file exists in the files table AND is deleted, skip it.
			deleted, err := IsFileDeleted(s, vaultID, c.Path)
			if err != nil {
				// File not in files table — it was tracked via revisions only.
				// This is normal for files not yet uploaded through PutFile.
				// Include the change.
			} else if deleted {
				// File was created then soft-deleted; the create revision is
				// stale. The client will not see this file in the manifest either.
				// Skip it entirely for initial sync.
				continue
			}
		}
		changes = append(changes, c)
	}

	return changes, nil
}

// RecordRevision appends a revision entry and returns the new revision ID.
func (s *Store) RecordRevision(vaultID, clientID, filePath, operation, hash string, size int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := s.db.Exec(`
INSERT INTO revisions (vault_id, file_path, operation, hash, size, timestamp, client_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		vaultID, filePath, operation, hash, size, now, clientID)
	if err != nil {
		return 0, fmt.Errorf("inserting revision: %w", err)
	}

	revID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting revision id: %w", err)
	}

	// Update client revision tracking.
	_, err = s.db.Exec(`
INSERT INTO client_revisions (vault_id, client_id, revision, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(vault_id, client_id) DO UPDATE SET
    revision = excluded.revision,
    updated_at = excluded.updated_at`,
		vaultID, clientID, revID, now)
	if err != nil {
		return 0, fmt.Errorf("updating client revision: %w", err)
	}

	return revID, nil
}

// GetFileByPath returns file metadata by vault and path.
func (s *Store) GetFileByPath(vaultID, path string) (*protocol.FileEntry, error) {
	var e protocol.FileEntry
	var deletedAt sql.NullString
	err := s.db.QueryRow(`
SELECT path, hash, size, modified_at, deleted_at
FROM files
WHERE vault_id = ? AND path = ?`, vaultID, path).Scan(
		&e.Path, &e.Hash, &e.Size, &e.ModifiedAt, &deletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying file by path: %w", err)
	}
	return &e, nil
}

// ValidateAPIKey checks if the given key exists in the api_keys table.
// Returns the vault_id associated with the key, or empty string if not found.
// Also updates last_used_at.
func (s *Store) ValidateAPIKey(keyHash string) (string, error) {
	var vaultID string
	err := s.db.QueryRow(`
SELECT vault_id FROM api_keys WHERE key_hash = ?`, keyHash).Scan(&vaultID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying api key: %w", err)
	}

	// Update last_used_at.
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = datetime('now') WHERE key_hash = ?`, keyHash)

	return vaultID, nil
}

// CreateAPIKey inserts a new API key into the store.
func (s *Store) CreateAPIKey(keyHash, vaultID, name string) error {
	_, err := s.db.Exec(`
INSERT INTO api_keys (key_hash, vault_id, name) VALUES (?, ?, ?)`,
		keyHash, vaultID, name)
	if err != nil {
		return fmt.Errorf("inserting api key: %w", err)
	}
	return nil
}

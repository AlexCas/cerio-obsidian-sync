package store

import (
	"database/sql"
	"fmt"
	"time"
)

// MarkFileDeleted sets the deleted_at timestamp on a file, keeping its content
// available for restoration within the grace period. It also records a "delete"
// revision so that clients can learn about the deletion via GetChangesSince.
func MarkFileDeleted(s *Store, vaultID, path string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB().Exec(`
UPDATE files
SET deleted_at = ?, updated_at = ?
WHERE vault_id = ? AND path = ? AND deleted_at IS NULL`,
		now, now, vaultID, path)
	if err != nil {
		return fmt.Errorf("marking file deleted: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("file not found or already deleted: %s/%s", vaultID, path)
	}

	// Record a "delete" revision so clients are notified of the deletion.
	if _, err := s.DB().Exec(`
INSERT INTO revisions (vault_id, file_path, operation, hash, size, timestamp, client_id)
VALUES (?, ?, 'delete', '', 0, ?, 'server')`,
		vaultID, path, now); err != nil {
		return fmt.Errorf("recording delete revision: %w", err)
	}

	return nil
}

// RestoreFile clears the deleted_at timestamp on a file, making it visible
// again in the manifest.
func RestoreFile(s *Store, vaultID, path string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB().Exec(`
UPDATE files
SET deleted_at = NULL, updated_at = ?
WHERE vault_id = ? AND path = ? AND deleted_at IS NOT NULL`,
		now, vaultID, path)
	if err != nil {
		return fmt.Errorf("restoring file: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("file not found or not deleted: %s/%s", vaultID, path)
	}

	return nil
}

// CleanupExpired permanently removes files that have been soft-deleted longer
// than the grace period. It deletes the metadata row from SQLite; the content
// blob on disk is left in place (it may be referenced by another file due to
// dedup). Actual blob cleanup would require reference counting.
func CleanupExpired(s *Store, graceDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -graceDays).Format(time.RFC3339)

	result, err := s.DB().Exec(`
DELETE FROM files
WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("cleaning up expired files: %w", err)
	}

	rows, _ := result.RowsAffected()
	_ = rows // caller can inspect if needed

	return nil
}

// IsFileDeleted checks whether a file is currently soft-deleted.
func IsFileDeleted(s *Store, vaultID, path string) (bool, error) {
	var deletedAt sql.NullString
	err := s.DB().QueryRow(`
SELECT deleted_at FROM files WHERE vault_id = ? AND path = ?`,
		vaultID, path).Scan(&deletedAt)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("file not found: %s/%s", vaultID, path)
	}
	if err != nil {
		return false, fmt.Errorf("checking file deleted state: %w", err)
	}
	return deletedAt.Valid, nil
}

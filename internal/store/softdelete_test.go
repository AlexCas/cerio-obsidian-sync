package store

import (
	"bytes"
	"testing"
)

func TestSoftDelete_MarksFile(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("delete me")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/doomed.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	// File should not be deleted.
	deleted, err := IsFileDeleted(s, "vault1", "notes/doomed.md")
	if err != nil {
		t.Fatalf("IsFileDeleted before delete: %v", err)
	}
	if deleted {
		t.Error("file should not be marked deleted initially")
	}

	// Soft-delete.
	if err := MarkFileDeleted(s, "vault1", "notes/doomed.md"); err != nil {
		t.Fatalf("MarkFileDeleted: %v", err)
	}

	deleted, err = IsFileDeleted(s, "vault1", "notes/doomed.md")
	if err != nil {
		t.Fatalf("IsFileDeleted after delete: %v", err)
	}
	if !deleted {
		t.Error("file should be marked deleted after MarkFileDeleted")
	}

	// File should not appear in manifest.
	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	for _, e := range manifest {
		if e.Path == "notes/doomed.md" {
			t.Error("soft-deleted file should not appear in manifest")
		}
	}
}

func TestSoftDelete_RestoreRecovers(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("restore me")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/recover.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	if err := MarkFileDeleted(s, "vault1", "notes/recover.md"); err != nil {
		t.Fatalf("MarkFileDeleted: %v", err)
	}

	// Restore.
	if err := RestoreFile(s, "vault1", "notes/recover.md"); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	deleted, err := IsFileDeleted(s, "vault1", "notes/recover.md")
	if err != nil {
		t.Fatalf("IsFileDeleted after restore: %v", err)
	}
	if deleted {
		t.Error("file should not be deleted after RestoreFile")
	}

	// File should reappear in manifest.
	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	found := false
	for _, e := range manifest {
		if e.Path == "notes/recover.md" {
			found = true
		}
	}
	if !found {
		t.Error("restored file should appear in manifest")
	}
}

func TestSoftDelete_CleanupRemovesExpired(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("expired content")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/old.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	// Mark deleted.
	if err := MarkFileDeleted(s, "vault1", "notes/old.md"); err != nil {
		t.Fatalf("MarkFileDeleted: %v", err)
	}

	// Manually set deleted_at to 31 days ago to simulate expiration.
	_, err := s.DB().Exec(`
UPDATE files SET deleted_at = datetime('now', '-31 days')
WHERE vault_id = ? AND path = ?`, "vault1", "notes/old.md")
	if err != nil {
		t.Fatalf("setting old deleted_at: %v", err)
	}

	// Cleanup with 30-day grace.
	if err := CleanupExpired(s, 30); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}

	// File should be gone entirely.
	entry, err := s.GetFileByPath("vault1", "notes/old.md")
	if err != nil {
		t.Fatalf("GetFileByPath: %v", err)
	}
	if entry != nil {
		t.Error("expired file should be hard-deleted after cleanup")
	}
}

func TestSoftDelete_CleanupKeepsRecent(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("recent delete")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/recent.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	// Mark deleted (just now).
	if err := MarkFileDeleted(s, "vault1", "notes/recent.md"); err != nil {
		t.Fatalf("MarkFileDeleted: %v", err)
	}

	// Cleanup with 30-day grace — should not remove this file.
	if err := CleanupExpired(s, 30); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}

	// File should still exist (soft-deleted).
	deleted, err := IsFileDeleted(s, "vault1", "notes/recent.md")
	if err != nil {
		t.Fatalf("IsFileDeleted: %v", err)
	}
	if !deleted {
		t.Error("recently deleted file should still be tracked")
	}
}

func TestSoftDelete_DoubleDeleteFails(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("double delete")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/double.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	if err := MarkFileDeleted(s, "vault1", "notes/double.md"); err != nil {
		t.Fatalf("first MarkFileDeleted: %v", err)
	}

	err := MarkFileDeleted(s, "vault1", "notes/double.md")
	if err == nil {
		t.Error("expected error on double delete, got nil")
	}
}

func TestSoftDelete_RestoreNonDeletedFails(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("not deleted")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "notes/alive.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	err := RestoreFile(s, "vault1", "notes/alive.md")
	if err == nil {
		t.Error("expected error when restoring non-deleted file, got nil")
	}
}

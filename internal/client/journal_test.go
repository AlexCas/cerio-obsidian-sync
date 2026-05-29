package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournal_CRUD(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// Record a create.
	if err := j.RecordChange("notes/test.md", JournalOpCreate, "abc123", 100); err != nil {
		t.Fatalf("RecordChange create: %v", err)
	}

	// Record an update.
	if err := j.RecordChange("notes/test.md", JournalOpUpdate, "def456", 120); err != nil {
		t.Fatalf("RecordChange update: %v", err)
	}

	// Get pending changes — should have one entry (updated).
	changes, err := j.GetPendingChanges()
	if err != nil {
		t.Fatalf("GetPendingChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change, got %d", len(changes))
	}
	if changes[0].Path != "notes/test.md" {
		t.Errorf("expected path notes/test.md, got %s", changes[0].Path)
	}
	if changes[0].Operation != JournalOpUpdate {
		t.Errorf("expected operation %s, got %s", JournalOpUpdate, changes[0].Operation)
	}
	if changes[0].Hash != "def456" {
		t.Errorf("expected hash def456, got %s", changes[0].Hash)
	}
	if changes[0].Size != 120 {
		t.Errorf("expected size 120, got %d", changes[0].Size)
	}

	// Record a second file.
	if err := j.RecordChange("README.md", JournalOpCreate, "ghi789", 50); err != nil {
		t.Fatalf("RecordChange second file: %v", err)
	}

	count, err := j.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 pending changes, got %d", count)
	}

	// Mark one as synced.
	if err := j.MarkSynced("notes/test.md"); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}

	count, err = j.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount after sync: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending change after sync, got %d", count)
	}
}

func TestJournal_CreateThenDelete_CancelsOut(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// Record a create.
	if err := j.RecordChange("temp.md", JournalOpCreate, "abc", 10); err != nil {
		t.Fatalf("RecordChange create: %v", err)
	}

	// Then delete it — should cancel out.
	if err := j.RecordChange("temp.md", JournalOpDelete, "", 0); err != nil {
		t.Fatalf("RecordChange delete: %v", err)
	}

	count, err := j.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pending changes after create+delete, got %d", count)
	}
}

func TestJournal_PersistAcrossOpenClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "journal.db")

	// Open, write, close.
	j := NewJournal(dbPath)
	if err := j.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := j.RecordChange("persist.md", JournalOpCreate, "hash1", 42); err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen, verify data persists.
	j2 := NewJournal(dbPath)
	if err := j2.Open(); err != nil {
		t.Fatalf("Open second time: %v", err)
	}
	defer j2.Close()

	changes, err := j2.GetPendingChanges()
	if err != nil {
		t.Fatalf("GetPendingChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change after reopen, got %d", len(changes))
	}
	if changes[0].Path != "persist.md" {
		t.Errorf("expected path persist.md, got %s", changes[0].Path)
	}
	if changes[0].Hash != "hash1" {
		t.Errorf("expected hash hash1, got %s", changes[0].Hash)
	}
}

func TestJournal_ClearPending(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// Add several changes.
	if err := j.RecordChange("a.md", JournalOpCreate, "a", 1); err != nil {
		t.Fatalf("RecordChange a: %v", err)
	}
	if err := j.RecordChange("b.md", JournalOpUpdate, "b", 2); err != nil {
		t.Fatalf("RecordChange b: %v", err)
	}
	if err := j.RecordChange("c.md", JournalOpDelete, "", 0); err != nil {
		t.Fatalf("RecordChange c: %v", err)
	}

	count, err := j.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 pending changes, got %d", count)
	}

	// Clear all.
	if err := j.ClearPending(); err != nil {
		t.Fatalf("ClearPending: %v", err)
	}

	count, err = j.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount after clear: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pending changes after clear, got %d", count)
	}
}

func TestJournal_MarkSyncedNonExistent(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// MarkSynced on a path that doesn't exist should not error.
	if err := j.MarkSynced("nonexistent.md"); err != nil {
		t.Errorf("MarkSynced on nonexistent path should not error: %v", err)
	}
}

func TestJournal_UpdateDeleteOnExistingUpdate(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// Update then delete — should remain as delete (not cancel out).
	if err := j.RecordChange("modified.md", JournalOpUpdate, "v1", 100); err != nil {
		t.Fatalf("RecordChange update: %v", err)
	}
	if err := j.RecordChange("modified.md", JournalOpDelete, "", 0); err != nil {
		t.Fatalf("RecordChange delete: %v", err)
	}

	changes, err := j.GetPendingChanges()
	if err != nil {
		t.Fatalf("GetPendingChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Operation != JournalOpDelete {
		t.Errorf("expected operation %s, got %s", JournalOpDelete, changes[0].Operation)
	}
}

func TestJournal_WALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "journal.db")

	j := NewJournal(dbPath)
	if err := j.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer j.Close()

	// Verify WAL mode is set.
	var mode string
	err := j.DB().QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("checking journal mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected WAL mode, got %s", mode)
	}

	// Verify the WAL file was created.
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); err != nil {
		t.Logf("WAL file not found at %s (may be created on first write): %v", walPath, err)
	}
}

func TestJournal_MetadataCRUD(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// GetMetadata on a non-existent key returns empty string.
	val, err := j.GetMetadata("last_sync")
	if err != nil {
		t.Fatalf("GetMetadata non-existent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for non-existent key, got %q", val)
	}

	// SetMetadata upserts a key.
	if err := j.SetMetadata("last_sync", "2026-05-28T14:30:00Z"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	val, err = j.GetMetadata("last_sync")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if val != "2026-05-28T14:30:00Z" {
		t.Errorf("expected last_sync timestamp, got %q", val)
	}

	// SetMetadata can update an existing key.
	if err := j.SetMetadata("last_sync", "2026-05-29T10:00:00Z"); err != nil {
		t.Fatalf("SetMetadata update: %v", err)
	}

	val, err = j.GetMetadata("last_sync")
	if err != nil {
		t.Fatalf("GetMetadata after update: %v", err)
	}
	if val != "2026-05-29T10:00:00Z" {
		t.Errorf("expected updated timestamp, got %q", val)
	}
}

func TestJournal_LastSync(t *testing.T) {
	j := NewJournal("")
	if err := j.OpenInMemory(); err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer j.Close()

	// LastSync returns empty when no sync has been recorded.
	lastSync, err := j.LastSync()
	if err != nil {
		t.Fatalf("LastSync: %v", err)
	}
	if lastSync != "" {
		t.Errorf("expected empty last_sync, got %q", lastSync)
	}

	// Set and retrieve last sync.
	if err := j.SetMetadata("last_sync", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	lastSync, err = j.LastSync()
	if err != nil {
		t.Fatalf("LastSync after set: %v", err)
	}
	if lastSync != "2026-01-01T00:00:00Z" {
		t.Errorf("expected last_sync timestamp, got %q", lastSync)
	}
}

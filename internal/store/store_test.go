package store

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dataDir := filepath.Join(tmpDir, "data")

	s := New(dataDir)
	if err := s.Open(dbPath); err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, tmpDir
}

func hashContent(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func TestStore_OpenCreatesDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "datastore")

	s := New(dataDir)
	if err := s.Open(filepath.Join(tmpDir, "test.db")); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("Open did not create data directory")
	}
}

func TestStore_PutFile_GetFile_RoundTrip(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("hello obsidian sync")
	hash := hashContent(content)
	vaultID := "vault1"
	path := "notes/test.md"

	err := s.PutFile(vaultID, path, hash, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	rc, err := s.GetFile(vaultID, hash)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Errorf("GetFile content mismatch: got %q, want %q", got, content)
	}
}

func TestStore_PutFile_HashMismatch(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("some content")
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	err := s.PutFile("vault1", "file.md", wrongHash, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected error for hash mismatch, got nil")
	}
}

func TestStore_PutFile_ContentDedup(t *testing.T) {
	s, tmpDir := openTestStore(t)

	content := []byte("duplicate content")
	hash := hashContent(content)

	// Put same content under two different paths.
	err := s.PutFile("vault1", "notes/a.md", hash, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutFile first: %v", err)
	}
	err = s.PutFile("vault1", "notes/b.md", hash, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutFile second: %v", err)
	}

	// Object file should exist exactly once.
	objPath := filepath.Join(tmpDir, "data", "vault1", "objects", hash[:2], hash)
	if _, err := os.Stat(objPath); os.IsNotExist(err) {
		t.Fatal("object file should exist after PutFile")
	}

	// Both files should appear in manifest.
	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Errorf("expected 2 manifest entries, got %d", len(manifest))
	}
}

func TestStore_GetManifest_AllFiles(t *testing.T) {
	s, _ := openTestStore(t)

	files := map[string]string{
		"notes/a.md": "content A",
		"notes/b.md": "content B",
		"notes/c.md": "content C",
	}
	for p, c := range files {
		hash := hashContent([]byte(c))
		if err := s.PutFile("vault1", p, hash, bytes.NewReader([]byte(c))); err != nil {
			t.Fatalf("PutFile %s: %v", p, err)
		}
	}

	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(manifest) != len(files) {
		t.Errorf("expected %d manifest entries, got %d", len(files), len(manifest))
	}
}

func TestStore_GetManifest_ExcludesDeleted(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("to be deleted")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "doomed.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	if err := s.DeleteFile("vault1", "doomed.md"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(manifest) != 0 {
		t.Errorf("expected 0 manifest entries after delete, got %d", len(manifest))
	}
}

func TestStore_RecordRevision(t *testing.T) {
	s, _ := openTestStore(t)

	revID, err := s.RecordRevision("vault1", "client1", "notes/a.md", protocol.OperationCreate, "abc123", 42)
	if err != nil {
		t.Fatalf("RecordRevision: %v", err)
	}
	if revID < 1 {
		t.Errorf("expected revision ID >= 1, got %d", revID)
	}

	revID2, err := s.RecordRevision("vault1", "client1", "notes/b.md", protocol.OperationUpdate, "def456", 100)
	if err != nil {
		t.Fatalf("RecordRevision 2: %v", err)
	}
	if revID2 <= revID {
		t.Errorf("expected second revision ID > first, got %d <= %d", revID2, revID)
	}
}

func TestStore_GetChangesSince(t *testing.T) {
	s, _ := openTestStore(t)

	// Record two revisions.
	rev1, _ := s.RecordRevision("vault1", "client1", "notes/a.md", protocol.OperationCreate, "hash1", 10)
	s.RecordRevision("vault1", "client1", "notes/b.md", protocol.OperationUpdate, "hash2", 20)

	// Get changes since revision 0 (all).
	changes, err := s.GetChangesSince("vault1", 0)
	if err != nil {
		t.Fatalf("GetChangesSince: %v", err)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changes since rev 0, got %d", len(changes))
	}

	// Get changes since rev1 (only the second).
	changes2, err := s.GetChangesSince("vault1", rev1)
	if err != nil {
		t.Fatalf("GetChangesSince rev1: %v", err)
	}
	if len(changes2) != 1 {
		t.Errorf("expected 1 change since rev %d, got %d", rev1, len(changes2))
	}
}

func TestStore_PutFile_UpsertsOnConflict(t *testing.T) {
	s, _ := openTestStore(t)

	v1 := []byte("version 1")
	h1 := hashContent(v1)
	if err := s.PutFile("vault1", "notes/a.md", h1, bytes.NewReader(v1)); err != nil {
		t.Fatalf("PutFile v1: %v", err)
	}

	v2 := []byte("version 2")
	h2 := hashContent(v2)
	if err := s.PutFile("vault1", "notes/a.md", h2, bytes.NewReader(v2)); err != nil {
		t.Fatalf("PutFile v2: %v", err)
	}

	// Manifest should have only one entry for this path.
	manifest, err := s.GetManifest("vault1")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("expected 1 manifest entry after upsert, got %d", len(manifest))
	}
	if manifest[0].Hash != h2 {
		t.Errorf("expected hash %s after upsert, got %s", h2, manifest[0].Hash)
	}
}

func TestStore_ValidateAPIKey(t *testing.T) {
	s, _ := openTestStore(t)

	keyHash := "abc123def456"
	vaultID := "vault1"

	if err := s.CreateAPIKey(keyHash, vaultID, "test-key"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	gotVault, err := s.ValidateAPIKey(keyHash)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if gotVault != vaultID {
		t.Errorf("expected vaultID %q, got %q", vaultID, gotVault)
	}

	// Non-existent key.
	gotVault2, err := s.ValidateAPIKey("nonexistent")
	if err != nil {
		t.Fatalf("ValidateAPIKey nonexistent: %v", err)
	}
	if gotVault2 != "" {
		t.Errorf("expected empty vaultID for nonexistent key, got %q", gotVault2)
	}
}

func TestStore_GetFileByPath(t *testing.T) {
	s, _ := openTestStore(t)

	content := []byte("find me by path")
	hash := hashContent(content)
	if err := s.PutFile("vault1", "deep/nested/file.md", hash, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	entry, err := s.GetFileByPath("vault1", "deep/nested/file.md")
	if err != nil {
		t.Fatalf("GetFileByPath: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, entry.Hash)
	}

	// Non-existent path.
	entry2, err := s.GetFileByPath("vault1", "nonexistent.md")
	if err != nil {
		t.Fatalf("GetFileByPath nonexistent: %v", err)
	}
	if entry2 != nil {
		t.Error("expected nil for nonexistent path")
	}
}

package client

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

func TestManifestBuilder_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(manifest.Entries) != 0 {
		t.Errorf("expected 0 entries in empty vault, got %d", len(manifest.Entries))
	}
}

func TestManifestBuilder_BasicFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files.
	if err := os.WriteFile(filepath.Join(dir, "note1.md"), []byte("hello world"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note2.md"), []byte("another note"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir", "deep.md"), []byte("deep file"), 0o644); err != nil {
		t.Fatalf("creating deep file: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(manifest.Entries))
	}

	// Verify entries are sorted by path (filepath.Walk order).
	paths := make([]string, len(manifest.Entries))
	for i, e := range manifest.Entries {
		paths[i] = e.Path
	}

	expectedPaths := []string{"note1.md", "note2.md", "subdir/deep.md"}
	for i, exp := range expectedPaths {
		if paths[i] != exp {
			t.Errorf("entry %d: expected path %s, got %s", i, exp, paths[i])
		}
	}
}

func TestManifestBuilder_SHA256Correctness(t *testing.T) {
	dir := t.TempDir()

	content := []byte("test content for hashing")
	if err := os.WriteFile(filepath.Join(dir, "test.md"), content, 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	expectedHash := ComputeHash(content)

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(manifest.Entries))
	}

	if manifest.Entries[0].Hash != expectedHash {
		t.Errorf("hash mismatch: expected %s, got %s", expectedHash, manifest.Entries[0].Hash)
	}
}

func TestManifestBuilder_ExcludesObsidian(t *testing.T) {
	dir := t.TempDir()

	// Create normal file.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("content"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	// Create .obsidian directory with files.
	if err := os.MkdirAll(filepath.Join(dir, ".obsidian"), 0o755); err != nil {
		t.Fatalf("creating .obsidian dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".obsidian", "config"), []byte("config data"), 0o644); err != nil {
		t.Fatalf("creating .obsidian config: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath:     dir,
		ExcludedPaths: []string{".obsidian"},
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry (excluding .obsidian), got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].Path != "note.md" {
		t.Errorf("expected note.md, got %s", manifest.Entries[0].Path)
	}
}

func TestManifestBuilder_ExcludesOSync(t *testing.T) {
	dir := t.TempDir()

	// Create normal file.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("content"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	// Create .osync directory with files (always excluded).
	if err := os.MkdirAll(filepath.Join(dir, ".osync"), 0o755); err != nil {
		t.Fatalf("creating .osync dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".osync", "config.yaml"), []byte("config"), 0o644); err != nil {
		t.Fatalf("creating .osync config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".osync", "journal.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("creating .osync journal: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry (excluding .osync), got %d", len(manifest.Entries))
	}
}

func TestManifestBuilder_SkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a normal small file.
	if err := os.WriteFile(filepath.Join(dir, "small.md"), []byte("small"), 0o644); err != nil {
		t.Fatalf("creating small file: %v", err)
	}

	// Create a file that exceeds the size limit.
	largeContent := make([]byte, 101)
	if err := os.WriteFile(filepath.Join(dir, "large.md"), largeContent, 0o644); err != nil {
		t.Fatalf("creating large file: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath:   dir,
		MaxFileSize: 100, // 100 bytes limit
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry (skipping large file), got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].Path != "small.md" {
		t.Errorf("expected small.md, got %s", manifest.Entries[0].Path)
	}
}

func TestManifestBuilder_DefaultMaxFileSize(t *testing.T) {
	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: t.TempDir(),
	})
	if mb.maxFileSize != protocol.MaxFileSize {
		t.Errorf("expected default max file size %d, got %d", protocol.MaxFileSize, mb.maxFileSize)
	}
}

func TestManifestBuilder_DefaultExcludedPaths(t *testing.T) {
	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: t.TempDir(),
	})
	if len(mb.excludedPaths) != 1 || mb.excludedPaths[0] != ".obsidian" {
		t.Errorf("expected default excluded paths [.obsidian], got %v", mb.excludedPaths)
	}
}

func TestManifestBuilder_ModifiedAt(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "timed.md"), []byte("content"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(manifest.Entries))
	}

	if manifest.Entries[0].ModifiedAt == "" {
		t.Error("expected non-empty modified_at")
	}
	// Should be in RFC3339-like format.
	if len(manifest.Entries[0].ModifiedAt) < 10 {
		t.Errorf("modified_at seems too short: %s", manifest.Entries[0].ModifiedAt)
	}
}

func TestManifestBuilder_CustomExcludedPaths(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("content"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatalf("creating templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "daily.md"), []byte("template"), 0o644); err != nil {
		t.Fatalf("creating template file: %v", err)
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath:     dir,
		ExcludedPaths: []string{".obsidian", "templates"},
	})

	manifest, err := mb.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry (excluding templates), got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].Path != "note.md" {
		t.Errorf("expected note.md, got %s", manifest.Entries[0].Path)
	}
}

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "empty",
			data:     []byte(""),
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world",
			data:     []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeHash(tt.data)
			if got != tt.expected {
				t.Errorf("ComputeHash(%q) = %s, want %s", tt.name, got, tt.expected)
			}
		})
	}
}

func TestManifestBuilder_BuildPaginated_SmallVault(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("note%d.md", i)), []byte("content"), 0o644); err != nil {
			t.Fatalf("creating test file: %v", err)
		}
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	// Request pages of size 10 — should fit in one page.
	pages, err := mb.BuildPaginated(10)
	if err != nil {
		t.Fatalf("BuildPaginated: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page for 5 entries, got %d", len(pages))
	}
	if pages[0].Page != 1 {
		t.Errorf("expected page 1, got page %d", pages[0].Page)
	}
	if len(pages[0].Entries) != 5 {
		t.Errorf("expected 5 entries on page, got %d", len(pages[0].Entries))
	}
	if pages[0].Total != 5 {
		t.Errorf("expected total 5, got %d", pages[0].Total)
	}
}

func TestManifestBuilder_BuildPaginated_MultiplePages(t *testing.T) {
	dir := t.TempDir()

	// Create 25 files.
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("note%02d.md", i)), []byte(fmt.Sprintf("content %d", i)), 0o644); err != nil {
			t.Fatalf("creating test file: %v", err)
		}
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	// Request pages of size 10 — should produce 3 pages (10, 10, 5).
	pages, err := mb.BuildPaginated(10)
	if err != nil {
		t.Fatalf("BuildPaginated: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages for 25 entries with pageSize 10, got %d", len(pages))
	}

	if len(pages[0].Entries) != 10 {
		t.Errorf("page 1: expected 10 entries, got %d", len(pages[0].Entries))
	}
	if len(pages[1].Entries) != 10 {
		t.Errorf("page 2: expected 10 entries, got %d", len(pages[1].Entries))
	}
	if len(pages[2].Entries) != 5 {
		t.Errorf("page 3: expected 5 entries, got %d", len(pages[2].Entries))
	}

	if pages[0].Total != 25 {
		t.Errorf("page 1 total: expected 25, got %d", pages[0].Total)
	}
}

func TestManifestBuilder_BuildPaginated_EmptyVault(t *testing.T) {
	dir := t.TempDir()

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	pages, err := mb.BuildPaginated(10)
	if err != nil {
		t.Fatalf("BuildPaginated: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page for empty vault, got %d", len(pages))
	}
	if len(pages[0].Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(pages[0].Entries))
	}
}

func TestManifestBuilder_BuildPaginated_ZeroPageSize(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("note%d.md", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("creating test file: %v", err)
		}
	}

	mb := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath: dir,
	})

	// Zero page size should return a single page with all entries.
	pages, err := mb.BuildPaginated(0)
	if err != nil {
		t.Fatalf("BuildPaginated: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page for zero pageSize, got %d", len(pages))
	}
	if len(pages[0].Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(pages[0].Entries))
	}
}

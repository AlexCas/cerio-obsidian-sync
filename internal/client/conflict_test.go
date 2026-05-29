package client

import (
	"strings"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

func TestConflictCopyName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		ext      string
		contains string
	}{
		{
			name:     "markdown file",
			input:    "meeting-notes.md",
			ext:      ".md",
			contains: "meeting-notes (conflicted copy",
		},
		{
			name:     "no extension",
			input:    "README",
			ext:      "",
			contains: "README (conflicted copy",
		},
		{
			name:     "nested path with extension",
			input:    "notes/daily/journal.md",
			ext:      ".md",
			contains: "journal (conflicted copy",
		},
		{
			name:     "multiple dots",
			input:    "data.backup.json",
			ext:      ".json",
			contains: "data.backup (conflicted copy",
		},
		{
			name:     "canvas file",
			input:    "mindmap.canvas",
			ext:      ".canvas",
			contains: "mindmap (conflicted copy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConflictCopyName(tt.input)

			if !strings.Contains(result, tt.contains) {
				t.Errorf("ConflictCopyName(%q) = %q, want to contain %q", tt.input, result, tt.contains)
			}

			if !strings.HasSuffix(result, tt.ext) {
				t.Errorf("ConflictCopyName(%q) = %q, want suffix %q", tt.input, result, tt.ext)
			}
		})
	}
}

func TestConflictCopyName_PreservesExtension(t *testing.T) {
	result := ConflictCopyName("note.md")
	if !strings.HasSuffix(result, ".md") {
		t.Errorf("expected .md extension, got %s", result)
	}
	if strings.HasPrefix(result, "note (") {
		// The base name should be "note", not "note.md"
	}

	// Verify the format: "note (conflicted copy YYYY-MM-DD HHMMSS).md"
	if !strings.HasPrefix(result, "note (conflicted copy ") {
		t.Errorf("expected prefix 'note (conflicted copy ', got %s", result)
	}
}

func TestDetectConflict(t *testing.T) {
	tests := []struct {
		name        string
		local       protocol.FileEntry
		server      protocol.FileChange
		hasConflict bool
	}{
		{
			name: "same hash no conflict",
			local: protocol.FileEntry{
				Path: "note.md",
				Hash: "abc123",
			},
			server: protocol.FileChange{
				Path: "note.md",
				Hash: "abc123",
			},
			hasConflict: false,
		},
		{
			name: "different hash conflict",
			local: protocol.FileEntry{
				Path: "note.md",
				Hash: "abc123",
			},
			server: protocol.FileChange{
				Path: "note.md",
				Hash: "def456",
			},
			hasConflict: true,
		},
		{
			name: "different paths no conflict",
			local: protocol.FileEntry{
				Path: "note1.md",
				Hash: "abc123",
			},
			server: protocol.FileChange{
				Path: "note2.md",
				Hash: "def456",
			},
			hasConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflict := DetectConflict(tt.local, tt.server)

			if tt.hasConflict && conflict == nil {
				t.Error("expected conflict, got nil")
			}
			if !tt.hasConflict && conflict != nil {
				t.Errorf("expected no conflict, got %+v", conflict)
			}
			if conflict != nil {
				if conflict.Path != tt.local.Path {
					t.Errorf("conflict path = %q, want %q", conflict.Path, tt.local.Path)
				}
				if conflict.ConflictPath == "" {
					t.Error("conflict copy path should not be empty")
				}
			}
		})
	}
}

func TestResolveConflict(t *testing.T) {
	conflict := &ConflictInfo{
		Path: "note.md",
		LocalEntry: protocol.FileEntry{
			Path: "note.md",
			Hash: "local-hash",
		},
		ServerEntry: protocol.FileChange{
			Path: "note.md",
			Hash: "server-hash",
		},
		ConflictPath: "note (conflicted copy 2026-05-28 143052).md",
	}

	tests := []struct {
		strategy      ConflictStrategy
		wantAction    string
		wantLocalPath string
	}{
		{
			strategy:      StrategyLocalWins,
			wantAction:    "upload-local",
			wantLocalPath: "note.md",
		},
		{
			strategy:      StrategyServerWins,
			wantAction:    "download-server",
			wantLocalPath: "note.md",
		},
		{
			strategy:      StrategyKeepBoth,
			wantAction:    "keep-both",
			wantLocalPath: "note (conflicted copy 2026-05-28 143052).md",
		},
		{
			strategy:      ConflictStrategy("unknown"),
			wantAction:    "keep-both", // defaults to keep-both
			wantLocalPath: "note (conflicted copy 2026-05-28 143052).md",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			localPath, _, action := ResolveConflict(conflict, tt.strategy)

			if action != tt.wantAction {
				t.Errorf("action = %q, want %q", action, tt.wantAction)
			}
			if localPath != tt.wantLocalPath {
				t.Errorf("localPath = %q, want %q", localPath, tt.wantLocalPath)
			}
		})
	}
}

func TestDetectConflict_GeneratesConflictCopyName(t *testing.T) {
	local := protocol.FileEntry{
		Path: "meeting-notes.md",
		Hash: "hash-v1",
	}
	server := protocol.FileChange{
		Path: "meeting-notes.md",
		Hash: "hash-v2",
	}

	conflict := DetectConflict(local, server)
	if conflict == nil {
		t.Fatal("expected conflict, got nil")
	}

	if conflict.ConflictPath == "" {
		t.Error("conflict copy path should not be empty")
	}

	if !strings.Contains(conflict.ConflictPath, "meeting-notes (conflicted copy") {
		t.Errorf("conflict copy name = %q, want to contain 'meeting-notes (conflicted copy'", conflict.ConflictPath)
	}

	if !strings.HasSuffix(conflict.ConflictPath, ".md") {
		t.Errorf("conflict copy name should end with .md, got %q", conflict.ConflictPath)
	}
}

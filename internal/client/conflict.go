package client

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

// ConflictStrategy defines how a conflict between local and server files
// should be resolved.
type ConflictStrategy string

const (
	// StrategyLocalWins overwrites the server version with the local version.
	StrategyLocalWins ConflictStrategy = "local-wins"

	// StrategyServerWins overwrites the local version with the server version.
	StrategyServerWins ConflictStrategy = "server-wins"

	// StrategyKeepBoth preserves both versions — the local version becomes
	// a conflict copy and the server version replaces the original.
	StrategyKeepBoth ConflictStrategy = "keep-both"
)

// ConflictInfo describes a detected conflict between local and server files.
type ConflictInfo struct {
	Path         string
	LocalEntry   protocol.FileEntry
	ServerEntry  protocol.FileChange
	ConflictPath string // Generated conflict copy name
}

// ConflictCopyName generates a conflict copy filename following Obsidian's
// naming convention: "{base} (conflicted copy {date}).{ext}"
// The date format is YYYY-MM-DD HHmmss.
//
// Example: "meeting-notes.md" → "meeting-notes (conflicted copy 2026-05-28 143052).md"
func ConflictCopyName(originalPath string) string {
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(originalPath, ext)
	timestamp := time.Now().UTC().Format("2006-01-02 150405")

	return fmt.Sprintf("%s (conflicted copy %s)%s", base, timestamp, ext)
}

// DetectConflict checks if there is a conflict between a local file entry
// and a server file change. A conflict exists when:
//   - Both have the same path
//   - Their SHA-256 hashes differ
//
// Timestamp comparison alone does NOT determine the winner — both versions
// are preserved per the spec.
func DetectConflict(localEntry protocol.FileEntry, serverEntry protocol.FileChange) *ConflictInfo {
	if localEntry.Path != serverEntry.Path {
		return nil
	}

	if localEntry.Hash == serverEntry.Hash {
		return nil // Same content, no conflict.
	}

	return &ConflictInfo{
		Path:         localEntry.Path,
		LocalEntry:   localEntry,
		ServerEntry:  serverEntry,
		ConflictPath: ConflictCopyName(localEntry.Path),
	}
}

// ResolveConflict applies the given resolution strategy to a conflict.
// It returns the action to take:
//   - StrategyLocalWins: upload local version (local entry is kept)
//   - StrategyServerWins: download server version (server entry replaces local)
//   - StrategyKeepBoth: save local as conflict copy, download server version
//
// The returned strings are the paths affected by the resolution.
func ResolveConflict(conflict *ConflictInfo, strategy ConflictStrategy) (localPath string, serverPath string, action string) {
	switch strategy {
	case StrategyLocalWins:
		return conflict.Path, conflict.Path, "upload-local"

	case StrategyServerWins:
		return conflict.Path, conflict.Path, "download-server"

	case StrategyKeepBoth:
		return conflict.ConflictPath, conflict.Path, "keep-both"

	default:
		// Default to keep-both for safety — no data loss.
		return conflict.ConflictPath, conflict.Path, "keep-both"
	}
}

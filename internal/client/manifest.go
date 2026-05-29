package client

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

// ManifestBuilder scans a vault directory and computes SHA-256 hashes
// for all files, building a Manifest for sync exchange.
type ManifestBuilder struct {
	vaultPath      string
	excludedPaths  []string
	maxFileSize    int64
	osyncDirName   string
}

// ManifestBuilderConfig holds configuration for the manifest builder.
type ManifestBuilderConfig struct {
	VaultPath     string
	ExcludedPaths []string
	MaxFileSize   int64
}

// NewManifestBuilder creates a new ManifestBuilder with the given config.
func NewManifestBuilder(cfg ManifestBuilderConfig) *ManifestBuilder {
	maxSize := cfg.MaxFileSize
	if maxSize <= 0 {
		maxSize = protocol.MaxFileSize
	}

	excluded := cfg.ExcludedPaths
	if excluded == nil {
		excluded = []string{".obsidian"}
	}

	return &ManifestBuilder{
		vaultPath:     cfg.VaultPath,
		excludedPaths: excluded,
		maxFileSize:   maxSize,
		osyncDirName:  ".osync",
	}
}

// Build walks the vault directory and returns a Manifest with SHA-256 hashes
// for every eligible file. It excludes paths matching configured exclusions
// and the .osync/ directory. Files larger than maxFileSize are skipped.
func (mb *ManifestBuilder) Build() (*protocol.Manifest, error) {
	var entries []protocol.FileEntry

	err := filepath.Walk(mb.vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking vault: %w", err)
		}

		// Compute relative path from vault root.
		relPath, err := filepath.Rel(mb.vaultPath, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		// Normalize to forward slashes for cross-platform consistency.
		relPath = filepath.ToSlash(relPath)

		// Skip the vault root itself.
		if relPath == "." {
			return nil
		}

		// Always exclude .osync/ directory.
		if mb.isOSyncDir(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories — they are implied by their files.
		if info.IsDir() {
			// Check if this directory should be excluded.
			if mb.isExcluded(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check exclusion patterns for files.
		if mb.isExcluded(relPath) {
			return nil
		}

		// Skip files exceeding max size.
		if info.Size() > mb.maxFileSize {
			return nil
		}

		// Compute SHA-256 hash.
		hash, err := computeFileHash(path)
		if err != nil {
			return fmt.Errorf("hashing file %s: %w", relPath, err)
		}

		modTime := info.ModTime().UTC().Format("2006-01-02T15:04:05Z")

		entries = append(entries, protocol.FileEntry{
			Path:       relPath,
			Hash:       hash,
			Size:       info.Size(),
			ModifiedAt: modTime,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("building manifest: %w", err)
	}

	if entries == nil {
		entries = []protocol.FileEntry{}
	}

	return &protocol.Manifest{Entries: entries}, nil
}

// isOSyncDir returns true if the path is inside or is the .osync/ directory.
func (mb *ManifestBuilder) isOSyncDir(relPath string) bool {
	parts := strings.Split(relPath, "/")
	for _, part := range parts {
		if part == mb.osyncDirName {
			return true
		}
	}
	return false
}

// isExcluded returns true if the path matches any exclusion pattern.
// Exclusion matches against the first path component (directory name).
func (mb *ManifestBuilder) isExcluded(relPath string) bool {
	parts := strings.Split(relPath, "/")
	for _, excluded := range mb.excludedPaths {
		// Match against the first component (e.g., ".obsidian" in ".obsidian/config").
		for _, part := range parts {
			if part == excluded {
				return true
			}
		}
	}
	return false
}

// computeFileHash computes the SHA-256 hash of a file.
func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ComputeHash computes SHA-256 of the given data and returns the hex string.
// This is a convenience function for testing and direct use.
func ComputeHash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// ManifestPage represents a single page of manifest entries for paginated upload.
type ManifestPage struct {
	Entries  []protocol.FileEntry
	Page     int
	PageSize int
	Total    int
}

// BuildPaginated builds the manifest and splits it into pages of the given size.
// Returns a slice of ManifestPage that can be sent sequentially to the server.
// If pageSize <= 0, a single page containing all entries is returned.
func (mb *ManifestBuilder) BuildPaginated(pageSize int) ([]ManifestPage, error) {
	manifest, err := mb.Build()
	if err != nil {
		return nil, fmt.Errorf("building manifest: %w", err)
	}

	if len(manifest.Entries) == 0 {
		return []ManifestPage{{
			Entries:  []protocol.FileEntry{},
			Page:     1,
			PageSize: pageSize,
			Total:    0,
		}}, nil
	}

	// If pageSize <= 0 or total fits in one page, return a single page.
	if pageSize <= 0 || len(manifest.Entries) <= pageSize {
		return []ManifestPage{{
			Entries:  manifest.Entries,
			Page:     1,
			PageSize: pageSize,
			Total:    len(manifest.Entries),
		}}, nil
	}

	var pages []ManifestPage
	total := len(manifest.Entries)
	for i := 0; i < total; i += pageSize {
		end := i + pageSize
		if end > total {
			end = total
		}
		pages = append(pages, ManifestPage{
			Entries:  manifest.Entries[i:end],
			Page:     len(pages) + 1,
			PageSize: pageSize,
			Total:    total,
		})
	}
	return pages, nil
}

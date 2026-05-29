// Package protocol defines all request/response types and constants
// for the obsidian-sync protocol.
package protocol

import "time"

// SyncSession represents an active sync session between client and server.
type SyncSession struct {
	SessionID string    `json:"session_id"`
	VaultID   string    `json:"vault_id"`
	Timestamp time.Time `json:"timestamp"`
}

// FileEntry represents a single file in the vault manifest.
// It is also used as ManifestEntry via type alias for design compatibility.
type FileEntry struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

// ManifestEntry is an alias for FileEntry, matching the design's naming convention.
type ManifestEntry = FileEntry

// Manifest is the full list of files in a vault, used for initial sync and recovery.
type Manifest struct {
	Entries []FileEntry `json:"entries"`
}

// SyncBeginRequest starts a new incremental sync session.
type SyncBeginRequest struct {
	VaultID       string `json:"vault_id"`
	ClientID      string `json:"client_id"`
	SinceRevision int64  `json:"since_revision"` // 0 = initial sync
}

// SyncBeginResponse contains server changes since the client's last revision.
type SyncBeginResponse struct {
	ServerChanges []FileChange `json:"server_changes"`
	Revision      int64        `json:"revision"`
}

// FileChange represents a single file change communicated from the server.
type FileChange struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
	Operation  string `json:"operation"` // "create", "update", "delete"
}

// SyncCompleteRequest confirms the sync session is done.
type SyncCompleteRequest struct {
	Revision int64 `json:"revision"`
}

// ManifestResponse is the server response to a full manifest upload.
// It tells the client which files to upload, download, or resolve as conflicts.
type ManifestResponse struct {
	Need     []string `json:"need"`     // paths client must upload
	Have     []string `json:"have"`     // paths client must download
	Conflict []string `json:"conflict"` // paths with conflicts
}

// PaginatedManifestRequest wraps a manifest page for paginated uploads.
// When a vault has more entries than the pagination threshold, the client
// sends the manifest in multiple pages using this structure.
type PaginatedManifestRequest struct {
	Entries  []FileEntry `json:"entries"`
	Page     int         `json:"page"`      // 1-based page number
	PageSize int         `json:"page_size"` // number of entries per page
	Total    int         `json:"total"`     // total number of entries across all pages
	VaultID  string      `json:"vault_id"`
}

// PaginatedManifestResponse is the server response to a paginated manifest page.
// The final page (page == total_pages) returns the full ManifestResponse with
// need/have/conflict lists accumulated. Intermediate pages return partial results.
type PaginatedManifestResponse struct {
	Need     []string `json:"need"`               // accumulated paths client must upload
	Have     []string `json:"have"`               // accumulated paths client must download
	Conflict []string `json:"conflict"`           // accumulated paths with conflicts
	Page     int      `json:"page"`               // page number just processed
	LastPage bool     `json:"last_page"`          // true if this was the final page
}

// FileUploadRequest represents metadata for a file being uploaded to the server.
type FileUploadRequest struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// FileDownloadResponse contains file metadata and content for a download.
type FileDownloadResponse struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
	Content    []byte `json:"content"`
}

// ErrorResponse represents a standard API error response.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SyncRequest is an alias for SyncBeginRequest for convenience.
type SyncRequest = SyncBeginRequest

// SyncResponse is an alias for SyncBeginResponse for convenience.
type SyncResponse = SyncBeginResponse
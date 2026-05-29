package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/user/obsidian-sync-f2p/internal/config"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

// ProgressReporter is called during sync to report progress.
type ProgressReporter interface {
	Report(phase string, current, total int, message string)
}

// ProgressFunc is a convenience type to use a function as a ProgressReporter.
type ProgressFunc func(phase string, current, total int, message string)

func (f ProgressFunc) Report(phase string, current, total int, message string) {
	f(phase, current, total, message)
}

// Syncer orchestrates the full sync cycle: begin → manifest exchange →
// file transfer → complete.
type Syncer struct {
	cfg        *config.Config
	journal    *Journal
	httpClient *http.Client
	reporter   ProgressReporter
	clientID   string
}

// SyncerConfig holds the configuration for creating a Syncer.
type SyncerConfig struct {
	Config   *config.Config
	Journal  *Journal
	Reporter ProgressReporter
}

// NewSyncer creates a new Syncer with the given configuration.
func NewSyncer(cfg SyncerConfig) *Syncer {
	timeout := 30 * time.Second
	if cfg.Config != nil && cfg.Config.SessionTimeout > 0 {
		timeout = cfg.Config.SessionTimeout
	}

	reporter := cfg.Reporter
	if reporter == nil {
		reporter = ProgressFunc(func(phase string, current, total int, message string) {
			log.Printf("[sync] %s %d/%d: %s", phase, current, total, message)
		})
	}

	// Default client ID: use configured value, or hostname as fallback.
	var clientID string
	if cfg.Config != nil {
		clientID = cfg.Config.ClientID
	}
	if clientID == "" {
		hostname, _ := os.Hostname()
		clientID = hostname
		if clientID == "" {
			clientID = "osync-client"
		}
	}

	return &Syncer{
		cfg:     cfg.Config,
		journal: cfg.Journal,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		reporter: reporter,
		clientID: clientID,
	}
}

// SyncResult contains the results of a sync cycle.
type SyncResult struct {
	Uploaded   int
	Downloaded int
	Conflicts  int
	Revision   int64
}

// Sync executes a full sync cycle:
// 1. POST /api/v1/sync/begin (get session_id)
// 2. Build local manifest, POST /api/v1/sync/manifest (get server manifest + changes)
// 3. For each changed file on server: GET /api/v1/sync/file?path=..., save locally
// 4. For each changed file locally: POST /api/v1/sync/file (upload)
// 5. Handle conflicts: detect and create conflict copies
// 6. POST /api/v1/sync/complete (end session)
func (s *Syncer) Sync(ctx context.Context) (*SyncResult, error) {
	result := &SyncResult{}

	if s.cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if s.cfg.ServerURL == "" {
		return nil, fmt.Errorf("server_url is not configured; run 'osync config set server_url <url>'")
	}
	if s.cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is not configured; run 'osync config set api_key <key>'")
	}

	baseURL := s.cfg.ServerURL

	// Phase 1: Begin sync session.
	s.reporter.Report("begin", 0, 7, "starting sync session")
	beginResp, err := s.beginSync(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("begin sync: %w", err)
	}
	sessionID := beginResp.SessionID
	result.Revision = beginResp.Revision

	// Phase 2: Manifest exchange.
	s.reporter.Report("manifest", 1, 7, "exchanging manifests")
	manifestResp, err := s.exchangeManifest(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("manifest exchange: %w", err)
	}

	// Phase 3: Download server changes.
	s.reporter.Report("download", 2, 7, fmt.Sprintf("downloading %d files", len(beginResp.ServerChanges)+len(manifestResp.Have)))
	downloaded, err := s.downloadFiles(ctx, baseURL, beginResp.ServerChanges, manifestResp.Have)
	if err != nil {
		return nil, fmt.Errorf("downloading files: %w", err)
	}
	result.Downloaded = downloaded

	// Phase 4: Upload files that the server needs (from manifest exchange).
	s.reporter.Report("upload-need", 3, 7, fmt.Sprintf("uploading %d files server needs", len(manifestResp.Need)))
	needUploaded, err := s.uploadManifestNeed(ctx, baseURL, manifestResp.Need)
	if err != nil {
		return nil, fmt.Errorf("uploading needed files: %w", err)
	}

	// Phase 5: Handle conflicts.
	s.reporter.Report("conflict", 4, 7, fmt.Sprintf("resolving %d conflicts", len(manifestResp.Conflict)))
	conflicts, err := s.handleConflicts(ctx, baseURL, manifestResp.Conflict)
	if err != nil {
		return nil, fmt.Errorf("handling conflicts: %w", err)
	}
	result.Conflicts = conflicts

	// Phase 6: Upload local changes (from journal).
	s.reporter.Report("upload", 5, 7, "uploading local changes")
	uploaded, err := s.uploadLocalChanges(ctx, baseURL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("uploading files: %w", err)
	}
	result.Uploaded = needUploaded + uploaded

	// Phase 7: Complete sync session.
	s.reporter.Report("complete", 6, 7, "completing sync session")
	if err := s.completeSync(ctx, baseURL, result.Revision); err != nil {
		return nil, fmt.Errorf("complete sync: %w", err)
	}

	s.reporter.Report("done", 7, 7, "sync complete")

	// Record last sync timestamp in journal.
	if s.journal != nil {
		_ = s.journal.SetMetadata("last_sync", time.Now().UTC().Format(time.RFC3339))
	}

	return result, nil
}

// beginResponse wraps the begin response with session_id.
type beginResponse struct {
	protocol.SyncBeginResponse
	SessionID string `json:"session_id"`
}

// beginSync starts a new sync session.
func (s *Syncer) beginSync(ctx context.Context, baseURL string) (*beginResponse, error) {
	reqBody := protocol.SyncBeginRequest{
		VaultID:       s.cfg.VaultPath,
		ClientID:      s.clientID,
		SinceRevision: 0, // TODO: track last revision
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling begin request: %w", err)
	}

	url := baseURL + protocol.SyncBeginPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating begin request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("sending begin request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("begin returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var beginResp beginResponse
	if err := json.NewDecoder(resp.Body).Decode(&beginResp); err != nil {
		return nil, fmt.Errorf("decoding begin response: %w", err)
	}

	return &beginResp, nil
}

// exchangeManifest sends the local manifest and receives the server's response.
// If the manifest exceeds the pagination threshold, it sends entries in pages.
func (s *Syncer) exchangeManifest(ctx context.Context, baseURL string) (*protocol.ManifestResponse, error) {
	manifestBuilder := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath:     s.cfg.VaultPath,
		ExcludedPaths: s.cfg.ExcludedPaths,
		MaxFileSize:   s.cfg.MaxFileSize,
	})

	manifest, err := manifestBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("building manifest: %w", err)
	}

	// Determine page size from config, fall back to default.
	pageSize := s.cfg.PageSize
	if pageSize <= 0 {
		pageSize = protocol.DefaultPageSize
	}

	// If manifest exceeds the pagination threshold, send paginated.
	if len(manifest.Entries) > protocol.PaginationThreshold {
		return s.exchangeManifestPaginated(ctx, baseURL, manifestBuilder, pageSize)
	}

	// Single request for non-paginated manifest.
	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	url := baseURL + protocol.SyncManifestPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating manifest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("sending manifest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manifest returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var manifestResp protocol.ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifestResp); err != nil {
		return nil, fmt.Errorf("decoding manifest response: %w", err)
	}

	return &manifestResp, nil
}

// exchangeManifestPaginated sends the manifest in pages when entries exceed
// the pagination threshold. It accumulates need/have/conflict lists across pages.
func (s *Syncer) exchangeManifestPaginated(ctx context.Context, baseURL string, mb *ManifestBuilder, pageSize int) (*protocol.ManifestResponse, error) {
	pages, err := mb.BuildPaginated(pageSize)
	if err != nil {
		return nil, fmt.Errorf("building paginated manifest: %w", err)
	}

	var accumulated protocol.ManifestResponse
	accumulated.Need = []string{}
	accumulated.Have = []string{}
	accumulated.Conflict = []string{}

	for _, page := range pages {
		pageReq := protocol.PaginatedManifestRequest{
			Entries:  page.Entries,
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
			VaultID:  s.cfg.VaultID,
		}

		body, err := json.Marshal(pageReq)
		if err != nil {
			return nil, fmt.Errorf("marshaling manifest page %d: %w", page.Page, err)
		}

		url := baseURL + protocol.SyncManifestPath
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating manifest page %d request: %w", page.Page, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

		resp, err := s.doRequest(req)
		if err != nil {
			return nil, fmt.Errorf("sending manifest page %d: %w", page.Page, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("manifest page %d returned status %d: %s", page.Page, resp.StatusCode, string(respBody))
		}

		var pageResp protocol.PaginatedManifestResponse
		if err := json.NewDecoder(resp.Body).Decode(&pageResp); err != nil {
			return nil, fmt.Errorf("decoding manifest page %d response: %w", page.Page, err)
		}

		accumulated.Need = append(accumulated.Need, pageResp.Need...)
		accumulated.Have = append(accumulated.Have, pageResp.Have...)
		accumulated.Conflict = append(accumulated.Conflict, pageResp.Conflict...)
	}

	return &accumulated, nil
}

// downloadFiles downloads server changes and files the client needs.
func (s *Syncer) downloadFiles(ctx context.Context, baseURL string, serverChanges []protocol.FileChange, havePaths []string) (int, error) {
	downloaded := 0

	// Download server changes.
	for _, change := range serverChanges {
		if change.Operation == protocol.OperationDelete {
			// Delete the local file.
			localPath := filepath.Join(s.cfg.VaultPath, change.Path)
			if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
				log.Printf("warning: failed to delete %s: %v", change.Path, err)
			}
			downloaded++
			continue
		}

		if err := s.downloadFile(ctx, baseURL, change.Path); err != nil {
			return downloaded, fmt.Errorf("downloading %s: %w", change.Path, err)
		}
		downloaded++
	}

	// Download files the client doesn't have.
	for _, path := range havePaths {
		if err := s.downloadFile(ctx, baseURL, path); err != nil {
			return downloaded, fmt.Errorf("downloading %s: %w", path, err)
		}
		downloaded++
	}

	return downloaded, nil
}

// downloadFile downloads a single file from the server and saves it locally.
func (s *Syncer) downloadFile(ctx context.Context, baseURL, filePath string) error {
	url := baseURL + protocol.SyncFilePath + "?path=" + url.PathEscape(filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return fmt.Errorf("sending download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var fileResp protocol.FileDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return fmt.Errorf("decoding download response: %w", err)
	}

	// Save to local vault.
	localPath := filepath.Join(s.cfg.VaultPath, fileResp.Path)
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := os.WriteFile(localPath, fileResp.Content, 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", localPath, err)
	}

	return nil
}

// handleConflicts resolves conflicts by downloading server versions and
// creating conflict copies for local files.
func (s *Syncer) handleConflicts(ctx context.Context, baseURL string, conflictPaths []string) (int, error) {
	if len(conflictPaths) == 0 {
		return 0, nil
	}

	// Build a quick lookup of local manifest entries.
	manifestBuilder := NewManifestBuilder(ManifestBuilderConfig{
		VaultPath:     s.cfg.VaultPath,
		ExcludedPaths: s.cfg.ExcludedPaths,
		MaxFileSize:   s.cfg.MaxFileSize,
	})
	localManifest, err := manifestBuilder.Build()
	if err != nil {
		return 0, fmt.Errorf("building local manifest for conflict check: %w", err)
	}

	localMap := make(map[string]protocol.FileEntry, len(localManifest.Entries))
	for _, e := range localManifest.Entries {
		localMap[e.Path] = e
	}

	conflicts := 0
	for _, path := range conflictPaths {
		localEntry, ok := localMap[path]
		if !ok {
			continue
		}

		// Create conflict copy name.
		conflictPath := ConflictCopyName(path)
		localFilePath := filepath.Join(s.cfg.VaultPath, path)
		conflictFilePath := filepath.Join(s.cfg.VaultPath, conflictPath)

		// Read local content.
		localContent, err := os.ReadFile(localFilePath)
		if err != nil {
			log.Printf("warning: failed to read local file %s for conflict: %v", path, err)
			continue
		}

		// Save local version as conflict copy.
		if err := os.WriteFile(conflictFilePath, localContent, 0o644); err != nil {
			log.Printf("warning: failed to write conflict copy %s: %v", conflictPath, err)
			continue
		}

		// Download server version to replace the original.
		if err := s.downloadFile(ctx, baseURL, path); err != nil {
			log.Printf("warning: failed to download server version of %s: %v", path, err)
			// Restore local version.
			_ = os.Rename(conflictFilePath, localFilePath)
			continue
		}

		// Record in journal.
		if s.journal != nil {
			_ = s.journal.RecordChange(conflictPath, JournalOpCreate, ComputeHash(localContent), int64(len(localContent)))
		}

		_ = localEntry // used above
		conflicts++
	}

	return conflicts, nil
}

// uploadManifestNeed uploads files that the server identified as needed
// during manifest exchange. These are files the client has locally but
// the server doesn't.
func (s *Syncer) uploadManifestNeed(ctx context.Context, baseURL string, needPaths []string) (int, error) {
	if len(needPaths) == 0 {
		return 0, nil
	}

	uploaded := 0
	for _, path := range needPaths {
		localPath := filepath.Join(s.cfg.VaultPath, path)

		// Compute hash of the local file.
		hash, err := computeFileHash(localPath)
		if err != nil {
			log.Printf("warning: failed to hash %s for upload: %v", path, err)
			continue
		}

		if err := s.uploadFile(ctx, baseURL, path, hash, localPath, ""); err != nil {
			log.Printf("warning: failed to upload needed file %s: %v", path, err)
			continue
		}
		uploaded++
	}

	return uploaded, nil
}

// uploadLocalChanges uploads all pending changes from the journal.
func (s *Syncer) uploadLocalChanges(ctx context.Context, baseURL, sessionID string) (int, error) {
	if s.journal == nil {
		return 0, nil
	}

	changes, err := s.journal.GetPendingChanges()
	if err != nil {
		return 0, fmt.Errorf("getting pending changes: %w", err)
	}

	uploaded := 0
	for _, change := range changes {
		if change.Operation == JournalOpDelete {
			// For deletes, we still inform the server.
			// The server handles soft-delete.
			if err := s.uploadDelete(ctx, baseURL, change.Path, sessionID); err != nil {
				log.Printf("warning: failed to upload delete for %s: %v", change.Path, err)
				continue
			}
		} else {
			// Upload the file content.
			localPath := filepath.Join(s.cfg.VaultPath, change.Path)
			if err := s.uploadFile(ctx, baseURL, change.Path, change.Hash, localPath, sessionID); err != nil {
				log.Printf("warning: failed to upload %s: %v", change.Path, err)
				continue
			}
		}

		// Mark as synced.
		_ = s.journal.MarkSynced(change.Path)
		uploaded++
	}

	return uploaded, nil
}

// uploadFile uploads a single file to the server using multipart form data.
func (s *Syncer) uploadFile(ctx context.Context, baseURL, path, hash, localPath, sessionID string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", localPath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("path", path)
	_ = writer.WriteField("hash", hash)
	_ = writer.WriteField("client_id", s.clientID)

	part, err := writer.CreateFormFile("content", filepath.Base(path))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copying file content: %w", err)
	}

	writer.Close()

	url := baseURL + protocol.SyncFilePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return fmt.Errorf("sending upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// uploadDelete informs the server about a deleted file.
// This uses a simple POST with the delete operation marker.
func (s *Syncer) uploadDelete(ctx context.Context, baseURL, path, sessionID string) error {
	reqBody := map[string]string{
		"path":       path,
		"operation":  protocol.OperationDelete,
		"client_id":  s.clientID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling delete request: %w", err)
	}

	url := baseURL + protocol.SyncFilePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return fmt.Errorf("sending delete request: %w", err)
	}
	defer resp.Body.Close()

	// Server may return various status codes for deletes; 200 or 404 are acceptable.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// completeSync ends the sync session.
func (s *Syncer) completeSync(ctx context.Context, baseURL string, revision int64) error {
	reqBody := protocol.SyncCompleteRequest{
		Revision: revision,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling complete request: %w", err)
	}

	url := baseURL + protocol.SyncCompletePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating complete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.doRequest(req)
	if err != nil {
		return fmt.Errorf("sending complete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// doRequest executes an HTTP request with basic retry for transient failures.
func (s *Syncer) doRequest(req *http.Request) (*http.Response, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Retry on server errors (5xx).
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

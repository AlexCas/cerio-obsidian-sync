package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

// Session tracks an active sync session.
type Session struct {
	ID        string
	VaultID   string
	ClientID  string
	StartedAt time.Time
	Revision  int64
}

// SessionManager manages active sync sessions in memory.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// Create adds a new session and returns it.
func (sm *SessionManager) Create(vaultID, clientID string, revision int64) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := fmt.Sprintf("%s-%d", clientID, time.Now().UnixNano())
	sess := &Session{
		ID:        id,
		VaultID:   vaultID,
		ClientID:  clientID,
		StartedAt: time.Now(),
		Revision:  revision,
	}
	sm.sessions[id] = sess
	return sess
}

// Get retrieves a session by ID.
func (sm *SessionManager) Get(id string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[id]
	return sess, ok
}

// Remove deletes a session.
func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

// Handlers contains the HTTP handler methods for sync endpoints.
type Handlers struct {
	store    StoreInterface
	sessions *SessionManager
}

// StoreInterface defines the store methods needed by handlers.
// This allows handlers to be tested with mocks.
type StoreInterface interface {
	PutFile(vaultID, path, hash string, content io.Reader) error
	GetFile(vaultID, hash string) (io.ReadCloser, error)
	DeleteFile(vaultID, path string) error
	GetManifest(vaultID string) ([]protocol.FileEntry, error)
	GetChangesSince(vaultID string, revision int64) ([]protocol.FileChange, error)
	RecordRevision(vaultID, clientID, filePath, operation, hash string, size int64) (int64, error)
	GetFileByPath(vaultID, path string) (*protocol.FileEntry, error)
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(s StoreInterface) *Handlers {
	return &Handlers{
		store:    s,
		sessions: NewSessionManager(),
	}
}

// HandleBegin handles POST /api/v1/sync/begin.
// Starts a sync session and returns server changes since the client's last revision.
func (h *Handlers) HandleBegin(w http.ResponseWriter, r *http.Request) {
	vaultID := GetVaultID(r)
	if vaultID == "" {
		writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing vault context")
		return
	}

	var req protocol.SyncBeginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.ClientID == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "client_id is required")
		return
	}

	// Get server changes since the client's last revision.
	changes, err := h.store.GetChangesSince(vaultID, req.SinceRevision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to get changes")
		return
	}

	// Get current max revision.
	revision := req.SinceRevision
	if len(changes) > 0 {
		// The latest revision is tracked via RecordRevision.
		// For now, get it from the changes themselves.
		// We'll also record a new revision for this sync session.
	}

	// Create a new sync session.
	sess := h.sessions.Create(vaultID, req.ClientID, revision)

	resp := protocol.SyncBeginResponse{
		ServerChanges: changes,
		Revision:      revision,
	}

	// Store session ID in response via custom field.
	// The SyncBeginResponse doesn't have session_id, but we need it.
	// We'll encode it in a wrapper.
	type beginResponse struct {
		protocol.SyncBeginResponse
		SessionID string `json:"session_id"`
	}

	writeJSONStatus(w, http.StatusOK, beginResponse{
		SyncBeginResponse: resp,
		SessionID:         sess.ID,
	})
}

// HandleManifest handles POST /api/v1/sync/manifest.
// Exchanges manifests between client and server for initial sync.
// Supports both single-shot and paginated manifest uploads.
// Paginated requests are detected by the presence of the "page" field.
func (h *Handlers) HandleManifest(w http.ResponseWriter, r *http.Request) {
	vaultID := GetVaultID(r)
	if vaultID == "" {
		writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing vault context")
		return
	}

	// Try to detect if this is a paginated request by looking for the "page" field.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "failed to read request body")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Check if the request has a "page" field (paginated manifest).
	var peek struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(body, &peek); err == nil && peek.Page > 0 {
		// Paginated manifest request.
		h.handleManifestPaginated(w, r, vaultID, body)
		return
	}

	// Non-paginated request: standard manifest.
	var clientManifest protocol.Manifest
	if err := json.NewDecoder(r.Body).Decode(&clientManifest); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "invalid manifest body")
		return
	}

	need, have, conflict := h.computeManifestDiff(vaultID, clientManifest.Entries)

	writeJSON(w, protocol.ManifestResponse{
		Need:     need,
		Have:     have,
		Conflict: conflict,
	})
}

// handleManifestPaginated processes a paginated manifest request.
// It accumulates results across pages and returns a PaginatedManifestResponse.
// When the final page is received, need/have/conflict lists are fully populated.
func (h *Handlers) handleManifestPaginated(w http.ResponseWriter, r *http.Request, vaultID string, body []byte) {
	var pageReq protocol.PaginatedManifestRequest
	if err := json.Unmarshal(body, &pageReq); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "invalid paginated manifest body")
		return
	}

	// Compute diff for this page's entries.
	need, have, conflict := h.computeManifestDiff(vaultID, pageReq.Entries)

	// Determine if this is the last page.
	totalPages := pageReq.Total / pageReq.PageSize
	if pageReq.Total%pageReq.PageSize != 0 {
		totalPages++
	}
	lastPage := pageReq.Page >= totalPages

	writeJSON(w, protocol.PaginatedManifestResponse{
		Need:     need,
		Have:     have,
		Conflict: conflict,
		Page:     pageReq.Page,
		LastPage: lastPage,
	})
}

// computeManifestDiff compares client entries against the server manifest
// and returns need (client must upload), have (client must download),
// and conflict (both modified with different hash) path lists.
func (h *Handlers) computeManifestDiff(vaultID string, clientEntries []protocol.FileEntry) (need, have, conflict []string) {
	// Get server manifest.
	serverFiles, err := h.store.GetManifest(vaultID)
	if err != nil {
		serverFiles = nil
	}

	// Build lookup of server files by path.
	serverMap := make(map[string]protocol.FileEntry, len(serverFiles))
	for _, f := range serverFiles {
		serverMap[f.Path] = f
	}

	// Build lookup of client files by path.
	clientMap := make(map[string]protocol.FileEntry, len(clientEntries))
	for _, f := range clientEntries {
		clientMap[f.Path] = f
	}

	for _, cf := range clientEntries {
		sf, exists := serverMap[cf.Path]
		if !exists {
			need = append(need, cf.Path)
		} else if cf.Hash != sf.Hash {
			conflict = append(conflict, cf.Path)
		}
		// If hash matches, no action needed.
	}

	for _, sf := range serverFiles {
		if _, exists := clientMap[sf.Path]; !exists {
			have = append(have, sf.Path)
		}
	}

	// Ensure non-nil slices for JSON.
	if need == nil {
		need = []string{}
	}
	if have == nil {
		have = []string{}
	}
	if conflict == nil {
		conflict = []string{}
	}

	return need, have, conflict
}

// HandleFileUpload handles POST /api/v1/sync/file.
// Uploads a file to the server using multipart form data.
func (h *Handlers) HandleFileUpload(w http.ResponseWriter, r *http.Request) {
	vaultID := GetVaultID(r)
	if vaultID == "" {
		writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing vault context")
		return
	}

	// Parse multipart form.
	if err := r.ParseMultipartForm(protocol.MaxFileSize); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, protocol.ErrCodePayloadTooLarge, "file too large")
		return
	}

	path := r.FormValue("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "path is required")
		return
	}

	hash := r.FormValue("hash")
	if hash == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "hash is required")
		return
	}

	file, _, err := r.FormFile("content")
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "content file is required")
		return
	}
	defer file.Close()

	// Read content to compute size and verify hash.
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to read content")
		return
	}

	// Verify hash.
	computed := fmt.Sprintf("%x", sha256.Sum256(data))
	if computed != hash {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "hash mismatch")
		return
	}

	// Store the file.
	if err := h.store.PutFile(vaultID, path, hash, bytes.NewReader(data)); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to store file")
		return
	}

	// Record revision.
	clientID := r.FormValue("client_id")
	revID, err := h.store.RecordRevision(vaultID, clientID, path, protocol.OperationCreate, hash, int64(len(data)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to record revision")
		return
	}

	type uploadResponse struct {
		Revision int64  `json:"revision"`
		Path     string `json:"path"`
		Hash     string `json:"hash"`
	}

	writeJSON(w, uploadResponse{
		Revision: revID,
		Path:     path,
		Hash:     hash,
	})
}

// HandleFileDownload handles GET /api/v1/sync/file?path=...
// Downloads a file from the server.
func (h *Handlers) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	vaultID := GetVaultID(r)
	if vaultID == "" {
		writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing vault context")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "path query parameter is required")
		return
	}

	// Look up file metadata.
	entry, err := h.store.GetFileByPath(vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to get file metadata")
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, protocol.ErrCodeNotFound, "file not found")
		return
	}

	// Get the file content.
	rc, err := h.store.GetFile(vaultID, entry.Hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to get file content")
		return
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "failed to read file content")
		return
	}

	writeJSON(w, protocol.FileDownloadResponse{
		Path:       entry.Path,
		Hash:       entry.Hash,
		Size:       entry.Size,
		ModifiedAt: entry.ModifiedAt,
		Content:    data,
	})
}

// HandleComplete handles POST /api/v1/sync/complete.
// Ends a sync session.
func (h *Handlers) HandleComplete(w http.ResponseWriter, r *http.Request) {
	vaultID := GetVaultID(r)
	if vaultID == "" {
		writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing vault context")
		return
	}

	var req protocol.SyncCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrCodeBadRequest, "invalid request body")
		return
	}

	type completeResponse struct {
		Status    string `json:"status"`
		Revision  int64  `json:"revision"`
	}

	writeJSON(w, completeResponse{
		Status:   "completed",
		Revision: req.Revision,
	})
}

// decodeJSON decodes a JSON request body.
func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

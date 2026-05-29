package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/protocol"
)

// mockStore implements StoreInterface for testing handlers.
type mockStore struct {
	files    map[string]map[string]*protocol.FileEntry // vaultID -> path -> entry
	contents map[string][]byte                         // hash -> content
	changes  []protocol.FileChange
	nextRev  int64
}

func newMockStore() *mockStore {
	return &mockStore{
		files:    make(map[string]map[string]*protocol.FileEntry),
		contents: make(map[string][]byte),
		nextRev:  1,
	}
}

func (m *mockStore) PutFile(vaultID, path, hash string, content io.Reader) error {
	data, _ := io.ReadAll(content)
	if m.files[vaultID] == nil {
		m.files[vaultID] = make(map[string]*protocol.FileEntry)
	}
	m.files[vaultID][path] = &protocol.FileEntry{
		Path:       path,
		Hash:       hash,
		Size:       int64(len(data)),
		ModifiedAt: "2026-01-01T00:00:00Z",
	}
	m.contents[hash] = data
	return nil
}

func (m *mockStore) GetFile(vaultID, hash string) (io.ReadCloser, error) {
	data, ok := m.contents[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockStore) DeleteFile(vaultID, path string) error {
	if m.files[vaultID] != nil {
		delete(m.files[vaultID], path)
	}
	return nil
}

func (m *mockStore) GetManifest(vaultID string) ([]protocol.FileEntry, error) {
	var entries []protocol.FileEntry
	for _, e := range m.files[vaultID] {
		entries = append(entries, *e)
	}
	return entries, nil
}

func (m *mockStore) GetChangesSince(vaultID string, revision int64) ([]protocol.FileChange, error) {
	return m.changes, nil
}

func (m *mockStore) RecordRevision(vaultID, clientID, filePath, operation, hash string, size int64) (int64, error) {
	rev := m.nextRev
	m.nextRev++
	return rev, nil
}

func (m *mockStore) GetFileByPath(vaultID, path string) (*protocol.FileEntry, error) {
	if m.files[vaultID] == nil {
		return nil, nil
	}
	return m.files[vaultID][path], nil
}

// Helper to create an authenticated request with vault_id in context.
func authRequest(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), vaultIDKey, "v1")
	return req.WithContext(ctx)
}

// newMultipartWriter creates a multipart writer.
func newMultipartWriter(buf *bytes.Buffer) *multipart.Writer {
	return multipart.NewWriter(buf)
}

func TestHandleBegin_CreatesSession(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	body := `{"vault_id":"v1","client_id":"c1","since_revision":0}`
	req := authRequest("POST", "/api/v1/sync/begin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleBegin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SessionID      string                   `json:"session_id"`
		ServerChanges []protocol.FileChange    `json:"server_changes"`
		Revision       int64                    `json:"revision"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("expected session_id in response")
	}
}

func TestHandleBegin_MissingClientID(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	body := `{"vault_id":"v1","since_revision":0}`
	req := authRequest("POST", "/api/v1/sync/begin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleBegin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleBegin_ReturnsServerChanges(t *testing.T) {
	ms := newMockStore()
	ms.changes = []protocol.FileChange{
		{Path: "a.md", Hash: "hash1", Operation: protocol.OperationCreate},
		{Path: "b.md", Hash: "hash2", Operation: protocol.OperationUpdate},
	}
	h := NewHandlers(ms)

	body := `{"vault_id":"v1","client_id":"c1","since_revision":0}`
	req := authRequest("POST", "/api/v1/sync/begin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleBegin(rec, req)

	var resp struct {
		ServerChanges []protocol.FileChange `json:"server_changes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(resp.ServerChanges) != 2 {
		t.Errorf("expected 2 server changes, got %d", len(resp.ServerChanges))
	}
}

func TestHandleManifest_Exchange(t *testing.T) {
	ms := newMockStore()
	// Pre-populate server with one file.
	ms.files["v1"] = map[string]*protocol.FileEntry{
		"server-only.md": {Path: "server-only.md", Hash: "shash", Size: 10, ModifiedAt: "2026-01-01"},
		"same.md":        {Path: "same.md", Hash: "samehash", Size: 5, ModifiedAt: "2026-01-01"},
	}
	h := NewHandlers(ms)

	// Client sends manifest with: client-only.md (new), same.md (same hash).
	body := `{"entries":[
		{"path":"client-only.md","hash":"chash","size":20,"modified_at":"2026-01-01"},
		{"path":"same.md","hash":"samehash","size":5,"modified_at":"2026-01-01"}
	]}`
	req := authRequest("POST", "/api/v1/sync/manifest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleManifest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp protocol.ManifestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	// client-only.md → need (client must upload)
	found := false
	for _, p := range resp.Need {
		if p == "client-only.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected client-only.md in Need")
	}

	// server-only.md → have (client must download)
	found = false
	for _, p := range resp.Have {
		if p == "server-only.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected server-only.md in Have")
	}
}

func TestHandleManifest_ConflictDetection(t *testing.T) {
	ms := newMockStore()
	ms.files["v1"] = map[string]*protocol.FileEntry{
		"conflict.md": {Path: "conflict.md", Hash: "serverhash", Size: 10, ModifiedAt: "2026-01-01"},
	}
	h := NewHandlers(ms)

	body := `{"entries":[
		{"path":"conflict.md","hash":"clienthash","size":15,"modified_at":"2026-01-01"}
	]}`
	req := authRequest("POST", "/api/v1/sync/manifest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleManifest(rec, req)

	var resp protocol.ManifestResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	found := false
	for _, p := range resp.Conflict {
		if p == "conflict.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected conflict.md in Conflict")
	}
}

func TestHandleFileUpload_RoundTrip(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	content := []byte("test file content")
	hash := fmt.Sprintf("%x", sha256.Sum256(content))

	// Build multipart form.
	var buf bytes.Buffer
	writer := newMultipartWriter(&buf)
	_ = writer.WriteField("path", "notes/upload.md")
	_ = writer.WriteField("hash", hash)
	_ = writer.WriteField("client_id", "c1")
	part, _ := writer.CreateFormFile("content", "upload.md")
	part.Write(content)
	writer.Close()

	req := authRequest("POST", "/api/v1/sync/file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	h.HandleFileUpload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify file was stored.
	entry, _ := ms.GetFileByPath("v1", "notes/upload.md")
	if entry == nil {
		t.Fatal("file not stored after upload")
	}
	if entry.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, entry.Hash)
	}
}

func TestHandleFileUpload_MissingPath(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	var buf bytes.Buffer
	writer := newMultipartWriter(&buf)
	_ = writer.WriteField("hash", "abc")
	writer.Close()

	req := authRequest("POST", "/api/v1/sync/file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	h.HandleFileUpload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleFileDownload_Success(t *testing.T) {
	ms := newMockStore()
	content := []byte("download me")
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	ms.files["v1"] = map[string]*protocol.FileEntry{
		"notes/download.md": {Path: "notes/download.md", Hash: hash, Size: int64(len(content)), ModifiedAt: "2026-01-01"},
	}
	ms.contents[hash] = content
	h := NewHandlers(ms)

	req := authRequest("GET", "/api/v1/sync/file?path=notes/download.md", nil)
	rec := httptest.NewRecorder()

	h.HandleFileDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp protocol.FileDownloadResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if resp.Path != "notes/download.md" {
		t.Errorf("expected path notes/download.md, got %s", resp.Path)
	}
	if !bytes.Equal(resp.Content, content) {
		t.Errorf("content mismatch")
	}
}

func TestHandleFileDownload_NotFound(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	req := authRequest("GET", "/api/v1/sync/file?path=nonexistent.md", nil)
	rec := httptest.NewRecorder()

	h.HandleFileDownload(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleFileDownload_MissingPath(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	req := authRequest("GET", "/api/v1/sync/file", nil)
	rec := httptest.NewRecorder()

	h.HandleFileDownload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleComplete_EndsSession(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	body := `{"revision":42}`
	req := authRequest("POST", "/api/v1/sync/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleComplete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status   string `json:"status"`
		Revision int64  `json:"revision"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Status)
	}
	if resp.Revision != 42 {
		t.Errorf("expected revision 42, got %d", resp.Revision)
	}
}

func TestHandleManifest_PaginatedRequest(t *testing.T) {
	ms := newMockStore()
	// Pre-populate server with files.
	ms.files["v1"] = map[string]*protocol.FileEntry{
		"server-only.md": {Path: "server-only.md", Hash: "shash", Size: 10, ModifiedAt: "2026-01-01"},
		"same.md":        {Path: "same.md", Hash: "samehash", Size: 5, ModifiedAt: "2026-01-01"},
	}
	h := NewHandlers(ms)

	// Send a paginated manifest request with page=1 (triggers paginated handler).
	body := `{
		"entries": [
			{"path":"client-only.md","hash":"chash","size":20,"modified_at":"2026-01-01"},
			{"path":"same.md","hash":"samehash","size":5,"modified_at":"2026-01-01"}
		],
		"page": 1,
		"page_size": 2,
		"total": 2,
		"vault_id": "v1"
	}`
	req := authRequest("POST", "/api/v1/sync/manifest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleManifest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp protocol.PaginatedManifestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	// client-only.md → need
	found := false
	for _, p := range resp.Need {
		if p == "client-only.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected client-only.md in Need")
	}

	// server-only.md → have
	found = false
	for _, p := range resp.Have {
		if p == "server-only.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected server-only.md in Have")
	}

	// Should be last page since 2 entries fit in 1 page with page_size 2.
	if !resp.LastPage {
		t.Error("expected last_page=true for single page")
	}
	if resp.Page != 1 {
		t.Errorf("expected page 1, got %d", resp.Page)
	}
}

func TestHandleManifest_NonPaginatedNoPage(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	// A manifest without "page" field should use the non-paginated path.
	body := `{"entries":[]}`
	req := authRequest("POST", "/api/v1/sync/manifest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleManifest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should return a ManifestResponse (not PaginatedManifestResponse).
	var resp protocol.ManifestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}
}

func TestHandleBegin_InvalidJSON(t *testing.T) {
	ms := newMockStore()
	h := NewHandlers(ms)

	req := authRequest("POST", "/api/v1/sync/begin", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleBegin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

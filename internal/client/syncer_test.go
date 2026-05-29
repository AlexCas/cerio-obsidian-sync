package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/config"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

// setupTestServer creates an httptest.Server with a real store backing.
func setupTestServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	dataDir := filepath.Join(dir, "data")

	s := store.New(dataDir)
	if err := s.Open(dbPath); err != nil {
		t.Fatalf("opening store: %v", err)
	}

	// Create a test API key.
	keyHash := sha256.Sum256([]byte("osync_testkey12345678901234567890123456789012345678901234567890"))
	keyHashStr := hex.EncodeToString(keyHash[:])
	if err := s.CreateAPIKey(keyHashStr, "test-vault", "test-key"); err != nil {
		t.Fatalf("creating API key: %v", err)
	}

	// Import server package to create a real server.
	// We'll build a simple handler manually to avoid import cycles.
	mux := http.NewServeMux()

	// Simple handlers that use the store.
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/v1/sync/begin", func(w http.ResponseWriter, r *http.Request) {
		var req protocol.SyncBeginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		changes, _ := s.GetChangesSince("test-vault", req.SinceRevision)

		resp := struct {
			protocol.SyncBeginResponse
			SessionID string `json:"session_id"`
		}{
			SyncBeginResponse: protocol.SyncBeginResponse{
				ServerChanges: changes,
				Revision:      1,
			},
			SessionID: "test-session-1",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/sync/manifest", func(w http.ResponseWriter, r *http.Request) {
		var manifest protocol.Manifest
		if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		serverFiles, _ := s.GetManifest("test-vault")
		serverMap := make(map[string]protocol.FileEntry)
		for _, f := range serverFiles {
			serverMap[f.Path] = f
		}

		clientMap := make(map[string]protocol.FileEntry)
		for _, f := range manifest.Entries {
			clientMap[f.Path] = f
		}

		var need, have, conflict []string
		for _, cf := range manifest.Entries {
			sf, exists := serverMap[cf.Path]
			if !exists {
				need = append(need, cf.Path)
			} else if cf.Hash != sf.Hash {
				conflict = append(conflict, cf.Path)
			}
		}
		for _, sf := range serverFiles {
			if _, exists := clientMap[sf.Path]; !exists {
				have = append(have, sf.Path)
			}
		}

		if need == nil {
			need = []string{}
		}
		if have == nil {
			have = []string{}
		}
		if conflict == nil {
			conflict = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(protocol.ManifestResponse{
			Need:     need,
			Have:     have,
			Conflict: conflict,
		})
	})

	mux.HandleFunc("/api/v1/sync/file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			path := r.URL.Query().Get("path")
			entry, _ := s.GetFileByPath("test-vault", path)
			if entry == nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			rc, err := s.GetFile("test-vault", entry.Hash)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			defer rc.Close()
			data, _ := io.ReadAll(rc)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(protocol.FileDownloadResponse{
				Path:       entry.Path,
				Hash:       entry.Hash,
				Size:       entry.Size,
				ModifiedAt: entry.ModifiedAt,
				Content:    data,
			})
			return
		}

		// POST: file upload.
		if err := r.ParseMultipartForm(protocol.MaxFileSize); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}

		path := r.FormValue("path")
		hash := r.FormValue("hash")
		file, _, err := r.FormFile("content")
		if err != nil {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, _ := io.ReadAll(file)
		computed := fmt.Sprintf("%x", sha256.Sum256(data))
		if computed != hash {
			http.Error(w, "hash mismatch", http.StatusBadRequest)
			return
		}

		_ = s.PutFile("test-vault", path, hash, bytes.NewReader(data))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"revision": 1,
			"path":     path,
			"hash":     hash,
		})
	})

	mux.HandleFunc("/api/v1/sync/complete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "completed",
			"revision": 1,
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		s.Close()
	})

	return server, s, "osync_testkey12345678901234567890123456789012345678901234567890"
}

func TestSyncer_SyncCycle(t *testing.T) {
	ts, _, apiKey := setupTestServer(t)

	vaultDir := t.TempDir()
	osyncDir := filepath.Join(vaultDir, ".osync")
	if err := os.MkdirAll(osyncDir, 0o755); err != nil {
		t.Fatalf("creating .osync dir: %v", err)
	}

	// Create test files in vault.
	if err := os.WriteFile(filepath.Join(vaultDir, "note1.md"), []byte("hello sync"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultDir, "note2.md"), []byte("another note"), 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	// Create journal.
	j := NewJournal(filepath.Join(osyncDir, "journal.db"))
	if err := j.Open(); err != nil {
		t.Fatalf("opening journal: %v", err)
	}
	defer j.Close()

	// Record a pending change.
	if err := j.RecordChange("note1.md", JournalOpCreate, ComputeHash([]byte("hello sync")), 10); err != nil {
		t.Fatalf("recording change: %v", err)
	}

	cfg := &config.Config{
		ServerURL:     ts.URL,
		APIKey:        apiKey,
		VaultPath:     vaultDir,
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	var reports []string
	reporter := ProgressFunc(func(phase string, current, total int, message string) {
		reports = append(reports, phase)
	})

	syncer := NewSyncer(SyncerConfig{
		Config:   cfg,
		Journal:  j,
		Reporter: reporter,
	})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if result.Uploaded < 1 {
		t.Errorf("expected at least 1 upload, got %d", result.Uploaded)
	}

	// Verify progress was reported.
	if len(reports) == 0 {
		t.Error("expected progress reports")
	}
}

func TestSyncer_NoServerURL(t *testing.T) {
	cfg := &config.Config{
		ServerURL: "",
		APIKey:    "osync_testkey",
		VaultPath: t.TempDir(),
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	_, err := syncer.Sync(context.Background())
	if err == nil {
		t.Error("expected error when server_url is not configured")
	}
}

func TestSyncer_NoAPIKey(t *testing.T) {
	cfg := &config.Config{
		ServerURL: "http://localhost:8080",
		APIKey:    "",
		VaultPath: t.TempDir(),
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	_, err := syncer.Sync(context.Background())
	if err == nil {
		t.Error("expected error when api_key is not configured")
	}
}

func TestSyncer_NilConfig(t *testing.T) {
	syncer := NewSyncer(SyncerConfig{})

	_, err := syncer.Sync(context.Background())
	if err == nil {
		t.Error("expected error when config is nil")
	}
}

func TestSyncer_DownloadFiles(t *testing.T) {
	ts, s, apiKey := setupTestServer(t)

	// Put a file on the server.
	content := []byte("server content")
	hash := ComputeHash(content)
	_ = s.PutFile("test-vault", "download-test.md", hash, bytes.NewReader(content))

	vaultDir := t.TempDir()
	cfg := &config.Config{
		ServerURL:     ts.URL,
		APIKey:        apiKey,
		VaultPath:     vaultDir,
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	// Download the file.
	err := syncer.downloadFile(context.Background(), ts.URL, "download-test.md")
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	// Verify the file was saved.
	localPath := filepath.Join(vaultDir, "download-test.md")
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "server content" {
		t.Errorf("downloaded content = %q, want %q", string(data), "server content")
	}
}

func TestSyncer_DownloadFilesWithSubdirs(t *testing.T) {
	ts, s, apiKey := setupTestServer(t)

	// Put a file in a subdirectory on the server.
	content := []byte("deep content")
	hash := ComputeHash(content)
	_ = s.PutFile("test-vault", "notes/deep/file.md", hash, bytes.NewReader(content))

	vaultDir := t.TempDir()
	cfg := &config.Config{
		ServerURL:     ts.URL,
		APIKey:        apiKey,
		VaultPath:     vaultDir,
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	err := syncer.downloadFile(context.Background(), ts.URL, "notes/deep/file.md")
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	localPath := filepath.Join(vaultDir, "notes", "deep", "file.md")
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "deep content" {
		t.Errorf("downloaded content = %q, want %q", string(data), "deep content")
	}
}

func TestSyncer_UploadFile(t *testing.T) {
	ts, s, apiKey := setupTestServer(t)

	vaultDir := t.TempDir()
	content := []byte("upload content")
	localPath := filepath.Join(vaultDir, "upload-test.md")
	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	cfg := &config.Config{
		ServerURL:     ts.URL,
		APIKey:        apiKey,
		VaultPath:     vaultDir,
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})
	hash := ComputeHash(content)

	err := syncer.uploadFile(context.Background(), ts.URL, "upload-test.md", hash, localPath, "session-1")
	if err != nil {
		t.Fatalf("uploadFile: %v", err)
	}

	// Verify file was stored on server.
	entry, err := s.GetFileByPath("test-vault", "upload-test.md")
	if err != nil {
		t.Fatalf("GetFileByPath: %v", err)
	}
	if entry == nil {
		t.Fatal("expected file to be stored on server")
	}
	if entry.Hash != hash {
		t.Errorf("server hash = %q, want %q", entry.Hash, hash)
	}
}

func TestSyncer_CompleteSync(t *testing.T) {
	ts, _, apiKey := setupTestServer(t)

	cfg := &config.Config{
		ServerURL:     ts.URL,
		APIKey:        apiKey,
		VaultPath:     t.TempDir(),
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	err := syncer.completeSync(context.Background(), ts.URL, 1)
	if err != nil {
		t.Fatalf("completeSync: %v", err)
	}
}

func TestSyncer_NetworkError(t *testing.T) {
	vaultDir := t.TempDir()
	cfg := &config.Config{
		ServerURL:     "http://127.0.0.1:0", // unreachable port
		APIKey:        "osync_testkey12345678901234567890123456789012345678901234567890",
		VaultPath:     vaultDir,
		ExcludedPaths: []string{".obsidian"},
		MaxFileSize:   protocol.MaxFileSize,
	}

	syncer := NewSyncer(SyncerConfig{Config: cfg})

	_, err := syncer.Sync(context.Background())
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

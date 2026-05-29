package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/auth"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

func openTestStoreForServer(t *testing.T) *store.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dataDir := filepath.Join(tmpDir, "data")

	s := store.New(dataDir)
	if err := s.Open(dbPath); err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := openTestStoreForServer(t)
	srv := NewServer(ServerConfig{
		Addr:   ":0",
		Store:  s,
		Logger: nil,
	})

	// Generate a valid API key for testing.
	key, _ := auth.GenerateKey()
	keyHash := sha256.Sum256([]byte(key))
	keyHashStr := hex.EncodeToString(keyHash[:])
	_ = s.CreateAPIKey(keyHashStr, "test-vault", "test-key")

	return srv, key
}

func TestNewServer_CreatesRouter(t *testing.T) {
	srv, _ := newTestServer(t)
	if srv == nil {
		t.Fatal("expected server, got nil")
	}
	if srv.router == nil {
		t.Fatal("expected router, got nil")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", protocol.HealthPath, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestServer_SyncRoutesRequireAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", protocol.SyncBeginPath},
		{"POST", protocol.SyncManifestPath},
		{"POST", protocol.SyncFilePath},
		{"GET", protocol.SyncFilePath},
		{"POST", protocol.SyncCompletePath},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without auth for %s %s, got %d", ep.method, ep.path, rec.Code)
			}
		})
	}
}

func TestServer_MiddlewareChainApplies(t *testing.T) {
	srv, key := newTestServer(t)

	// Test that with a valid key, the auth middleware passes.
	req := httptest.NewRequest("POST", protocol.SyncBeginPath, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// Should not be 401 (auth passed), should be 400 (bad JSON from empty body)
	if rec.Code == http.StatusUnauthorized {
		t.Errorf("auth middleware should have passed with valid key, got 401")
	}
}

func TestServer_BeginEndpointWithAuth(t *testing.T) {
	srv, key := newTestServer(t)

	req := httptest.NewRequest("POST", protocol.SyncBeginPath, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// Should not be 401
	if rec.Code == http.StatusUnauthorized {
		t.Error("should pass auth with valid key")
	}
}

func TestFormatAddr(t *testing.T) {
	addr := FormatAddr("localhost", 8080)
	if addr != "localhost:8080" {
		t.Errorf("expected localhost:8080, got %s", addr)
	}
}

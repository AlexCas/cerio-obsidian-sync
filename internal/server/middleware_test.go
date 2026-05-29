package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/obsidian-sync-f2p/internal/auth"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

func openTestStoreForMiddleware(t *testing.T) (*store.Store, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dataDir := filepath.Join(tmpDir, "data")

	s := store.New(dataDir)
	if err := s.Open(dbPath); err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, tmpDir
}

func createTestAPIKey(s *store.Store, vaultID, name string) string {
	key, _ := auth.GenerateKey()
	keyHash := sha256.Sum256([]byte(key))
	keyHashStr := hex.EncodeToString(keyHash[:])
	_ = s.CreateAPIKey(keyHashStr, vaultID, name)
	return key
}

func TestAuthValidation_RejectsMissingToken(t *testing.T) {
	s, _ := openTestStoreForMiddleware(t)

	mw := AuthValidation(s)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth")
	}))

	req := httptest.NewRequest("POST", "/api/v1/sync/begin", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthValidation_RejectsInvalidFormat(t *testing.T) {
	s, _ := openTestStoreForMiddleware(t)

	mw := AuthValidation(s)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with bad token")
	}))

	req := httptest.NewRequest("POST", "/api/v1/sync/begin", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthValidation_RejectsUnknownKey(t *testing.T) {
	s, _ := openTestStoreForMiddleware(t)

	mw := AuthValidation(s)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with unknown key")
	}))

	// Generate a valid-format key but don't store it.
	key, _ := auth.GenerateKey()

	req := httptest.NewRequest("POST", "/api/v1/sync/begin", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthValidation_AcceptsValidKey(t *testing.T) {
	s, _ := openTestStoreForMiddleware(t)
	vaultID := "test-vault"
	key := createTestAPIKey(s, vaultID, "test")

	mw := AuthValidation(s)
	var gotVaultID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVaultID = GetVaultID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/sync/begin", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotVaultID != vaultID {
		t.Errorf("expected vaultID %q, got %q", vaultID, gotVaultID)
	}
}

func TestSizeLimit_RejectsLargeBody(t *testing.T) {
	mw := SizeLimit(100) // 100 byte limit
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for oversized body")
	}))

	body := strings.Repeat("x", 101)
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(body))
	req.ContentLength = 101
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rec.Code)
	}
}

func TestSizeLimit_AllowsSmallBody(t *testing.T) {
	mw := SizeLimit(100)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader("small")
	req := httptest.NewRequest("POST", "/upload", body)
	req.ContentLength = 5
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called for small body")
	}
}

func TestRequestLogger_LogsRequest(t *testing.T) {
	var output string
	logger := log.New(io.Discard, "", 0)
	// We just test that the middleware doesn't break the handler.
	mw := RequestLogger(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	_ = output
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, protocol.ErrCodeBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if errResp.Code != protocol.ErrCodeBadRequest {
		t.Errorf("expected code %q, got %q", protocol.ErrCodeBadRequest, errResp.Code)
	}
	if errResp.Message != "test error" {
		t.Errorf("expected message %q, got %q", "test error", errResp.Message)
	}
}

func TestGetVaultID_Missing(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	vaultID := GetVaultID(req)
	if vaultID != "" {
		t.Errorf("expected empty vaultID, got %q", vaultID)
	}
}

func TestAuthValidation_MalformedBearerHeader(t *testing.T) {
	s, _ := openTestStoreForMiddleware(t)

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "osync_something"},
		{"wrong scheme", "Basic osync_something"},
		{"empty after bearer", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := AuthValidation(s)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("handler should not be called")
			}))

			req := httptest.NewRequest("POST", "/api/v1/sync/begin", nil)
			req.Header.Set("Authorization", tt.header)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", rec.Code)
			}
		})
	}
}

// Verify the error response format for size limit.
func TestSizeLimit_ResponseBody(t *testing.T) {
	mw := SizeLimit(10)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach handler")
	}))

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(fmt.Sprintf("%011d", 0)))
	req.ContentLength = 11
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if errResp.Code != protocol.ErrCodePayloadTooLarge {
		t.Errorf("expected code %q, got %q", protocol.ErrCodePayloadTooLarge, errResp.Code)
	}
}

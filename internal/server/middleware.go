// Package server provides the HTTP server, middleware, and sync endpoint handlers
// for the obsidian-sync server.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/user/obsidian-sync-f2p/internal/auth"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

// contextKey is used for storing values in request context.
type contextKey string

const (
	// vaultIDKey stores the authenticated vault_id in the request context.
	vaultIDKey contextKey = "vault_id"
)

// AuthValidation returns middleware that validates the Bearer token from the
// Authorization header against the store's api_keys table. On success, it
// sets the vault_id in the request context.
func AuthValidation(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			token, ok := auth.ExtractBearerToken(authHeader)
			if !ok {
				writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "missing or invalid authorization header")
				return
			}

			if err := auth.ValidateKey(token); err != nil {
				writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "invalid API key format")
				return
			}

			// Hash the key to look it up in the store.
			keyHash := sha256.Sum256([]byte(token))
			keyHashStr := hex.EncodeToString(keyHash[:])

			vaultID, err := s.ValidateAPIKey(keyHashStr)
			if err != nil {
				writeError(w, http.StatusInternalServerError, protocol.ErrCodeInternal, "internal error")
				return
			}
			if vaultID == "" {
				writeError(w, http.StatusUnauthorized, protocol.ErrCodeUnauthorized, "invalid API key")
				return
			}

			// Store vault_id in context for downstream handlers.
			ctx := context.WithValue(r.Context(), vaultIDKey, vaultID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetVaultID retrieves the vault_id from the request context.
func GetVaultID(r *http.Request) string {
	v, _ := r.Context().Value(vaultIDKey).(string)
	return v
}

// SizeLimit returns middleware that rejects request bodies exceeding maxBytes.
func SizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				writeError(w, http.StatusRequestEntityTooLarge, protocol.ErrCodePayloadTooLarge,
					"request body exceeds maximum allowed size")
				return
			}
			// Limit the read body as well.
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger returns middleware that logs method, path, status, and duration.
func RequestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Printf("%s %s %d %s",
				r.Method,
				r.URL.Path,
				ww.Status(),
				time.Since(start).Round(time.Microsecond),
			)
		})
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(protocol.ErrorResponse{Code: code, Message: message})
}

// writeJSON writes a JSON response with 200 status.
func writeJSON(w http.ResponseWriter, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

// writeJSONStatus writes a JSON response with a custom status code.
func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// Package auth provides API key generation, validation, and Bearer token extraction
// for the obsidian-sync protocol.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// KeyPrefix is the required prefix for all API keys.
	KeyPrefix = "osync_"

	// KeyHexLength is the expected length of the hex-encoded random portion (32 bytes = 64 hex chars).
	KeyHexLength = 64

	// KeyTotalLength is the total length of a key including the prefix.
	KeyTotalLength = len(KeyPrefix) + KeyHexLength
)

// GenerateKey creates a new API key with the osync_ prefix followed by
// 32 cryptographically random bytes hex-encoded (64 hex chars).
func GenerateKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return KeyPrefix + hex.EncodeToString(bytes), nil
}

// ValidateKey checks that a key has the correct osync_ prefix,
// the correct total length, and contains only hex characters after the prefix.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}

	if !strings.HasPrefix(key, KeyPrefix) {
		return fmt.Errorf("key must start with %q prefix", KeyPrefix)
	}

	hexPart := key[len(KeyPrefix):]
	if len(hexPart) != KeyHexLength {
		return fmt.Errorf("key must be %d hex characters after %q prefix, got %d",
			KeyHexLength, KeyPrefix, len(hexPart))
	}

	for _, c := range hexPart {
		if !isHexChar(c) {
			return fmt.Errorf("key contains invalid character %q after prefix, expected hex only", c)
		}
	}

	return nil
}

// ExtractBearerToken extracts the token from an Authorization header value.
// It expects the format "Bearer <token>" and returns the token string.
// Returns ("", false) if the header is empty, malformed, or not a Bearer token.
func ExtractBearerToken(authHeader string) (string, bool) {
	if authHeader == "" {
		return "", false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}

	return token, true
}

// isHexChar returns true if c is a valid lowercase or uppercase hex character.
func isHexChar(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
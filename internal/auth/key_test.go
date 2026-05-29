package auth

import (
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	// Must have osync_ prefix.
	if !strings.HasPrefix(key, KeyPrefix) {
		t.Errorf("key = %q, must have prefix %q", key, KeyPrefix)
	}

	// Must be correct total length.
	if len(key) != KeyTotalLength {
		t.Errorf("key length = %d, want %d", len(key), KeyTotalLength)
	}

	// Hex part must be valid hex characters.
	hexPart := key[len(KeyPrefix):]
	for _, c := range hexPart {
		if !isHexChar(c) {
			t.Errorf("hex part contains invalid char %q", c)
		}
	}

	// Must be lowercase hex (GenerateKey uses hex.EncodeToString which produces lowercase).
	for _, c := range hexPart {
		if c >= 'A' && c <= 'F' {
			t.Errorf("hex part contains uppercase char %q, want lowercase", c)
		}
	}
}

func TestGenerateKeyUniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey() error: %v", err)
		}
		if keys[key] {
			t.Fatalf("duplicate key generated: %s", key)
		}
		keys[key] = true
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid key",
			key:     "osync_" + strings.Repeat("a", 64),
			wantErr: false,
		},
		{
			name:    "valid key with mixed hex",
			key:     "osync_" + strings.Repeat("0123456789abcdef", 4),
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
		},
		{
			name:    "missing prefix",
			key:     strings.Repeat("a", 70),
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			key:     "sync_" + strings.Repeat("a", 64),
			wantErr: true,
		},
		{
			name:    "too short",
			key:     "osync_abc123",
			wantErr: true,
		},
		{
			name:    "too long",
			key:     "osync_" + strings.Repeat("a", 65),
			wantErr: true,
		},
		{
			name:    "invalid chars in hex part",
			key:     "osync_" + strings.Repeat("g", 64),
			wantErr: true,
		},
		{
			name:    "spaces in hex part",
			key:     "osync_" + strings.Repeat(" ", 64),
			wantErr: true,
		},
		{
			name:    "uppercase hex (valid hex but unusual)",
			key:     "osync_" + strings.Repeat("A", 64),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateKeyRejectsGeneratedThenModified(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	// Original key should validate.
	if err := ValidateKey(key); err != nil {
		t.Errorf("ValidateKey(generated key) error = %v, want nil", err)
	}

	// Truncated key should fail.
	if err := ValidateKey(key[:10]); err == nil {
		t.Error("ValidateKey(truncated key) should fail")
	}

	// Key with prefix removed should fail.
	if err := ValidateKey(key[len(KeyPrefix):]); err == nil {
		t.Error("ValidateKey(key without prefix) should fail")
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
		wantOK    bool
	}{
		{
			name:      "valid bearer token",
			header:    "Bearer osync_abc123",
			wantToken: "osync_abc123",
			wantOK:    true,
		},
		{
			name:      "valid bearer with long token",
			header:    "Bearer osync_" + strings.Repeat("f", 64),
			wantToken: "osync_" + strings.Repeat("f", 64),
			wantOK:    true,
		},
		{
			name:      "case insensitive bearer",
			header:    "bearer osync_test",
			wantToken: "osync_test",
			wantOK:    true,
		},
		{
			name:      "BEARER uppercase",
			header:    "BEARER osync_test",
			wantToken: "osync_test",
			wantOK:    true,
		},
		{
			name:      "empty header",
			header:    "",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "no scheme",
			header:    "osync_test",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "wrong scheme",
			header:    "Basic dXNlcjpwYXNz",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "bearer with no token",
			header:    "Bearer ",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "bearer only no space",
			header:    "Bearer",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "token with extra spaces trimmed",
			header:    "Bearer  osync_spaces  ",
			wantToken: "osync_spaces",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := ExtractBearerToken(tt.header)
			if ok != tt.wantOK {
				t.Errorf("ExtractBearerToken(%q) ok = %v, want %v", tt.header, ok, tt.wantOK)
			}
			if token != tt.wantToken {
				t.Errorf("ExtractBearerToken(%q) token = %q, want %q", tt.header, token, tt.wantToken)
			}
		})
	}
}
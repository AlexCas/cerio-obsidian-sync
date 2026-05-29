package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"ServerURL default", cfg.ServerURL, ""},
		{"APIKey default", cfg.APIKey, ""},
		{"VaultPath default", cfg.VaultPath, "."},
		{"Port default", cfg.Port, 8080},
		{"MaxFileSize default", cfg.MaxFileSize, int64(50 * 1024 * 1024)},
		{"SoftDeleteGraceDays default", cfg.SoftDeleteGraceDays, 30},
		{"PageSize default", cfg.PageSize, 5000},
		{"SyncInterval default", cfg.SyncInterval, 30 * time.Second},
		{"DebounceInterval default", cfg.DebounceInterval, 500 * time.Millisecond},
		{"SafetyScanInterval default", cfg.SafetyScanInterval, 60 * time.Second},
		{"SessionTimeout default", cfg.SessionTimeout, 10 * time.Minute},
		{"DataDir default", cfg.DataDir, "./data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}

	// Check excluded paths separately (slice comparison).
	defaults := DefaultExcludedPaths()
	if len(cfg.ExcludedPaths) != len(defaults) {
		t.Errorf("ExcludedPaths length = %d, want %d", len(cfg.ExcludedPaths), len(defaults))
	}
	for i, got := range cfg.ExcludedPaths {
		want := defaults[i]
		if got != want {
			t.Errorf("ExcludedPaths[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestLoadFromYAMLFile(t *testing.T) {
	// Create a temp directory with a config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server_url: "http://localhost:9090"
api_key: "osync_testkey123"
vault_path: "/tmp/vault"
port: 9090
max_file_size: 10485760
sync_interval: 60s
debounce_interval: 1s
excluded_paths:
  - .obsidian
  - .git
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ServerURL != "http://localhost:9090" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "http://localhost:9090")
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9090)
	}
	if cfg.MaxFileSize != 10485760 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 10485760)
	}
	if cfg.SyncInterval != 60*time.Second {
		t.Errorf("SyncInterval = %v, want %v", cfg.SyncInterval, 60*time.Second)
	}
	if cfg.DebounceInterval != 1*time.Second {
		t.Errorf("DebounceInterval = %v, want %v", cfg.DebounceInterval, 1*time.Second)
	}
	if len(cfg.ExcludedPaths) != 2 {
		t.Errorf("ExcludedPaths length = %d, want 2", len(cfg.ExcludedPaths))
	}
}

func TestLoadFromEnvVars(t *testing.T) {
	// Set env vars before loading.
	envVars := map[string]string{
		"OSYNC_SERVER_URL": "http://env-server:7070",
		"OSYNC_PORT":       "7070",
		"OSYNC_API_KEY":    "osync_environkey",
	}

	for key, val := range envVars {
		t.Setenv(key, val)
	}

	// Use a non-existent config file so only env vars apply.
	cfg, err := Load("/nonexistent/config.yaml", nil)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ServerURL != "http://env-server:7070" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "http://env-server:7070")
	}
	if cfg.Port != 7070 {
		t.Errorf("Port = %d, want %d", cfg.Port, 7070)
	}
	if cfg.APIKey != "osync_environkey" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "osync_environkey")
	}
}

func TestLoadDefaultsWhenNoConfig(t *testing.T) {
	// Load with non-existent file and no env overrides.
	cfg, err := Load("/nonexistent/config.yaml", nil)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Port != DefaultServerPort {
		t.Errorf("Port = %d, want default %d", cfg.Port, DefaultServerPort)
	}
	if cfg.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("MaxFileSize = %d, want default %d", cfg.MaxFileSize, DefaultMaxFileSize)
	}
	if cfg.SoftDeleteGraceDays != DefaultSoftDeleteGraceDays {
		t.Errorf("SoftDeleteGraceDays = %d, want default %d", cfg.SoftDeleteGraceDays, DefaultSoftDeleteGraceDays)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	// Create a config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server_url: "http://file-server:8080"
port: 8080
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	// Override with env var.
	t.Setenv("OSYNC_SERVER_URL", "http://env-override:9999")

	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Env should override file value.
	if cfg.ServerURL != "http://env-override:9999" {
		t.Errorf("ServerURL = %q, want %q (env override)", cfg.ServerURL, "http://env-override:9999")
	}
	// File value should still apply when no env override.
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d (from file)", cfg.Port, 8080)
	}
}

func TestDefaultExcludedPaths(t *testing.T) {
	paths := DefaultExcludedPaths()
	if len(paths) != 1 {
		t.Fatalf("DefaultExcludedPaths length = %d, want 1", len(paths))
	}
	if paths[0] != ".obsidian" {
		t.Errorf("DefaultExcludedPaths[0] = %q, want %q", paths[0], ".obsidian")
	}
}
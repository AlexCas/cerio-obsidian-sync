// Package config provides koanf-based configuration loading from files,
// environment variables, and command-line flags.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/spf13/pflag"
)

// Config holds all application configuration.
type Config struct {
	ServerURL           string        `koanf:"server_url" json:"server_url"`
	APIKey              string        `koanf:"api_key" json:"api_key"`
	VaultPath           string        `koanf:"vault_path" json:"vault_path"`
	VaultID             string        `koanf:"vault_id" json:"vault_id"`
	ClientID            string        `koanf:"client_id" json:"client_id"`
	ExcludedPaths       []string      `koanf:"excluded_paths" json:"excluded_paths"`
	SyncInterval        time.Duration `koanf:"sync_interval" json:"sync_interval"`
	PageSize            int           `koanf:"page_size" json:"page_size"`
	MaxFileSize         int64         `koanf:"max_file_size" json:"max_file_size"`
	Host                string        `koanf:"host" json:"host"`
	Port                int           `koanf:"port" json:"port"`
	DataDir             string        `koanf:"data_dir" json:"data_dir"`
	SoftDeleteGraceDays int           `koanf:"soft_delete_grace_days" json:"soft_delete_grace_days"`
	DebounceInterval    time.Duration `koanf:"debounce_interval" json:"debounce_interval"`
	SafetyScanInterval  time.Duration `koanf:"safety_scan_interval" json:"safety_scan_interval"`
	SessionTimeout      time.Duration `koanf:"session_timeout" json:"session_timeout"`
}

// Load loads configuration from the given file path, environment variables,
// and command-line flags. The precedence is: flags > env > file > defaults.
//
// Environment variables are prefixed with OSYNC_ and use _ as separator.
// For example, OSYNC_SERVER_URL maps to server_url.
//
// If configFile is empty, it defaults to ".osync/config.yaml" relative to
// the working directory. If the file does not exist, it is silently skipped.
func Load(configFile string, flags *pflag.FlagSet) (*Config, error) {
	cfg := DefaultConfig()
	k := koanf.New(".")

	// Resolve config file path.
	if configFile == "" {
		configFile = filepath.Join(DefaultConfigDir, DefaultConfigFile)
	}

	// Layer 1: file (lowest priority).
	if _, err := os.Stat(configFile); err == nil {
		if err := k.Load(file.Provider(configFile), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("loading config file %s: %w", configFile, err)
		}
	}

	// Layer 2: environment variables.
	// The custom callback lowercases but preserves underscores so that
	// OSYNC_SERVER_URL maps to server_url (flat key), not server.url (nested).
	if err := k.Load(env.Provider("OSYNC_", ".", envKeyTransform), nil); err != nil {
		return nil, fmt.Errorf("loading env vars: %w", err)
	}

	// Layer 3: command-line flags (highest priority).
	if flags != nil {
		if err := k.Load(posflag.Provider(flags, ".", k), nil); err != nil {
			return nil, fmt.Errorf("loading flags: %w", err)
		}
	}

	// Unmarshal into config struct (merges with defaults).
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return cfg, nil
}

// envKeyTransform converts an env var key (e.g., OSYNC_SERVER_URL) to a
// koanf flat key (e.g., server_url). It strips the prefix and lowercases,
// preserving underscores for flat keys instead of converting them to dots.
func envKeyTransform(s string) string {
	s = strings.TrimPrefix(s, "OSYNC_")
	return strings.ToLower(s)
}

// RegisterFlags binds common configuration flags to the given FlagSet.
func RegisterFlags(f *pflag.FlagSet) {
	f.String("server-url", "", "Server URL to sync with")
	f.String("api-key", "", "API key for authentication")
	f.String("vault-path", "", "Path to the Obsidian vault")
	f.Int("port", DefaultServerPort, "Server listen port")
	f.String("data-dir", DefaultDataDir, "Server data directory")
	f.Int("page-size", DefaultPageSize, "Number of entries per manifest page")
	f.String("config", filepath.Join(DefaultConfigDir, DefaultConfigFile), "Config file path")
}
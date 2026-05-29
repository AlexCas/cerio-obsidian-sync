package config

import "time"

// Default values for configuration.
const (
	DefaultServerPort            = 8080
	DefaultMaxFileSize    int64  = 50 * 1024 * 1024 // 50 MB
	DefaultSoftDeleteGraceDays   = 30
	DefaultDebounceInterval      = 500 * time.Millisecond
	DefaultSafetyScanInterval    = 60 * time.Second
	DefaultSessionTimeout        = 10 * time.Minute
	DefaultPageSize               = 5000
	DefaultSyncInterval           = 30 * time.Second
	DefaultConfigDir              = ".osync"
	DefaultConfigFile             = "config.yaml"
	DefaultDataDir                = "./data"
)

// DefaultExcludedPaths returns the default list of paths excluded from sync.
func DefaultExcludedPaths() []string {
	return []string{".obsidian"}
}

// DefaultConfig returns a Config populated with all default values.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:            "",
		APIKey:               "",
		VaultPath:            ".",
		VaultID:              "",
		ClientID:             "",
		ExcludedPaths:        DefaultExcludedPaths(),
		SyncInterval:         DefaultSyncInterval,
		PageSize:             DefaultPageSize,
		MaxFileSize:          DefaultMaxFileSize,
		Port:                 DefaultServerPort,
		Host:                 "0.0.0.0",
		DataDir:              DefaultDataDir,
		SoftDeleteGraceDays:  DefaultSoftDeleteGraceDays,
		DebounceInterval:    DefaultDebounceInterval,
		SafetyScanInterval:  DefaultSafetyScanInterval,
		SessionTimeout:      DefaultSessionTimeout,
	}
}
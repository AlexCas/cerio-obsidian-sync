package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/auth"
	"github.com/user/obsidian-sync-f2p/internal/config"
	"github.com/user/obsidian-sync-f2p/internal/server"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the osync sync server",
	Long:  "Start the osync HTTP server for syncing Obsidian vaults. The server listens for client connections and manages file storage, conflict resolution, and API key authentication.",
	Args:  cobra.NoArgs,
	RunE:  runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().String("addr", "", "Listen address (default: 0.0.0.0)")
	serverCmd.Flags().Int("port", 0, "Listen port (default: 8080)")
	serverCmd.Flags().String("data-dir", "", "Data directory for file storage and database (default: ./data)")
	serverCmd.Flags().String("db-path", "", "Database path (default: <data-dir>/osync.db)")
	serverCmd.Flags().String("vault-id", "", "Default vault ID for API key seeding (default: default)")
	serverCmd.Flags().String("api-key", "", "API key for authentication (or set OSYNC_API_KEY env var)")
	serverCmd.Flags().String("server-url", "", "Server URL (used for reference)")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load config from environment, flags, and defaults.
	// The server doesn't use a config file — it reads from env vars and flags.
	cfg, err := config.Load("", cmd.Flags())
	if err != nil {
		// Fall back to defaults if config loading fails (e.g., no config file).
		cfg = config.DefaultConfig()
	}

	// Override from flags if provided.
	if addr, _ := cmd.Flags().GetString("addr"); addr != "" {
		cfg.Host = addr
	}
	if port, _ := cmd.Flags().GetInt("port"); port != 0 {
		cfg.Port = port
	}
	if dataDir, _ := cmd.Flags().GetString("data-dir"); dataDir != "" {
		cfg.DataDir = dataDir
	}

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "osync.db")
	if explicit, _ := cmd.Flags().GetString("db-path"); explicit != "" {
		dbPath = explicit
	}

	// Open store.
	s := store.New(cfg.DataDir)
	if err := s.Open(dbPath); err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer s.Close()

	// Seed API key from environment if configured.
	// This allows Docker deployments to set OSYNC_API_KEY and have
	// it automatically registered on first boot.
	vaultID, _ := cmd.Flags().GetString("vault-id")
	if vaultID == "" {
		vaultID = "default"
	}
	if cfg.APIKey != "" {
		if err := seedAPIKey(s, cfg.APIKey, vaultID); err != nil {
			log.Printf("warning: failed to seed API key: %v", err)
		}
	}

	// Create and start server.
	addr := server.FormatAddr(cfg.Host, cfg.Port)
	srv := server.NewServer(server.ServerConfig{
		Addr:   addr,
		Store:  s,
		Logger: log.Default(),
	})

	log.Printf("Starting osync server on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// seedAPIKey registers an API key in the store if it doesn't already exist.
// It hashes the raw key for secure storage.
func seedAPIKey(s *store.Store, rawKey, vaultID string) error {
	// Validate the key format first.
	if err := auth.ValidateKey(rawKey); err != nil {
		return fmt.Errorf("invalid API key format: %w", err)
	}

	// Hash the key for storage.
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	if err := s.CreateAPIKey(keyHash, vaultID, "env-seeded"); err != nil {
		// Key might already exist — that's fine, log and continue.
		log.Printf("API key already seeded or error: %v (continuing)", err)
		return nil
	}

	log.Printf("API key seeded for vault %q", vaultID)
	return nil
}
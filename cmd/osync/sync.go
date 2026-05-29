package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/client"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize vault with the server",
	Long:  "Executes one sync cycle: begin → manifest exchange → file transfer → complete.",
	Args:  cobra.NoArgs,
	RunE:  runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	vaultPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Load config.
	configFile := filepath.Join(vaultPath, config.DefaultConfigDir, config.DefaultConfigFile)
	cfg, err := config.Load(configFile, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override vault path with current directory if not set.
	if cfg.VaultPath == "" || cfg.VaultPath == "." {
		cfg.VaultPath = vaultPath
	}

	// Validate required config.
	if cfg.ServerURL == "" {
		return fmt.Errorf("server_url is not configured; run 'osync config set server_url <url>'")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("api_key is not configured; run 'osync config set api_key <key>'")
	}

	// Open journal.
	journalPath := filepath.Join(vaultPath, config.DefaultConfigDir, "journal.db")
	journal := client.NewJournal(journalPath)
	if err := journal.Open(); err != nil {
		return fmt.Errorf("opening journal: %w", err)
	}
	defer journal.Close()

	// Create syncer with progress reporting.
	reporter := client.ProgressFunc(func(phase string, current, total int, message string) {
		fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s: %s\n", current, total, phase, message)
	})

	syncer := client.NewSyncer(client.SyncerConfig{
		Config:   cfg,
		Journal:  journal,
		Reporter: reporter,
	})

	// Execute sync.
	result, err := syncer.Sync(context.Background())
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Report results.
	fmt.Fprintln(cmd.OutOrStdout())
	if result.Uploaded == 0 && result.Downloaded == 0 && result.Conflicts == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Sync complete: %d uploaded, %d downloaded, %d conflicts\n",
			result.Uploaded, result.Downloaded, result.Conflicts)
	}

	return nil
}

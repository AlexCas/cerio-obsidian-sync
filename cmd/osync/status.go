package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/client"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	Long:  "Shows pending changes count, last sync timestamp, and connection status to the server.",
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	// Open journal.
	journalPath := filepath.Join(vaultPath, config.DefaultConfigDir, "journal.db")
	journal := client.NewJournal(journalPath)
	if err := journal.Open(); err != nil {
		return fmt.Errorf("opening journal: %w", err)
	}
	defer journal.Close()

	// Pending changes count.
	pendingCount, err := journal.PendingCount()
	if err != nil {
		return fmt.Errorf("counting pending changes: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Pending changes: %d\n", pendingCount)

	// Last sync timestamp.
	lastSync, err := journal.LastSync()
	if err != nil {
		return fmt.Errorf("reading last sync: %w", err)
	}
	if lastSync != "" {
		// Try to parse and format nicely.
		if t, err := time.Parse(time.RFC3339, lastSync); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Last sync: %s\n", t.Format("2006-01-02 15:04:05 UTC"))
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Last sync: %s\n", lastSync)
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Last sync: never")
	}

	// Connection status.
	fmt.Fprint(cmd.OutOrStdout(), "Server: ")
	if cfg.ServerURL == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "not configured")
	} else {
		connectivity := checkConnection(cfg.ServerURL, cfg.APIKey)
		fmt.Fprintf(cmd.OutOrStdout(), "connected (%s)\n", connectivity)
	}

	return nil
}

// checkConnection pings the server health endpoint and returns a status string.
func checkConnection(serverURL, apiKey string) string {
	client := &http.Client{Timeout: 5 * time.Second}

	url := serverURL + "/api/v1/health"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "error"
	}

	resp, err := client.Do(req)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "ok"
	}
	return fmt.Sprintf("status %d", resp.StatusCode)
}
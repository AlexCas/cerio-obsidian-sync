package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/client"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize osync in the current vault",
	Long:  "Creates the .osync/ directory with configuration and initializes the local journal database.",
	Args:  cobra.NoArgs,
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	vaultPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	osyncDir := filepath.Join(vaultPath, config.DefaultConfigDir)

	// Create .osync/ directory.
	if err := os.MkdirAll(osyncDir, 0o755); err != nil {
		return fmt.Errorf("creating %s directory: %w", osyncDir, err)
	}

	// Check if already initialized.
	configPath := filepath.Join(osyncDir, config.DefaultConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "osync already initialized in %s\n", vaultPath)
		return nil
	}

	// Generate a vault ID.
	vaultID, err := generateVaultID()
	if err != nil {
		return fmt.Errorf("generating vault ID: %w", err)
	}

	// Write default config.
	cfg := config.DefaultConfig()
	cfg.VaultPath = vaultPath
	cfg.VaultID = vaultID

	if err := writeConfig(configPath, cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Initialize journal database.
	journalPath := filepath.Join(osyncDir, "journal.db")
	journal := client.NewJournal(journalPath)
	if err := journal.Open(); err != nil {
		return fmt.Errorf("initializing journal: %w", err)
	}
	journal.Close()

	// Add .osync/ to .gitignore if it exists.
	gitignorePath := filepath.Join(vaultPath, ".gitignore")
	addToGitignore(gitignorePath, config.DefaultConfigDir)

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized osync in %s\n", vaultPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  Vault ID: %s\n", vaultID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Config:   %s\n", configPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  Journal:  %s\n", journalPath)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	fmt.Fprintln(cmd.OutOrStdout(), "  osync config set server_url <url>")
	fmt.Fprintln(cmd.OutOrStdout(), "  osync config set api_key <key>")
	fmt.Fprintln(cmd.OutOrStdout(), "  osync sync")

	return nil
}

// generateVaultID creates a random vault identifier.
func generateVaultID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// writeConfig writes the config struct to a YAML file.
func writeConfig(path string, cfg *config.Config) error {
	// Use koanf to write the config.
	// We'll write it manually as YAML for simplicity.
	yamlContent := fmt.Sprintf(`server_url: %q
api_key: %q
vault_path: %q
vault_id: %q
excluded_paths:
  - .obsidian
sync_interval: %s
max_file_size: %d
port: %d
data_dir: %q
soft_delete_grace_days: %d
debounce_interval: %s
safety_scan_interval: %s
session_timeout: %s
`,
		cfg.ServerURL,
		cfg.APIKey,
		cfg.VaultPath,
		cfg.VaultID,
		cfg.SyncInterval.String(),
		cfg.MaxFileSize,
		cfg.Port,
		cfg.DataDir,
		cfg.SoftDeleteGraceDays,
		cfg.DebounceInterval.String(),
		cfg.SafetyScanInterval.String(),
		cfg.SessionTimeout.String(),
	)

	return os.WriteFile(path, []byte(yamlContent), 0o644)
}

// addToGitignore appends a path to .gitignore if the file exists
// and the path is not already present.
func addToGitignore(gitignorePath, entry string) {
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		// .gitignore doesn't exist — skip.
		return
	}

	// Check if already present.
	content := string(data)
	lines := splitLines(content)
	for _, line := range lines {
		if line == entry {
			return // Already present.
		}
	}

	// Append the entry.
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString("\n" + entry + "\n")
}

// splitLines splits a string into lines, trimming whitespace.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := trimSpace(s[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		line := trimSpace(s[start:])
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// trimSpace trims leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

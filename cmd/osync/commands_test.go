package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/client"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

// newTestCmd creates a fresh cobra.Command with the given output buffer for testing.
func newTestCmd(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	cmd.SetOut(out)
	return cmd
}

func TestInitCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Error("init command not registered with root")
	}
}

func TestSyncCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "sync" {
			found = true
			break
		}
	}
	if !found {
		t.Error("sync command not registered with root")
	}
}

func TestConfigCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("config command not registered with root")
	}
}

func TestConfigSubcommands_Registered(t *testing.T) {
	configCmd, _, _ := rootCmd.Find([]string{"config"})
	if configCmd == nil {
		t.Fatal("config command not found")
	}

	subUses := make(map[string]bool)
	for _, cmd := range configCmd.Commands() {
		parts := strings.Fields(cmd.Use)
		if len(parts) > 0 {
			subUses[parts[0]] = true
		}
	}

	for _, expected := range []string{"get", "set", "list"} {
		if !subUses[expected] {
			t.Errorf("config subcommand %q not registered", expected)
		}
	}
}

func TestRunInit_CreatesOSyncDir(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	if _, err := os.Stat(osyncDir); os.IsNotExist(err) {
		t.Error(".osync directory was not created")
	}
}

func TestRunInit_CreatesConfig(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	configPath := filepath.Join(vaultDir, config.DefaultConfigDir, config.DefaultConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestRunInit_CreatesJournal(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	journalPath := filepath.Join(vaultDir, config.DefaultConfigDir, "journal.db")
	if _, err := os.Stat(journalPath); os.IsNotExist(err) {
		t.Error("journal database was not created")
	}
}

func TestRunInit_Idempotent(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	// First init.
	if err := runInit(cmd, []string{}); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	configPath := filepath.Join(vaultDir, config.DefaultConfigDir, config.DefaultConfigFile)
	originalData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	// Second init — should be idempotent.
	buf.Reset()
	if err := runInit(cmd, []string{}); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	newData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config after second init: %v", err)
	}

	if string(originalData) != string(newData) {
		t.Error("config was modified on second init — should be idempotent")
	}
}

func TestRunInit_AddsToGitignore(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	// Create .gitignore first.
	gitignorePath := filepath.Join(vaultDir, ".gitignore")
	os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	if err := runInit(cmd, []string{}); err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	if !strings.Contains(string(data), ".osync") {
		t.Error(".osync was not added to .gitignore")
	}
}

func TestConfigKeys(t *testing.T) {
	keys := configKeys()

	expectedKeys := []string{"server_url", "api_key", "vault_path", "excluded_paths", "port"}
	for _, expected := range expectedKeys {
		found := false
		for _, k := range keys {
			if k == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected config key %q not found in configKeys()", expected)
		}
	}
}

func TestIsValidConfigKey(t *testing.T) {
	if !isValidConfigKey("server_url") {
		t.Error("server_url should be a valid config key")
	}
	if !isValidConfigKey("api_key") {
		t.Error("api_key should be a valid config key")
	}
	if isValidConfigKey("nonexistent_key") {
		t.Error("nonexistent_key should not be valid")
	}
}

func TestGenerateVaultID(t *testing.T) {
	id, err := generateVaultID()
	if err != nil {
		t.Fatalf("generateVaultID: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("vault ID length = %d, want 32 (16 bytes hex)", len(id))
	}

	id2, _ := generateVaultID()
	if id == id2 {
		t.Error("two generated vault IDs should not be equal")
	}
}

func TestRunSync_RequiresServerURL(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	// Create .osync with empty config.
	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	os.WriteFile(filepath.Join(osyncDir, config.DefaultConfigFile), []byte("server_url: \"\"\napi_key: \"\"\n"), 0o644)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runSync(cmd, []string{})
	if err == nil {
		t.Error("expected error when server_url is not configured")
	}
}

func TestRunConfigList(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runConfigList(cmd, []string{})
	if err != nil {
		t.Fatalf("runConfigList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "server_url") {
		t.Error("config list should contain server_url")
	}
	if !strings.Contains(output, "api_key") {
		t.Error("config list should contain api_key")
	}
}

func TestRunConfigGetSet_RoundTrip(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	// Initialize with minimal config.
	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	cfg := config.DefaultConfig()
	cfg.VaultPath = vaultDir
	cfg.VaultID = "test-vault-id"
	writeConfig(filepath.Join(osyncDir, config.DefaultConfigFile), cfg)

	// Set server_url.
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runConfigSet(cmd, []string{"server_url", "http://localhost:9090"})
	if err != nil {
		t.Fatalf("runConfigSet failed: %v", err)
	}

	// Get server_url.
	buf.Reset()
	err = runConfigGet(cmd, []string{"server_url"})
	if err != nil {
		t.Fatalf("runConfigGet failed: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "http://localhost:9090" {
		t.Errorf("config get server_url = %q, want %q", output, "http://localhost:9090")
	}
}

func TestRunConfigGet_InvalidKey(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	cfg := config.DefaultConfig()
	writeConfig(filepath.Join(osyncDir, config.DefaultConfigFile), cfg)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runConfigGet(cmd, []string{"nonexistent_key"})
	if err == nil {
		t.Error("expected error for invalid config key")
	}
}

func TestRunConfigSet_InvalidKey(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	cfg := config.DefaultConfig()
	writeConfig(filepath.Join(osyncDir, config.DefaultConfigFile), cfg)

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runConfigSet(cmd, []string{"invalid_key", "value"})
	if err == nil {
		t.Error("expected error for invalid config key")
	}
}

func TestStatusCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Error("status command not registered with root")
	}
}

func TestMenuCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "menu" {
			found = true
			break
		}
	}
	if !found {
		t.Error("menu command not registered with root")
	}
}

func TestRootCommand_HasAllSubcommands(t *testing.T) {
	expected := []string{"version", "init", "sync", "config", "status", "server", "menu"}
	commands := rootCmd.Commands()

	for _, exp := range expected {
		found := false
		for _, cmd := range commands {
			if cmd.Name() == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("root command missing %q subcommand", exp)
		}
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := config.DefaultConfig()
	cfg.VaultPath = "/test/vault"
	cfg.VaultID = "abc123"
	cfg.ServerURL = "http://localhost:8080"

	if err := writeConfig(path, cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "/test/vault") {
		t.Error("config file should contain vault path")
	}
}

func TestAddToGitignore(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")

	// No .gitignore — should not create one.
	addToGitignore(gitignorePath, ".osync")
	if _, err := os.Stat(gitignorePath); !os.IsNotExist(err) {
		t.Error("should not create .gitignore if it doesn't exist")
	}

	// Create .gitignore and add entry.
	os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644)
	addToGitignore(gitignorePath, ".osync")

	data, _ := os.ReadFile(gitignorePath)
	if !strings.Contains(string(data), ".osync") {
		t.Error(".osync should be added to .gitignore")
	}

	// Adding again should not duplicate.
	addToGitignore(gitignorePath, ".osync")
	data2, _ := os.ReadFile(gitignorePath)
	count := strings.Count(string(data2), ".osync")
	if count != 1 {
		t.Errorf("expected .osync to appear once in .gitignore, got %d times", count)
	}
}

func TestRunStatus_ShowsPendingCount(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	// Initialize with config.
	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	cfg := config.DefaultConfig()
	cfg.VaultPath = vaultDir
	cfg.VaultID = "test-vault"
	cfg.ServerURL = ""
	writeConfig(filepath.Join(osyncDir, config.DefaultConfigFile), cfg)

	// Open journal and record a change.
	journal := client.NewJournal(filepath.Join(osyncDir, "journal.db"))
	if err := journal.Open(); err != nil {
		t.Fatalf("opening journal: %v", err)
	}
	defer journal.Close()

	if err := journal.RecordChange("test.md", client.JournalOpCreate, "abc123", 10); err != nil {
		t.Fatalf("recording change: %v", err)
	}

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runStatus(cmd, []string{})
	if err != nil {
		t.Fatalf("runStatus failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Pending changes: 1") {
		t.Errorf("expected 'Pending changes: 1' in output, got: %s", output)
	}
	if !strings.Contains(output, "Last sync: never") {
		t.Errorf("expected 'Last sync: never' in output, got: %s", output)
	}
	if !strings.Contains(output, "Server: not configured") {
		t.Errorf("expected 'Server: not configured' in output, got: %s", output)
	}
}

func TestRunStatus_ShowsLastSync(t *testing.T) {
	vaultDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(vaultDir)
	defer os.Chdir(origDir)

	osyncDir := filepath.Join(vaultDir, config.DefaultConfigDir)
	os.MkdirAll(osyncDir, 0o755)
	cfg := config.DefaultConfig()
	cfg.VaultPath = vaultDir
	cfg.VaultID = "test-vault"
	writeConfig(filepath.Join(osyncDir, config.DefaultConfigFile), cfg)

	// Open journal and set last_sync metadata.
	journal := client.NewJournal(filepath.Join(osyncDir, "journal.db"))
	if err := journal.Open(); err != nil {
		t.Fatalf("opening journal: %v", err)
	}
	defer journal.Close()

	if err := journal.SetMetadata("last_sync", "2026-05-28T14:30:00Z"); err != nil {
		t.Fatalf("setting metadata: %v", err)
	}

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := runStatus(cmd, []string{})
	if err != nil {
		t.Fatalf("runStatus failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Last sync:") {
		t.Errorf("expected 'Last sync:' in output, got: %s", output)
	}
	if strings.Contains(output, "never") {
		t.Errorf("should not show 'never' when last_sync is set, got: %s", output)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\n\nb", []string{"a", "b"}},
		{"  a  \n  b  ", []string{"a", "b"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

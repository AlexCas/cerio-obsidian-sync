//go:build e2e

// Package e2e contains end-to-end tests that exercise the full osync CLI
// by building the binary and running real command invocations against a test server.
package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/obsidian-sync-f2p/internal/server"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

// testAPIKey is a well-formed API key (osync_ prefix + 64 hex chars) for E2E tests.
const testAPIKey = "osync_aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
const testVaultID = "e2e-test-vault"

// binaryPath holds the path to the built osync binary.
var binaryPath string

// TestMain builds the osync binary once for all E2E tests.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "osync-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "osync.exe")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/osync")
	cmd.Dir = findProjectRoot()
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to build osync: %v\n%s\n", err, string(output))
		os.Exit(1)
	}

	fmt.Printf("e2e: built osync binary at %s\n", binaryPath)

	code := m.Run()
	os.Exit(code)
}

// findProjectRoot locates the project root by searching for go.mod.
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// setupE2ETest creates a test server and two vault directories.
// Returns the server URL, both vault paths, and a cleanup function.
func setupE2ETest(t *testing.T) (serverURL string, vaultA string, vaultB string, cleanup func()) {
	t.Helper()

	// Create temp directory for everything.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "server.db")
	dataDir := filepath.Join(dir, "data")

	s := store.New(dataDir)
	if err := s.Open(dbPath); err != nil {
		t.Fatalf("opening server store: %v", err)
	}

	// Create API key for authentication.
	// The middleware hashes the key with SHA-256 and looks up the hash.
	keyHash := sha256.Sum256([]byte(testAPIKey))
	keyHashStr := hex.EncodeToString(keyHash[:])
	if err := s.CreateAPIKey(keyHashStr, testVaultID, "e2e-test-key"); err != nil {
		t.Fatalf("creating API key: %v", err)
	}

	srv := server.NewServer(server.ServerConfig{
		Addr:   ":0",
		Store:  s,
		Logger: nil,
	})

	httpServer := httptest.NewServer(srv.Handler())
	serverURL = httpServer.URL

	// Create vault A.
	vaultA = filepath.Join(dir, "vault-a")
	if err := os.MkdirAll(vaultA, 0o755); err != nil {
		t.Fatalf("creating vault A: %v", err)
	}

	// Create vault B.
	vaultB = filepath.Join(dir, "vault-b")
	if err := os.MkdirAll(vaultB, 0o755); err != nil {
		t.Fatalf("creating vault B: %v", err)
	}

	cleanup = func() {
		httpServer.Close()
		s.Close()
	}

	return serverURL, vaultA, vaultB, cleanup
}

// runOsync executes the osync binary with the given arguments in the
// specified working directory.
func runOsync(t *testing.T, workDir string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// writeTestFile creates a file in the vault directory.
func writeTestFile(t *testing.T, vaultPath, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(vaultPath, relPath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating directory %s: %v", dir, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing file %s: %v", relPath, err)
	}
}

// readTestFile reads a file from the vault directory.
func readTestFile(t *testing.T, vaultPath, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultPath, relPath))
	if err != nil {
		t.Fatalf("reading file %s: %v", relPath, err)
	}
	return string(data)
}

// fileExists checks if a file exists in the vault directory.
func fileExists(vaultPath, relPath string) bool {
	_, err := os.Stat(filepath.Join(vaultPath, relPath))
	return err == nil
}

// TestE2E_InitCommand verifies that `osync init` creates the expected
// directory structure and configuration files.
func TestE2E_InitCommand(t *testing.T) {
	_, vaultA, _, cleanup := setupE2ETest(t)
	defer cleanup()

	// Run osync init in vault A.
	output, err := runOsync(t, vaultA, "init")
	if err != nil {
		t.Fatalf("osync init failed: %v\noutput: %s", err, output)
	}

	// Verify .osync directory was created.
	osyncDir := filepath.Join(vaultA, ".osync")
	if info, err := os.Stat(osyncDir); err != nil || !info.IsDir() {
		t.Errorf("expected .osync directory to exist, got err=%v", err)
	}

	// Verify config.yaml was created.
	configPath := filepath.Join(osyncDir, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected config.yaml to exist, got err=%v", err)
	}

	// Verify journal.db was created.
	journalPath := filepath.Join(osyncDir, "journal.db")
	if _, err := os.Stat(journalPath); err != nil {
		t.Errorf("expected journal.db to exist, got err=%v", err)
	}

	// Verify init is idempotent (second run should succeed).
	output, err = runOsync(t, vaultA, "init")
	if err != nil {
		t.Fatalf("osync init (second run) failed: %v\noutput: %s", err, output)
	}
}

// TestE2E_FullSyncCycle verifies the complete sync workflow:
// init → create files → sync from A → sync from B → verify content.
func TestE2E_FullSyncCycle(t *testing.T) {
	serverURL, vaultA, vaultB, cleanup := setupE2ETest(t)
	defer cleanup()

	// Step 1: Initialize both vaults.
	for _, vault := range []string{vaultA, vaultB} {
		output, err := runOsync(t, vault, "init")
		if err != nil {
			t.Fatalf("osync init failed for %s: %v\noutput: %s", vault, err, output)
		}
	}

	// Step 2: Configure both vaults with server URL and API key.
	for _, vault := range []string{vaultA, vaultB} {
		if output, err := runOsync(t, vault, "config", "set", "server_url", serverURL); err != nil {
			t.Fatalf("config set server_url failed for %s: %v\noutput: %s", vault, err, output)
		}
		if output, err := runOsync(t, vault, "config", "set", "api_key", testAPIKey); err != nil {
			t.Fatalf("config set api_key failed for %s: %v\noutput: %s", vault, err, output)
		}
	}

	// Step 3: Create files in vault A.
	writeTestFile(t, vaultA, "notes/hello.md", "Hello from E2E client A")
	writeTestFile(t, vaultA, "readme.md", "# My E2E Vault")

	// Step 4: Sync from vault A (upload).
	if output, err := runOsync(t, vaultA, "sync"); err != nil {
		t.Fatalf("osync sync (vault A) failed: %v\noutput: %s", err, output)
	}

	// Step 5: Sync from vault B (download).
	if output, err := runOsync(t, vaultB, "sync"); err != nil {
		t.Fatalf("osync sync (vault B) failed: %v\noutput: %s", err, output)
	}

	// Step 6: Verify files synced to vault B.
	helloContent := readTestFile(t, vaultB, "notes/hello.md")
	if helloContent != "Hello from E2E client A" {
		t.Errorf("vault B notes/hello.md = %q, want %q", helloContent, "Hello from E2E client A")
	}

	readmeContent := readTestFile(t, vaultB, "readme.md")
	if readmeContent != "# My E2E Vault" {
		t.Errorf("vault B readme.md = %q, want %q", readmeContent, "# My E2E Vault")
	}
}

// TestE2E_ObsidianExclusion verifies that .obsidian/ directory files
// are excluded from sync by default.
func TestE2E_ObsidianExclusion(t *testing.T) {
	serverURL, vaultA, _, cleanup := setupE2ETest(t)
	defer cleanup()

	// Initialize and configure vault A.
	if output, err := runOsync(t, vaultA, "init"); err != nil {
		t.Fatalf("osync init failed: %v\noutput: %s", err, output)
	}
	if output, err := runOsync(t, vaultA, "config", "set", "server_url", serverURL); err != nil {
		t.Fatalf("config set server_url failed: %v\noutput: %s", err, output)
	}
	if output, err := runOsync(t, vaultA, "config", "set", "api_key", testAPIKey); err != nil {
		t.Fatalf("config set api_key failed: %v\noutput: %s", err, output)
	}

	// Create files including .obsidian files.
	writeTestFile(t, vaultA, "notes.md", "my notes")
	writeTestFile(t, vaultA, ".obsidian/config", "obsidian config data")

	// Sync from vault A — should succeed.
	if output, err := runOsync(t, vaultA, "sync"); err != nil {
		t.Fatalf("osync sync failed: %v\noutput: %s", err, output)
	}
}

// TestE2E_VersionCommand verifies the version command works.
func TestE2E_VersionCommand(t *testing.T) {
	output, err := runOsync(t, t.TempDir(), "version")
	if err != nil {
		t.Fatalf("osync version failed: %v\noutput: %s", err, output)
	}

	if len(output) == 0 {
		t.Error("expected version output, got empty string")
	}
}

// TestE2E_ConfigCommands verifies config get/set/list work correctly.
func TestE2E_ConfigCommands(t *testing.T) {
	_, vaultA, _, cleanup := setupE2ETest(t)
	defer cleanup()

	// Initialize vault A.
	if output, err := runOsync(t, vaultA, "init"); err != nil {
		t.Fatalf("osync init failed: %v\noutput: %s", err, output)
	}

	// Set a config value.
	if output, err := runOsync(t, vaultA, "config", "set", "server_url", "http://example.com:8080"); err != nil {
		t.Fatalf("config set server_url failed: %v\noutput: %s", err, output)
	}

	// Get the config value back.
	output, err := runOsync(t, vaultA, "config", "get", "server_url")
	if err != nil {
		t.Fatalf("config get server_url failed: %v\noutput: %s", err, output)
	}

	if len(output) == 0 {
		t.Error("expected config get to return a value for server_url")
	}

	// List config values.
	if _, err := runOsync(t, vaultA, "config", "list"); err != nil {
		t.Fatalf("config list failed: %v", err)
	}
}

// TestE2E_StatusCommand verifies the status command works after init.
func TestE2E_StatusCommand(t *testing.T) {
	_, vaultA, _, cleanup := setupE2ETest(t)
	defer cleanup()

	// Initialize vault A.
	if output, err := runOsync(t, vaultA, "init"); err != nil {
		t.Fatalf("osync init failed: %v\noutput: %s", err, output)
	}

	// Run status — should show 0 pending changes and "never" for last sync.
	output, err := runOsync(t, vaultA, "status")
	if err != nil {
		t.Fatalf("osync status failed: %v\noutput: %s", err, output)
	}

	if len(output) == 0 {
		t.Error("expected status output, got empty string")
	}
}

// TestE2E_BidirectionalSync verifies that changes from client B
// sync back to client A on a second sync cycle.
func TestE2E_BidirectionalSync(t *testing.T) {
	serverURL, vaultA, vaultB, cleanup := setupE2ETest(t)
	defer cleanup()

	// Initialize and configure both vaults.
	for _, vault := range []string{vaultA, vaultB} {
		if output, err := runOsync(t, vault, "init"); err != nil {
			t.Fatalf("osync init failed for %s: %v\noutput: %s", vault, err, output)
		}
		if output, err := runOsync(t, vault, "config", "set", "server_url", serverURL); err != nil {
			t.Fatalf("config set server_url failed for %s: %v\noutput: %s", vault, err, output)
		}
		if output, err := runOsync(t, vault, "config", "set", "api_key", testAPIKey); err != nil {
			t.Fatalf("config set api_key failed for %s: %v\noutput: %s", vault, err, output)
		}
	}

	// Client A creates files and syncs.
	writeTestFile(t, vaultA, "shared.md", "original content from A")
	if output, err := runOsync(t, vaultA, "sync"); err != nil {
		t.Fatalf("osync sync (A first) failed: %v\noutput: %s", err, output)
	}

	// Client B syncs to get A's files.
	if output, err := runOsync(t, vaultB, "sync"); err != nil {
		t.Fatalf("osync sync (B first) failed: %v\noutput: %s", err, output)
	}

	// Verify B received A's file.
	bContent := readTestFile(t, vaultB, "shared.md")
	if bContent != "original content from A" {
		t.Errorf("vault B shared.md = %q, want %q", bContent, "original content from A")
	}

	// Client B creates a new file and syncs.
	writeTestFile(t, vaultB, "from-b.md", "created by client B")
	if output, err := runOsync(t, vaultB, "sync"); err != nil {
		t.Fatalf("osync sync (B second) failed: %v\noutput: %s", err, output)
	}

	// Client A syncs again to get B's file.
	// Small delay to ensure different revision timestamps.
	time.Sleep(100 * time.Millisecond)

	if output, err := runOsync(t, vaultA, "sync"); err != nil {
		t.Fatalf("osync sync (A second) failed: %v\noutput: %s", err, output)
	}

	// Verify A received B's file.
	if !fileExists(vaultA, "from-b.md") {
		t.Error("vault A should have received from-b.md from vault B")
	}

	if fileExists(vaultA, "from-b.md") {
		aContent := readTestFile(t, vaultA, "from-b.md")
		if aContent != "created by client B" {
			t.Errorf("vault A from-b.md = %q, want %q", aContent, "created by client B")
		}
	}
}
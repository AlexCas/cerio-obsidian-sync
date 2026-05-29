package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandExists(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd must be initialized")
	}
	if rootCmd.Use != "osync" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "osync")
	}
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short must not be empty")
	}
}

func TestVersionCommandOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(bytes.NewBuffer(nil))
	// Reset to default args after test
	defer rootCmd.SetArgs(nil)

	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	expected := "osync " + Version
	if output != expected {
		t.Errorf("version output = %q, want %q", output, expected)
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"version"})
	if err != nil {
		t.Fatalf("failed to find version command: %v", err)
	}
	if cmd.Use != "version" {
		t.Errorf("version command Use = %q, want %q", cmd.Use, "version")
	}
}

func TestRootCommandHasSubcommands(t *testing.T) {
	subcommands := rootCmd.Commands()
	if len(subcommands) == 0 {
		t.Error("rootCmd should have at least one subcommand")
	}

	names := make(map[string]bool)
	for _, cmd := range subcommands {
		names[cmd.Name()] = true
	}

	if !names["version"] {
		t.Error("rootCmd must have 'version' subcommand")
	}
}
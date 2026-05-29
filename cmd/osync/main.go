package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var menuCmd = &cobra.Command{
	Use:   "menu",
	Short: "Launch interactive TUI menu",
	Long:  "Launch an interactive terminal user interface for managing osync configuration and operations.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMenu()
	},
}

// Version is set at build time via -ldflags "-X main.Version=vX.Y.Z".
var Version = "v0.1.0"

var rootCmd = &cobra.Command{
	Use:   "osync",
	Short: "Obsidian vault synchronization tool",
	Long:  "osync synchronizes Obsidian vaults across devices using diff-based sync with SHA-256 content-addressed storage.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "osync %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(menuCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
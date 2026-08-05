// Package cmd is the Cobra command surface for agent-sweeper: a `sweep`
// subcommand that runs the interactive TUI flow and a `stats` subcommand that
// prints the per-agent footprint table.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is the CLI version, overridable at build time via -ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "agent-sweeper",
	Short: "Clean stale AI coding-agent session stores",
	Long: `agent-sweeper detects the six major coding agents' local session stores,
reports their filesystem footprint, and cleans stale sessions interactively.
Actively-open sessions are never touched, and nothing is deleted without review.`,
	Version: version,
}

// Execute runs the CLI, exiting with status 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

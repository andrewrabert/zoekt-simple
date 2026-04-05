// Package cli implements the unified zoekt CLI with subcommands.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root cobra command for the unified zoekt binary.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "zoekt",
		Short: "Zoekt code search tools",
		Long:  "A unified CLI for zoekt-simple code search: server, search client, file retrieval, and git cloning.",
	}

	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newGetFileCmd())
	rootCmd.AddCommand(newGitCloneCmd())

	// Add all upstream zoekt commands as subcommands.
	for _, cmd := range upstreamCommands() {
		rootCmd.AddCommand(cmd)
	}

	return rootCmd
}

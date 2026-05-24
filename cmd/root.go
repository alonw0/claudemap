package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "claudemap",
	Short: "CLAUDE.md context analyzer for Claude Code",
	Long:  "Discover, visualize, and validate the CLAUDE.md files that Claude Code loads for a given working directory.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

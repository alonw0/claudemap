package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alonw0/claudemap/render"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("claudemap", render.Version)
	},
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version holds the current version of the application.
// The default ("devel") is overridden via build flags.
var version = "devel"

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current version of turkis",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("turkis %s\n", version)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}

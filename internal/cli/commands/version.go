package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version holds the current version of the application.
// The default ("devel") is overridden via build flags.
var Version = "devel"

// NewVersionCmd creates a new version command
func VersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the current version of turkis",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("turkis %s\n", Version)
		},
	}

	return cmd
}

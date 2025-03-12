package commands

import (
	"fmt"

	"github.com/ameistad/turkis/internal/version"
	"github.com/spf13/cobra"
)

// VersionCmd creates a new version command
func VersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the current version of turkis",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("turkis %s\n", version.GetVersion())
		},
	}

	return cmd
}

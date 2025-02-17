package app

import (
	"github.com/spf13/cobra"
)

// NewAppCommand creates a new instance of the app command with its subcommands
func AppCmd() *cobra.Command {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
		Long:  `Commands for managing applications`,
	}

	appCmd.AddCommand(deployAppCmd())
	appCmd.AddCommand(deployAllCmd())
	appCmd.AddCommand(listAppsCmd())
	appCmd.AddCommand(statusAppCmd())
	appCmd.AddCommand(statusAllCmd())
	appCmd.AddCommand(rollbackAppCmd())

	return appCmd
}

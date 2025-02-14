package cmd

import (
	"github.com/ameistad/turkis/cmd/app"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:           "turkis",
	Short:         "turkis builds and runs Docker containers based on a YAML config",
	SilenceErrors: true, // Don't print errors automatically
	SilenceUsage:  true, // Don't show usage on error
}

func init() {
	RootCmd.AddCommand(app.AppCmd())
}

func Execute() error {
	return RootCmd.Execute()
}

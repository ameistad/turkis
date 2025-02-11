package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "turkis",
	Short:         "turkis builds and runs Docker containers based on a YAML config",
	SilenceErrors: true, // Don't print errors automatically
	SilenceUsage:  true, // Don't show usage on error
}

func Execute() error {
	return rootCmd.Execute()
}

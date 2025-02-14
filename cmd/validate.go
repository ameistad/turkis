package cmd

import (
	"fmt"

	"github.com/ameistad/turkis/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:          "validate",
	Short:        "Validate the config file",
	SilenceUsage: true, // Don't show usage on error
	RunE: func(cmd *cobra.Command, args []string) error {
		confFilePath, err := config.DefaultConfigFilePath()
		if err != nil {
			return fmt.Errorf("couldn't determine config file path: %w", err)
		}

		confFile, err := config.LoadConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("failed to load config from '%s': %w", confFilePath, err)
		}

		if err := config.ValidateConfigFile(confFile); err != nil {
			return fmt.Errorf("config validation error: %w", err)
		}

		fmt.Println("Config file is valid!")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(validateCmd)
}

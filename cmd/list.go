package cmd

import (
	"fmt"

	"github.com/ameistad/turkis/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all apps from config",
	RunE: func(cmd *cobra.Command, args []string) error {
		confFilePath, err := config.DefaultConfigFilePath()
		if err != nil {
			return err
		}

		confFile, err := config.LoadConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := config.ValidateConfigFile(confFile); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		fmt.Println("Apps in config:")
		for _, app := range confFile.Apps {
			fmt.Printf(" - %s\n", app.Name)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

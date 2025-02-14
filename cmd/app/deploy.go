package app

import (
	"fmt"

	"github.com/ameistad/turkis/config"
	"github.com/ameistad/turkis/deploy"
	"github.com/spf13/cobra"
)

func deployAppCmd() *cobra.Command {
	deployAppCmd := &cobra.Command{
		Use:   "deploy <app-name>",
		Short: "Deploy an application",
		Long:  `Deploy a single application by name`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			confFilePath, err := config.DefaultConfigFilePath()
			if err != nil {
				return err
			}
			confFile, err := config.LoadAndValidateConfig(confFilePath)
			if err != nil {
				return fmt.Errorf("configuration error: %w", err)
			}

			var appCfg *config.AppConfig
			for i := range confFile.Apps {
				if confFile.Apps[i].Name == appName {
					appCfg = &confFile.Apps[i]
					break
				}
			}
			if appCfg == nil {
				return fmt.Errorf("app '%s' not found in config", appName)
			}

			return deploy.DeployApp(appCfg)
		},
	}
	return deployAppCmd
}

func deployAllCmd() *cobra.Command {
	deployAllCmd := &cobra.Command{
		Use:   "deploy-all",
		Short: "Deploy all applications",
		Long:  `Deploy all applications defined in the configuration file.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			confFilePath, err := config.DefaultConfigFilePath()
			if err != nil {
				return err
			}
			confFile, err := config.LoadAndValidateConfig(confFilePath)
			if err != nil {
				return fmt.Errorf("configuration error: %w", err)
			}

			// Iterate over all apps using indices to take a pointer reference.
			for i := range confFile.Apps {
				appCfg := &confFile.Apps[i]
				fmt.Printf("Deploying app '%s'...\n", appCfg.Name)
				if err := deploy.DeployApp(appCfg); err != nil {
					fmt.Printf("Failed to deploy app '%s': %v\n", appCfg.Name, err)
				} else {
					fmt.Printf("Successfully deployed app '%s'.\n", appCfg.Name)
				}
			}
			return nil
		},
	}
	return deployAllCmd
}

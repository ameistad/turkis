package app

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/ameistad/turkis/config"
	dnsutil "github.com/ameistad/turkis/internal"
	"github.com/spf13/cobra"
)

func statusAppCmd() *cobra.Command {
	statusAppCmd := &cobra.Command{
		Use:   "status <app-name>",
		Short: "Get the status of an application",
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

			// Find app in config
			var app *config.AppConfig
			for _, a := range confFile.Apps {
				if a.Name == appName {
					app = &a
					break
				}
			}
			if app == nil {
				return fmt.Errorf("app '%s' not found in config", appName)
			}

			// Call printAppStatus and return its error (if any)
			if err := printAppStatus(app); err != nil {
				return err
			}

			return nil
		},
	}
	return statusAppCmd
}

func statusAllCmd() *cobra.Command {
	statusAllCmd := &cobra.Command{
		Use:   "status-all",
		Short: "Get the status of all applications in the configuration file",
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

			// Find app in config
			for i := range confFile.Apps {
				if err := printAppStatus(&confFile.Apps[i]); err != nil {
					return err
				}
			}

			return nil
		},
	}
	return statusAllCmd
}

func printAppStatus(app *config.AppConfig) error {

	containerID, err := getContainerID(app.Name)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}

	// Get container status
	status, err := getContainerStatus(app.Name)
	if err != nil {
		return fmt.Errorf("failed to get container status: %w", err)
	}
	fmt.Printf("App: %s\n", app.Name)
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Domains:\n")
	for _, d := range app.Domains {
		// Display the canonical domain
		ip, err := dnsutil.GetARecord(d.Domain)
		if err != nil {
			fmt.Printf("  %s -> no A record found\n", d.Domain)
		} else {
			fmt.Printf("  %s -> %s\n", d.Domain, ip.String())
		}

		// Display any aliases if available
		for _, alias := range d.Aliases {
			ipAlias, err := dnsutil.GetARecord(alias)
			if err != nil {
				fmt.Printf("  %s -> no A record found\n", alias)
			} else {
				fmt.Printf("  %s -> %s\n", alias, ipAlias.String())
			}
		}
	}
	fmt.Printf("Container ID: %s\n", containerID)
	fmt.Printf("Dockerfile: %s\n", app.Dockerfile)
	fmt.Printf("Build Context: %s\n", app.BuildContext)

	if len(app.Env) > 0 {
		fmt.Println("Environment Variables:")
		for k, v := range app.Env {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	return nil
}

// getContainerID returns the container ID for an app by filtering on the image ancestor.
func getContainerID(appName string) (string, error) {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("ancestor=%s:latest", appName), "--format", "{{.ID}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getContainerStatus returns the container status for an app.
func getContainerStatus(appName string) (string, error) {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("ancestor=%s:latest", appName), "--format", "{{.Status}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	status := strings.TrimSpace(string(output))
	if status == "" {
		return "Not running", nil
	}
	return status, nil
}

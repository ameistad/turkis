package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ameistad/turkis/config"
	"github.com/ameistad/turkis/internal/embed"
	"github.com/spf13/cobra"
)

// TraefikConfig holds Traefik-specific settings.
type TraefikConfig struct {
	Email         string
	DockerNetwork string
}

// TemplateData now contains a nested Traefik field.
type TemplateData struct {
	Traefik TraefikConfig
}

// promptForTraefikConfig prompts the user to enter Traefik configuration.
func promptForTraefikConfig() (TraefikConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter ACME email used for setting up TLS certificates: \n")
	email, err := reader.ReadString('\n')
	if err != nil {
		return TraefikConfig{}, err
	}

	return TraefikConfig{
		Email:         strings.TrimSpace(email),
		DockerNetwork: config.DockerNetwork,
	}, nil
}

// createConfigFile backs up the current config file (if it exists) and writes a new one.
func createConfigFile(data TemplateData) error {
	configFilePath, err := config.DefaultConfigFilePath()
	if err != nil {
		return err
	}

	// Backup if the config file exists.
	if _, err := os.Stat(configFilePath); err == nil {
		backupPath := filepath.Join(filepath.Dir(configFilePath), "old-"+filepath.Base(configFilePath))
		if err := os.Rename(configFilePath, backupPath); err != nil {
			return fmt.Errorf("failed to backup config file: %w", err)
		}
		fmt.Printf("Backed up config file to %s\n", backupPath)
	}

	// Read and execute the template.
	templateContent, err := embed.TemplateFS.ReadFile("templates/apps.yml")
	if err != nil {
		return fmt.Errorf("failed to read config template: %w", err)
	}

	tmpl, err := template.New("apps").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse config template: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(configFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	if err = tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	fmt.Printf("Successfully created config file '%s'\n", configFilePath)
	return nil
}

// installTraefik backs up the existing Traefik compose file (if any) and writes a new one.
// It does not automatically run the container so that the user can review and edit it first.
// installTraefik backs up the existing Traefik compose file (if any) and writes a new one.
// It also creates the dynamic directory and copies all required configuration files.
func installTraefik(data TemplateData) error {
	confDir, err := config.DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	traefikDir := filepath.Join(confDir, "traefik")
	if err := os.MkdirAll(traefikDir, 0755); err != nil {
		return fmt.Errorf("failed to create traefik directory: %w", err)
	}

	// Create dynamic directory
	dynamicDir := filepath.Join(traefikDir, "dynamic")
	if err := os.MkdirAll(dynamicDir, 0755); err != nil {
		return fmt.Errorf("failed to create traefik dynamic directory: %w", err)
	}

	composePath := filepath.Join(traefikDir, "docker-compose.yml")

	// Backup existing traefik compose file.
	if _, err := os.Stat(composePath); err == nil {
		backupPath := filepath.Join(traefikDir, "old-docker-compose.yml")
		if err := os.Rename(composePath, backupPath); err != nil {
			return fmt.Errorf("failed to backup traefik compose file: %w", err)
		}
		fmt.Printf("Backed up Traefik compose file to %s\n", backupPath)
	} else {
		// For clarity, print that no backup was needed.
		fmt.Println("No existing Traefik compose file found; no backup was created.")
	}

	// Copy docker-compose.yml
	templateContent, err := embed.TemplateFS.ReadFile("templates/traefik/docker-compose.yml")
	if err != nil {
		return fmt.Errorf("failed to read traefik template: %w", err)
	}

	tmpl, err := template.New("traefik-compose").Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse traefik template: %w", err)
	}

	f, err := os.Create(composePath)
	if err != nil {
		return fmt.Errorf("failed to create traefik compose file: %w", err)
	}
	defer f.Close()

	if err = tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute traefik template: %w", err)
	}

	fmt.Printf("Successfully created Traefik compose file '%s'\n", composePath)
	fmt.Println("Please review and edit the configuration as needed, then manually run:")
	fmt.Printf("  docker compose -f %s up -d\n", composePath)
	return nil
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration files and prepare Traefik for production",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Prompt the user for Traefik configuration.
		traefikConf, err := promptForTraefikConfig()
		if err != nil {
			return fmt.Errorf("failed to get Traefik configuration: %w", err)
		}

		// Build the template data with Traefik settings.
		data := TemplateData{
			Traefik: traefikConf,
		}

		// Backup and overwrite the apps configuration file.
		if err := createConfigFile(data); err != nil {
			return fmt.Errorf("failed to generate config file: %w", err)
		}

		// Backup and overwrite the Traefik Docker Compose file.
		if err := installTraefik(data); err != nil {
			return fmt.Errorf("failed to generate Traefik compose file: %w", err)
		}

		// Compute the path to the generated docker-compose file.
		confDir, err := config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to determine config directory: %w", err)
		}
		composePath := filepath.Join(confDir, "traefik", "docker-compose.yml")

		// Print instructions to the user.
		fmt.Println("Configuration files have been generated and backed up as needed.")
		fmt.Printf("Please review your Traefik compose file at:\n  %s\n", composePath)
		fmt.Println("When you're ready, start Traefik by running:")
		fmt.Printf("  docker compose -f %s up -d\n", composePath)

		return nil
	},
}

func init() {
	RootCmd.AddCommand(initCmd)
}

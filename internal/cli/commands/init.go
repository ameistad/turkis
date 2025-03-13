package commands

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"github.com/ameistad/turkis/internal/config"
	"github.com/ameistad/turkis/internal/embed"
	"github.com/spf13/cobra"
)

// NewInitCmd creates a new init command
func InitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration files and prepare HAProxy for production",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get the default config directory.
			configDir, err := config.DefaultConfigDirPath()
			if err != nil {
				return fmt.Errorf("failed to determine config directory: %w", err)
			}

			// Check if directory already exists
			if _, err := os.Stat(configDir); err == nil {
				fmt.Println("Warning: Configuration directory already exists. Files may be overwritten.")
			}

			if err := copyTemplates(configDir); err != nil {
				return err
			}

			// Prompt the user for email and update apps.yml.
			if err := updateConfig(); err != nil {
				return err
			}

			fmt.Printf("Configuration files created successfully in %s\n", configDir)
			fmt.Println("Add your applications to apps.yml and run 'turkis deploy <app-name>' to start the reverse proxy.")
			fmt.Println("\nBefore starting HAProxy and the monitor, run the setup script:")
			fmt.Printf("cd %s/containers && ./setup.sh\n", configDir)
			fmt.Println("\nThen start the containers with:")
			fmt.Printf("docker compose -f %s/containers/docker-compose.yml up -d", configDir)
			return nil
		},
	}

	return cmd
}

func copyTemplates(dst string) error {
	fmt.Printf("Copying templates to %s\n", dst)
	// Create the destination directory if it doesn't exist
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Walk the embedded filesystem starting at the init directory.
	return fs.WalkDir(embed.InitFS, "init", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking embedded filesystem: %w", err)
		}

		// Compute the relative path based on the init directory.
		relPath, err := filepath.Rel("init", path)
		if err != nil {
			return fmt.Errorf("failed to determine relative path: %w", err)
		}

		targetPath := filepath.Join(dst, relPath)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Read the file from the embed FS.
		data, err := embed.InitFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Determine the file mode - make shell scripts executable
		fileMode := fs.FileMode(0644)
		if filepath.Ext(targetPath) == ".sh" {
			fileMode = 0755
		}

		if err := os.WriteFile(targetPath, data, fileMode); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		return nil
	})
}

// promptForEmailAndUpdateConfig prompts the user for an email and replaces the email in the config file
func updateConfig() error {
	// Prompt for email with validation
	// var email string
	// for {
	// 	fmt.Print("Enter email for Let's Encrypt TLS certificates: ")
	// 	if _, err := fmt.Scanln(&email); err != nil {
	// 		if err.Error() == "unexpected newline" {
	// 			fmt.Println("Email cannot be empty")
	// 			continue
	// 		}
	// 		return fmt.Errorf("failed to read email input: %w", err)
	// 	}

	// 	if !helpers.IsValidEmail(email) {
	// 		fmt.Println("Please enter a valid email address")
	// 		continue
	// 	}
	// 	break
	// }

	// Get the full path to apps.yml.
	configFile, err := config.DefaultConfigFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine config file path: %w", err)
	}

	fmt.Printf("Updating configuration file: %s\n", configFile)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %w", configFile, err)
	}

	// Use text/template instead of string replacement
	tmpl, err := template.New("config").Parse(string(data))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Create a buffer to store the output
	var buf bytes.Buffer

	// Execute template with data
	templateData := struct {
		ConfigDirPath string
	}{}
	configDirPath, err := config.DefaultConfigDirPath()
	if err != nil {
		return fmt.Errorf("failed to write updated config file: %w", err)
	}
	templateData.ConfigDirPath = configDirPath

	if err := tmpl.Execute(&buf, templateData); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := os.WriteFile(configFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write updated config file: %w", err)
	}

	fmt.Println("Configuration file updated successfully")
	return nil
}

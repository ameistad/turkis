package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Domain represents either a simple canonical domain or a mapping that includes aliases.
// When decoding a scalar, the value is assigned to the Domain field and Aliases will be empty.
type Domain struct {
	Domain  string   `yaml:"domain"`
	Aliases []string `yaml:"aliases,omitempty"`
}

// UnmarshalYAML handles decoding a Domain from either a plain scalar or a mapping.
func (d *Domain) UnmarshalYAML(value *yaml.Node) error {
	// If the YAML node is a scalar, treat it as a simple canonical domain.
	if value.Kind == yaml.ScalarNode {
		d.Domain = value.Value
		d.Aliases = []string{}
		return nil
	}

	// If the node is a mapping, decode it normally.
	if value.Kind == yaml.MappingNode {
		type domainAlias Domain // alias to avoid recursion
		var da domainAlias
		if err := value.Decode(&da); err != nil {
			return err
		}
		*d = Domain(da)
		// Ensure Aliases is not nil.
		if d.Aliases == nil {
			d.Aliases = []string{}
		}
		return nil
	}

	return fmt.Errorf("unexpected YAML node kind %d for Domain", value.Kind)
}

// AppConfig defines the configuration for an application.
type AppConfig struct {
	Name         string            `yaml:"name"`
	Domains      []Domain          `yaml:"domains"`
	Dockerfile   string            `yaml:"dockerfile"`
	BuildContext string            `yaml:"buildContext"`
	Env          map[string]string `yaml:"env"`
}

// TraefikConfig contains global Traefik settings.
type TraefikConfig struct {
	Email  string `yaml:"email"`
	Domain string `yaml:"domain"`
}

// Config represents the overall configuration.
type Config struct {
	Traefik TraefikConfig `yaml:"traefik"`
	Apps    []AppConfig   `yaml:"apps"`
}

const (
	ConfigFileName = "apps.yml"
)

// DefaultConfigPath returns "~/.config/turkis".
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "turkis"), nil
}

// DefaultConfigFilePath returns "~/.config/turkis/apps.yml".
func DefaultConfigFilePath() (string, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(configPath, ConfigFileName), nil
}

// LoadConfig loads YAML from the provided path into a Config struct.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}
	var conf Config
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &conf, nil
}

// ValidateConfigFile checks that the Config is well-formed.
func ValidateConfigFile(conf *Config) error {
	// Validate Traefik configuration.
	if conf.Traefik.Email == "" {
		return errors.New("traefik acme email is missing in config")
	}
	if conf.Traefik.Domain == "" {
		return errors.New("traefik domain is missing in config")
	}
	// Validate apps.
	if len(conf.Apps) == 0 {
		return errors.New("no apps defined in config")
	}
	for _, app := range conf.Apps {
		if app.Name == "" {
			return errors.New("found an app with an empty name")
		}
		if len(app.Domains) == 0 {
			return fmt.Errorf("app '%s': no domains defined", app.Name)
		}
		for _, domain := range app.Domains {
			if domain.Domain == "" {
				return fmt.Errorf("app '%s': found an empty domain", app.Name)
			}
		}
		if app.Dockerfile == "" {
			return fmt.Errorf("app '%s': missing dockerfile path", app.Name)
		}
		if app.BuildContext == "" {
			return fmt.Errorf("app '%s': missing build context path", app.Name)
		}
		// Check Dockerfile.
		fileInfo, err := os.Stat(app.Dockerfile)
		if os.IsNotExist(err) {
			return fmt.Errorf("app '%s': dockerfile '%s' does not exist", app.Name, app.Dockerfile)
		} else if err != nil {
			return fmt.Errorf("app '%s': unable to check dockerfile '%s': %w", app.Name, app.Dockerfile, err)
		}
		if fileInfo.IsDir() {
			return fmt.Errorf("app '%s': dockerfile '%s' is a directory, not a file", app.Name, app.Dockerfile)
		}

		// Check BuildContext.
		ctxInfo, err := os.Stat(app.BuildContext)
		if os.IsNotExist(err) {
			return fmt.Errorf("app '%s': build context '%s' does not exist", app.Name, app.BuildContext)
		} else if err != nil {
			return fmt.Errorf("app '%s': unable to check build context '%s': %w", app.Name, app.BuildContext, err)
		}
		if !ctxInfo.IsDir() {
			return fmt.Errorf("app '%s': build context '%s' is not a directory", app.Name, app.BuildContext)
		}
	}
	return nil
}

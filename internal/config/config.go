package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// DockerNetwork is the network name to which containers are attached.
	DockerNetwork = "turkis-public"

	// DefaultContainerPort is the port on which your container serves HTTP.
	DefaultContainerPort = 80

	ConfigFileName = "apps.yml"
)

func DefaultConfigDirPath() (string, error) {
	// First check if TURKIS_CONFIG_PATH is set.
	if envPath, ok := os.LookupEnv("TURKIS_CONFIG_PATH"); ok && envPath != "" {
		// Validate that the path exists and is a directory.
		if info, err := os.Stat(envPath); err == nil && info.IsDir() {
			return envPath, nil
		}
		return "", fmt.Errorf("TURKIS_CONFIG_PATH is set to '%s' but it is not a valid directory", envPath)
	}

	// Fallback to the default path.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "turkis"), nil
}

// DefaultConfigFilePath returns "~/.config/turkis/apps.yml".
func DefaultConfigFilePath() (string, error) {
	configDirPath, err := DefaultConfigDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirPath, ConfigFileName), nil
}

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
	Name              string            `yaml:"name"`
	Domains           []Domain          `yaml:"domains"`
	Dockerfile        string            `yaml:"dockerfile"`
	BuildContext      string            `yaml:"buildContext"`
	Env               map[string]string `yaml:"env"`
	KeepOldContainers int               `yaml:"keepOldContainers,omitempty"`
	Volumes           []string          `yaml:"volumes,omitempty"`
	HealthCheckPath   string            `yaml:"healthCheckPath,omitempty"`
}

// TraefikConfig contains global Traefik settings.
type TLSConfig struct {
	Email string `yaml:"email"`
}

// Config represents the overall configuration.
type Config struct {
	TLS  TLSConfig   `yaml:"tls"`
	Apps []AppConfig `yaml:"apps"`
}

// NormalizeConfig sets default values for the loaded configuration.
func NormalizeConfig(conf *Config) *Config {
	normalized := *conf
	normalized.Apps = make([]AppConfig, len(conf.Apps))
	for i, app := range conf.Apps {
		normalized.Apps[i] = app

		// Default KeepOldContainers to 3 if not set.
		if app.KeepOldContainers == 0 {
			normalized.Apps[i].KeepOldContainers = 3
		}

		// Default health check path to "/" if not set.
		if app.HealthCheckPath == "" {
			normalized.Apps[i].HealthCheckPath = "/"
		}
	}
	return &normalized
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}

// LoadAndValidateConfig loads the configuration from a file, normalizes it, and validates it.
func LoadAndValidateConfig(path string) (*Config, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	normalizedConfig := NormalizeConfig(config)

	if err := ValidateConfigFile(normalizedConfig); err != nil {
		return nil, err
	}
	return normalizedConfig, nil
}

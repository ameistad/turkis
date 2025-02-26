package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DockerNetwork = "turkis-public"

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
}

// TraefikConfig contains global Traefik settings.
type TraefikConfig struct {
	Email string `yaml:"email"`
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

// NormalizeConfig sets default values for the loaded configuration.
func NormalizeConfig(conf *Config) {
	for i, app := range conf.Apps {
		if app.KeepOldContainers == 0 {
			conf.Apps[i].KeepOldContainers = 3
		}
	}
}

// LoadAndValidateConfig loads the configuration from a file, normalizes it, and validates it.
func LoadAndValidateConfig(path string) (*Config, error) {
	conf, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	NormalizeConfig(conf)

	if err := ValidateConfigFile(conf); err != nil {
		return nil, err
	}
	return conf, nil
}

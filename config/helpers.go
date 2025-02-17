package config

import "fmt"

func AppConfigByName(appName string) (*AppConfig, error) {
	configFilePath, err := DefaultConfigFilePath()
	if err != nil {
		return nil, err
	}

	configFile, err := LoadAndValidateConfig(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	var appConfig *AppConfig
	for i := range configFile.Apps {
		if configFile.Apps[i].Name == appName {
			appConfig = &configFile.Apps[i]
			break
		}
	}
	if appConfig == nil {
		return nil, fmt.Errorf("app '%s' not found in config", appName)
	}

	return appConfig, nil
}

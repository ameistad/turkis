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
			// Create a copy of the app config
			app := configFile.Apps[i]
			appConfig = &app
			break
		}
	}
	if appConfig == nil {
		return nil, fmt.Errorf("app '%s' not found in config", appName)
	}

	// Store the TLS email in the environment variables map
	// This is a bit of a hack, but allows us to pass this information to the deploy function
	if appConfig.Env == nil {
		appConfig.Env = make(map[string]string)
	}
	// Use a special key that won't conflict with actual environment variables
	appConfig.Env["__TURKIS_TLS_EMAIL"] = configFile.TLS.Email

	return appConfig, nil
}

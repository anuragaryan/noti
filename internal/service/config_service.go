package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"noti/internal/domain"
)

// ConfigService handles application configuration
type ConfigService struct {
	basePath      string
	defaultConfig []byte
}

// NewConfigService creates a new config service
func NewConfigService(basePath string, defaultConfig []byte) *ConfigService {
	return &ConfigService{
		basePath:      basePath,
		defaultConfig: defaultConfig,
	}
}

// Load loads the application configuration from config.json
func (s *ConfigService) Load() (*domain.Config, error) {
	configFilePath := filepath.Join(s.basePath, "config.json")

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		// Config file does not exist, so create it from the embedded template
		fmt.Printf("config.json not found. Creating from embedded template at %s\n", configFilePath)
		// Ensure the directory for config.json exists
		dir := filepath.Dir(configFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for config.json: %w", err)
		}
		if err := os.WriteFile(configFilePath, s.defaultConfig, 0644); err != nil {
			return nil, fmt.Errorf("failed to write default config file: %w", err)
		}
		// Use the embedded config data for this session
		data = s.defaultConfig
	}

	// Unmarshal the config data (either from file or the embedded default)
	var config domain.Config
	if err := json.Unmarshal(data, &config); err != nil {
		// If unmarshalling fails, it could be a corrupt file. Try to restore it
		fmt.Printf("WARNING: Failed to unmarshal config.json: %v. Restoring from template.\n", err)
		if err := os.WriteFile(configFilePath, s.defaultConfig, 0644); err != nil {
			return nil, fmt.Errorf("failed to restore default config file: %w", err)
		}
		data = s.defaultConfig // Use the default config for this session
		// Retry unmarshalling with the default data
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedded default config: %w", err)
		}
	}

	// Set defaults for any fields that might be missing (for backward compatibility)
	if config.RealtimeTranscriptionChunkSeconds <= 0 {
		config.RealtimeTranscriptionChunkSeconds = 3
	}
	if config.ModelName == "" {
		config.ModelName = "base.en"
	}

	fmt.Printf("Loaded config from: %s\n", configFilePath)
	return &config, nil
}

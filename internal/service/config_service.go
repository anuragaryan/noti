package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"noti/internal/domain"
)

// ConfigService handles application configuration
type ConfigService struct {
	basePath      string
	defaultConfig []byte
	firstRun      bool
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
	s.firstRun = false

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		// Config file does not exist, so create it from the embedded template
		slog.Info("config.json not found. Creating from embedded template", "path", configFilePath)
		// Ensure the directory for config.json exists
		dir := filepath.Dir(configFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for config.json: %w", err)
		}
		if err := os.WriteFile(configFilePath, s.defaultConfig, 0644); err != nil {
			return nil, fmt.Errorf("failed to write default config file: %w", err)
		}
		s.firstRun = true
		// Use the embedded config data for this session
		data = s.defaultConfig
	}

	// Unmarshal the config data (either from file or the embedded default)
	var config domain.Config
	if err := json.Unmarshal(data, &config); err != nil {
		// If unmarshalling fails, it could be a corrupt file. Try to restore it
		slog.Warn("Failed to unmarshal config.json. Restoring from template.", "error", err)
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
	// Note: RealtimeTranscriptionChunkSeconds is deprecated and ignored
	if config.ModelName == "" {
		config.ModelName = "base.en"
	}

	// Set LLM defaults if not configured
	if config.LLM.Provider == "" {
		config.LLM.Provider = "api" // Default to API provider
	}
	if config.LLM.Temperature == 0 {
		config.LLM.Temperature = 0.7
	}
	if config.LLM.MaxTokens == 0 {
		config.LLM.MaxTokens = 512
	}

	// Set Audio defaults if not configured
	if config.Audio.DefaultSource == "" {
		config.Audio.DefaultSource = "microphone"
	}
	if config.Audio.Mixer.MicrophoneGain == 0 {
		config.Audio.Mixer.MicrophoneGain = 1.0
	}
	if config.Audio.Mixer.SystemGain == 0 {
		config.Audio.Mixer.SystemGain = 1.0
	}
	if config.Audio.Mixer.MixMode == "" {
		config.Audio.Mixer.MixMode = "sum"
	}

	slog.Info("Loaded config", "path", configFilePath)
	return &config, nil
}

// IsFirstRun reports whether config.json was created during the latest Load call.
func (s *ConfigService) IsFirstRun() bool {
	return s.firstRun
}

// Save saves the configuration to config.json
func (s *ConfigService) Save(config *domain.Config) error {
	configFilePath := filepath.Join(s.basePath, "config.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	slog.Info("Saved config", "path", configFilePath)
	return nil
}

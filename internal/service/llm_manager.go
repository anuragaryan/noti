package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"noti/internal/domain"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LLMManager handles LLM service lifecycle and provider management
type LLMManager struct {
	basePath       string
	ctx            context.Context
	provider       LLMProvider
	config         *domain.LLMConfig
	downloadScript []byte
}

// ProviderFactory is a function that creates an LLM provider
type ProviderFactory func(basePath string, config *domain.LLMConfig) (LLMProvider, error)

// NewLLMManager creates a new LLM manager
func NewLLMManager(basePath string, downloadScript []byte) *LLMManager {
	return &LLMManager{
		basePath:       basePath,
		downloadScript: downloadScript,
	}
}

// SetContext sets the Wails runtime context
func (m *LLMManager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Initialize attempts to initialize the LLM provider
func (m *LLMManager) Initialize(config *domain.LLMConfig, factory ProviderFactory) error {
	if config == nil || config.Provider == "" {
		return fmt.Errorf("invalid LLM configuration")
	}

	m.config = config

	// Create provider based on configuration
	provider, err := factory(m.basePath, config)
	if err != nil {
		// For local provider, attempt to download model if missing
		if config.Provider == "local" {
			fmt.Printf("LLM model not found, attempting download...\n")
			if downloadErr := m.DownloadModel(config.ModelName, factory); downloadErr != nil {
				return fmt.Errorf("failed to download model: %w (original error: %v)", downloadErr, err)
			}
			// Try creating provider again after download
			provider, err = factory(m.basePath, config)
			if err != nil {
				return fmt.Errorf("failed to create provider after download: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create provider: %w", err)
		}
	}

	// Initialize the provider
	if err := provider.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize provider: %w", err)
	}

	m.provider = provider
	fmt.Printf("LLM provider initialized successfully (%s)\n", config.Provider)

	// Notify frontend
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "llm:ready", map[string]interface{}{
			"provider": config.Provider,
			"model":    config.ModelName,
		})
	}

	return nil
}

// Generate produces text using the configured provider
func (m *LLMManager) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("LLM provider not initialized")
	}

	if !m.provider.IsAvailable() {
		return nil, fmt.Errorf("LLM provider not available")
	}

	return m.provider.Generate(ctx, request)
}

// SwitchProvider switches to a different provider configuration
func (m *LLMManager) SwitchProvider(newConfig *domain.LLMConfig, factory ProviderFactory) error {
	// Cleanup old provider
	if m.provider != nil {
		m.provider.Cleanup()
		m.provider = nil
	}

	// Initialize new provider
	return m.Initialize(newConfig, factory)
}

// DownloadModel downloads a local LLM model
func (m *LLMManager) DownloadModel(modelName string, factory ProviderFactory) error {
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "llm:download:start", modelName)
	}

	// Get the models directory
	modelsPath := filepath.Join(m.basePath, "models", "llm")
	if err := os.MkdirAll(modelsPath, 0755); err != nil {
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "llm:download:error", "Failed to create models directory")
		}
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	// Define the script path
	scriptPath := filepath.Join(modelsPath, ".download-llm-model.sh")

	// Write the embedded script
	if err := os.WriteFile(scriptPath, m.downloadScript, 0755); err != nil {
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "llm:download:error", "Failed to write download script")
		}
		return fmt.Errorf("failed to write download script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Run the script
	cmd := exec.Command(scriptPath, modelName)
	cmd.Dir = modelsPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Model download script failed:\n%s\n", string(output))
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "llm:download:error", "Model download failed. Check logs.")
		}
		return fmt.Errorf("model download script failed: %w", err)
	}

	fmt.Printf("Model download script output:\n%s\n", string(output))
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "llm:download:finish", modelName)
	}

	return nil
}

// IsAvailable returns whether the LLM provider is available
func (m *LLMManager) IsAvailable() bool {
	return m.provider != nil && m.provider.IsAvailable()
}

// GetProvider returns the current provider
func (m *LLMManager) GetProvider() LLMProvider {
	return m.provider
}

// GetConfig returns the current configuration
func (m *LLMManager) GetConfig() *domain.LLMConfig {
	return m.config
}

// GetStatus returns the current status
func (m *LLMManager) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"available": m.IsAvailable(),
		"provider":  "",
		"model":     "",
	}

	if m.config != nil {
		status["provider"] = m.config.Provider
		status["model"] = m.config.ModelName
	}

	if m.provider != nil {
		status["info"] = m.provider.GetModelInfo()
	}

	return status
}

// Cleanup cleans up the LLM provider
func (m *LLMManager) Cleanup() {
	if m.provider != nil {
		m.provider.Cleanup()
		m.provider = nil
	}
}

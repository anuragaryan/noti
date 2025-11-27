package service

import (
	"context"
	"fmt"

	"noti/internal/domain"
	"noti/internal/llm/api"
	"noti/internal/llm/local"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LLMManager handles LLM service lifecycle and provider management
type LLMManager struct {
	basePath            string
	ctx                 context.Context
	provider            LLMProvider
	config              *domain.LLMConfig
	downloadScriptLLM   []byte
	downloadScriptLlama []byte
}

// NewLLMManager creates a new LLM manager
func NewLLMManager(basePath string, downloadScriptLLM, downloadScriptLlama []byte) *LLMManager {
	return &LLMManager{
		basePath:            basePath,
		downloadScriptLLM:   downloadScriptLLM,
		downloadScriptLlama: downloadScriptLlama,
	}
}

// SetContext sets the Wails runtime context
func (m *LLMManager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Initialize attempts to initialize the LLM provider
func (m *LLMManager) Initialize(config *domain.LLMConfig) error {
	if config == nil || config.Provider == "" {
		return fmt.Errorf("invalid LLM configuration")
	}

	m.config = config

	var provider LLMProvider
	var err error

	// Create provider based on configuration
	switch config.Provider {
	case "api":
		apiProvider, apiErr := api.NewProvider(config)
		if apiErr != nil {
			err = apiErr
		} else {
			provider = apiProvider
		}
	case "local":
		localProvider, localErr := local.NewProvider(m.basePath, config, m.downloadScriptLLM)
		if localErr != nil {
			err = localErr
		} else {
			// Set up server manager for local provider
			serverManager := local.NewServerManager(m.basePath, m.downloadScriptLlama)
			localProvider.SetServerManager(serverManager)
			provider = localProvider
		}
	default:
		return fmt.Errorf("unknown provider: %s", config.Provider)
	}

	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
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

// GenerateStream produces text using streaming
func (m *LLMManager) GenerateStream(ctx context.Context, request *domain.LLMRequest, callback domain.StreamCallback) error {
	if !m.SupportsStreaming() {
		return fmt.Errorf("streaming not available")
	}

	return m.provider.GenerateStream(ctx, request, callback)
}

// SupportsStreaming checks if current provider supports streaming
func (m *LLMManager) SupportsStreaming() bool {
	return m.provider != nil && m.provider.IsAvailable() && m.provider.SupportsStreaming()
}

// SwitchProvider switches to a different provider configuration
func (m *LLMManager) SwitchProvider(newConfig *domain.LLMConfig) error {
	// Cleanup old provider
	if m.provider != nil {
		m.provider.Cleanup()
		m.provider = nil
	}

	// Initialize new provider
	return m.Initialize(newConfig)
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

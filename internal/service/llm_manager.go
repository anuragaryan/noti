package service

import (
	"context"
	"fmt"
	"log/slog"

	"noti/internal/domain"
	"noti/internal/llm/api"
	"noti/internal/llm/local"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LLMManager handles LLM service lifecycle and provider management
type LLMManager struct {
	basePath string
	ctx      context.Context
	provider LLMProvider
	config   *domain.LLMConfig
}

// NewLLMManager creates a new LLM manager
func NewLLMManager(basePath string) *LLMManager {
	return &LLMManager{
		basePath: basePath,
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

	provider, err := m.buildProvider(config)
	if err != nil {
		return err
	}

	m.provider = provider
	m.config = config
	slog.Info("LLM provider initialized successfully", "provider", config.Provider)

	// Notify frontend
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "llm:ready", map[string]interface{}{
			"provider":  config.Provider,
			"modelName": config.ModelName,
			"model":     config.ModelName,
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
	if m.provider == nil {
		return fmt.Errorf("LLM provider not initialized")
	}

	if !m.provider.IsAvailable() {
		return fmt.Errorf("LLM provider not available")
	}

	if !m.provider.SupportsStreaming() {
		return fmt.Errorf("streaming not supported by current provider")
	}

	return m.provider.GenerateStream(ctx, request, callback)
}

// SupportsStreaming checks if current provider supports streaming
func (m *LLMManager) SupportsStreaming() bool {
	return m.provider != nil && m.provider.IsAvailable() && m.provider.SupportsStreaming()
}

// SwitchProvider switches to a different provider configuration
func (m *LLMManager) SwitchProvider(newConfig *domain.LLMConfig) error {
	if newConfig == nil || newConfig.Provider == "" {
		return fmt.Errorf("invalid LLM configuration")
	}

	oldProvider := m.provider
	oldConfig := m.config

	if oldProvider != nil {
		oldProvider.Cleanup()
	}
	m.provider = nil

	newProvider, err := m.buildProvider(newConfig)
	if err != nil {
		slog.Warn("Failed to switch LLM provider; attempting rollback", "error", err)

		if oldConfig != nil && oldConfig.Provider != "" {
			rollbackProvider, rollbackErr := m.buildProvider(oldConfig)
			if rollbackErr != nil {
				m.config = nil
				return fmt.Errorf("failed to switch LLM provider: %w (rollback failed: %v)", err, rollbackErr)
			}
			m.provider = rollbackProvider
			m.config = oldConfig
		} else {
			m.config = nil
		}

		return fmt.Errorf("failed to switch LLM provider: %w", err)
	}

	m.provider = newProvider
	m.config = newConfig
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

func (m *LLMManager) buildProvider(config *domain.LLMConfig) (LLMProvider, error) {
	var provider LLMProvider

	// Create provider based on configuration
	switch config.Provider {
	case "api":
		apiProvider, err := api.NewProvider(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider: %w", err)
		}
		provider = apiProvider
	case "local":
		localProvider, err := local.NewProvider(m.basePath, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider: %w", err)
		}

		// Set up server manager for local provider
		serverManager := local.NewServerManager(m.basePath)
		localProvider.SetServerManager(serverManager)
		provider = localProvider
	default:
		return nil, fmt.Errorf("unknown provider: %s", config.Provider)
	}

	// Initialize the provider. Context must be wired in before Initialize so any
	// model/download work performed during initialization can emit events.
	provider.SetContext(m.ctx)
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

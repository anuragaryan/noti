package main

import (
	"noti/internal/domain"
	"noti/internal/service"
)

// LocalLLMAdapter adapts LocalLLMProvider to service.LLMProvider interface
type LocalLLMAdapter struct {
	*LocalLLMProvider
}

// APILLMAdapter adapts APILLMProvider to service.LLMProvider interface
type APILLMAdapter struct {
	*APILLMProvider
}

// NewLLMProvider creates the appropriate LLM provider based on configuration
func NewLLMProvider(basePath string, config *domain.LLMConfig) (service.LLMProvider, error) {
	switch config.Provider {
	case "local":
		provider, err := NewLocalLLMProvider(basePath, config, downloadScriptLLM)
		if err != nil {
			return nil, err
		}

		// Create and set the llama-server manager
		serverManager := NewLlamaServerManager(basePath, downloadScriptLlamaServer)
		provider.SetServerManager(serverManager)

		return &LocalLLMAdapter{LocalLLMProvider: provider}, nil

	case "api":
		provider, err := NewAPILLMProvider(config)
		if err != nil {
			return nil, err
		}
		return &APILLMAdapter{APILLMProvider: provider}, nil

	default:
		return nil, nil // No provider configured
	}
}

package main

import (
	"context"
	"fmt"
	"sync"

	"noti/internal/domain"
)

// LocalLLMProvider implements LLM using local models
// This is a placeholder implementation - local LLM support is not currently available
type LocalLLMProvider struct {
	modelMutex sync.Mutex
	modelPath  string
	config     *domain.LLMConfig
	available  bool
}

// NewLocalLLMProvider creates a new local LLM provider
func NewLocalLLMProvider(basePath string, config *domain.LLMConfig) (*LocalLLMProvider, error) {
	return &LocalLLMProvider{
		modelPath: "",
		config:    config,
		available: false,
	}, nil
}

// Initialize loads the LLM model
func (p *LocalLLMProvider) Initialize() error {
	fmt.Println("=== Local LLM Provider ===")
	fmt.Println("⚠️  Local LLM support is not currently available")
	fmt.Println("⚠️  Please use the API provider instead")
	return fmt.Errorf("local LLM provider is not implemented")
}

// Generate produces text based on the request
func (p *LocalLLMProvider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	return nil, fmt.Errorf("local LLM provider is not implemented")
}

// IsAvailable returns whether the provider is ready to use
func (p *LocalLLMProvider) IsAvailable() bool {
	return false
}

// Cleanup releases resources
func (p *LocalLLMProvider) Cleanup() {
	fmt.Println("Local LLM provider cleanup (no-op)")
}

// GetModelInfo returns information about the current model
func (p *LocalLLMProvider) GetModelInfo() map[string]interface{} {
	return map[string]interface{}{
		"provider":  "local",
		"available": false,
		"status":    "not implemented",
		"message":   "Local LLM support is not currently available. Please use API provider.",
	}
}

package service

import (
	"context"

	"noti/internal/domain"
)

// LLMProvider defines the interface all LLM providers must implement
type LLMProvider interface {
	// Initialize sets up the provider (load model, validate API key, etc.)
	Initialize() error

	// Generate produces text based on the request
	Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error)

	// IsAvailable returns whether the provider is ready to use
	IsAvailable() bool

	// Cleanup releases resources
	Cleanup()

	// GetModelInfo returns information about the current model
	GetModelInfo() map[string]interface{}
}

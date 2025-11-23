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

// STTService defines the interface for STT operations
type STTService interface {
	// Initialize sets up the STT service
	Initialize() error

	// SetContext sets the Wails runtime context for events
	SetContext(ctx context.Context)

	// StartRecording begins audio capture
	StartRecording() error

	// StopRecording stops audio capture and returns the final transcription
	StopRecording() (*domain.TranscriptionResult, error)

	// IsRecording returns whether recording is in progress
	IsRecording() bool

	// Cleanup releases resources
	Cleanup()
}

package main

import (
	"context"

	"noti/internal/service"
)

// STTServiceAdapter adapts STTService to service.STTServiceInterface
type STTServiceAdapter struct {
	*STTService
}

// NewSTTServiceAdapter creates a new adapter
func NewSTTServiceAdapter(basePath string, chunkSeconds int, modelName string) (service.STTServiceInterface, error) {
	stt, err := NewSTTService(basePath, chunkSeconds, modelName)
	if err != nil {
		return nil, err
	}
	return &STTServiceAdapter{STTService: stt}, nil
}

// StopRecording adapts the return type
func (a *STTServiceAdapter) StopRecording() (interface{}, error) {
	return a.STTService.StopRecording()
}

// Initialize delegates to the underlying service
func (a *STTServiceAdapter) Initialize() error {
	return a.STTService.Initialize()
}

// SetContext delegates to the underlying service
func (a *STTServiceAdapter) SetContext(ctx context.Context) {
	a.STTService.SetContext(ctx)
}

// StartRecording delegates to the underlying service
func (a *STTServiceAdapter) StartRecording() error {
	return a.STTService.StartRecording()
}

// IsRecording delegates to the underlying service
func (a *STTServiceAdapter) IsRecording() bool {
	return a.STTService.IsRecording()
}

// Cleanup delegates to the underlying service
func (a *STTServiceAdapter) Cleanup() {
	a.STTService.Cleanup()
}

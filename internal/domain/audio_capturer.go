// Package domain contains core domain types and interfaces
package domain

import "context"

// AudioCapturer defines the interface for platform-specific audio capture
type AudioCapturer interface {
	// Initialize sets up the audio capturer
	Initialize() error

	// GetAvailableDevices returns all available audio devices for this capturer
	GetAvailableDevices() ([]AudioDevice, error)

	// CheckPermissions checks if necessary permissions are granted
	CheckPermissions() PermissionStatus

	// RequestPermissions prompts the user for necessary permissions
	RequestPermissions() error

	// StartCapture begins audio capture with the given configuration
	// The callback is invoked for each audio chunk captured
	StartCapture(ctx context.Context, config AudioCaptureConfig, callback AudioCallback) error

	// StopCapture stops audio capture
	StopCapture() error

	// IsCapturing returns whether capture is in progress
	IsCapturing() bool

	// GetSupportedSources returns which audio sources this capturer supports
	GetSupportedSources() []AudioSource

	// Cleanup releases resources
	Cleanup()
}

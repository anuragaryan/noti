//go:build darwin

// Package darwin provides macOS-specific audio capture using ScreenCaptureKit
package darwin

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreMedia -framework CoreGraphics -framework Foundation -framework AVFoundation -framework AudioToolbox

#include "system_capturer.h"
#include <stdlib.h>

// Forward declaration for Go callback
extern void goAudioCallback(float *data, int frameCount, int sampleRate, int channels);

// Bridge function that calls the Go callback
static void audioCallbackBridge(float *data, int frameCount, int sampleRate, int channels) {
    goAudioCallback(data, frameCount, sampleRate, channels);
}

// Wrapper to start capture with our bridge callback
static int startCaptureWithCallback(int sampleRate, int channels) {
    return SystemAudioCapturer_Start(sampleRate, channels, audioCallbackBridge);
}
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unsafe"

	"noti/internal/domain"
)

var (
	globalCallback domain.AudioCallback
	callbackMutex  sync.RWMutex
)

//export goAudioCallback
func goAudioCallback(data *C.float, frameCount C.int, sampleRate C.int, channels C.int) {
	callbackMutex.RLock()
	cb := globalCallback
	callbackMutex.RUnlock()

	if cb == nil {
		return
	}

	frames := int(frameCount)
	chans := int(channels)
	if frames <= 0 || chans <= 0 {
		return
	}

	// Total number of float32 values in the interleaved buffer
	totalSamples := frames * chans
	cSlice := (*[1 << 28]C.float)(unsafe.Pointer(data))[:totalSamples:totalSamples]

	// The transcriber requires mono audio. If the C layer passed through a
	// multi-channel buffer (e.g. config.Channels != 1), downmix here so the
	// chunk is never silently dropped by the transcriber's channel check.
	var goData []float32
	outChans := chans
	if chans > 1 {
		goData = make([]float32, frames)
		scale := float32(1.0 / float64(chans))
		for i := 0; i < frames; i++ {
			var sum float32
			for c := 0; c < chans; c++ {
				sum += float32(cSlice[i*chans+c])
			}
			goData[i] = sum * scale
		}
		outChans = 1
	} else {
		goData = make([]float32, totalSamples)
		for i := 0; i < totalSamples; i++ {
			goData[i] = float32(cSlice[i])
		}
	}

	chunk := domain.AudioChunk{
		Data:       goData,
		SampleRate: int(sampleRate),
		Channels:   outChans,
		Timestamp:  time.Now().UnixMilli(),
	}

	cb(chunk)
}

// SystemAudioCapturer implements AudioCapturer for macOS system audio using ScreenCaptureKit
type SystemAudioCapturer struct {
	initialized bool
	isCapturing bool
	mutex       sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewSystemAudioCapturer creates a new macOS system audio capturer
func NewSystemAudioCapturer() *SystemAudioCapturer {
	return &SystemAudioCapturer{}
}

// Initialize sets up the ScreenCaptureKit capturer
func (c *SystemAudioCapturer) Initialize() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.initialized {
		return nil
	}

	result := C.SystemAudioCapturer_Initialize()
	if result != 0 {
		return fmt.Errorf("failed to initialize system audio capturer: macOS 12.3+ required (error code: %d)", result)
	}

	c.initialized = true
	slog.Info("✓ System audio capturer initialized (ScreenCaptureKit)")

	return nil
}

// GetAvailableDevices returns available system audio sources
func (c *SystemAudioCapturer) GetAvailableDevices() ([]domain.AudioDevice, error) {
	if !c.initialized {
		return nil, fmt.Errorf("capturer not initialized")
	}

	// ScreenCaptureKit captures all system audio as a single source
	return []domain.AudioDevice{
		{
			ID:         "system-audio",
			Name:       "System Audio",
			Source:     domain.AudioSourceSystem,
			IsDefault:  true,
			SampleRate: 48000, // ScreenCaptureKit default
			Channels:   2,     // Stereo by default
		},
	}, nil
}

// CheckPermissions checks screen recording permission
func (c *SystemAudioCapturer) CheckPermissions() domain.PermissionStatus {
	if !c.initialized {
		return domain.PermissionUnknown
	}

	status := C.SystemAudioCapturer_CheckPermission()
	switch status {
	case 1:
		return domain.PermissionGranted
	case 2:
		return domain.PermissionDenied
	default:
		return domain.PermissionUnknown
	}
}

// RequestPermissions prompts for screen recording permission
func (c *SystemAudioCapturer) RequestPermissions() error {
	if !c.initialized {
		return fmt.Errorf("capturer not initialized")
	}

	C.SystemAudioCapturer_RequestPermission()
	return nil
}

// StartCapture begins system audio capture
func (c *SystemAudioCapturer) StartCapture(ctx context.Context, config domain.AudioCaptureConfig, callback domain.AudioCallback) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.initialized {
		return fmt.Errorf("capturer not initialized")
	}

	if c.isCapturing {
		return fmt.Errorf("already capturing")
	}

	// Check permissions first
	if c.CheckPermissions() != domain.PermissionGranted {
		return fmt.Errorf("screen recording permission not granted - required for system audio capture. Please grant permission in System Settings > Privacy & Security > Screen Recording")
	}

	slog.Info("\n=== Starting System Audio Capture ===")
	slog.Info("Audio config", "sampleRate", config.SampleRate, "channels", config.Channels)

	// Set global callback
	callbackMutex.Lock()
	globalCallback = callback
	callbackMutex.Unlock()

	// Start capture via C bridge
	result := C.startCaptureWithCallback(C.int(config.SampleRate), C.int(config.Channels))
	if result != 0 {
		callbackMutex.Lock()
		globalCallback = nil
		callbackMutex.Unlock()

		var errMsg string
		switch result {
		case -1:
			errMsg = "capturer not initialized or macOS version too old"
		case -2:
			errMsg = "already capturing"
		case -3:
			errMsg = "failed to get shareable content - check screen recording permission"
		case -4:
			errMsg = "no displays found"
		case -5:
			errMsg = "failed to add stream output"
		case -6:
			errMsg = "failed to start capture"
		default:
			errMsg = fmt.Sprintf("unknown error code: %d", result)
		}
		return fmt.Errorf("failed to start system audio capture: %s", errMsg)
	}

	c.isCapturing = true
	c.ctx, c.cancel = context.WithCancel(ctx)

	slog.Info("✓ System audio capture started successfully!")

	// Monitor context cancellation
	go func() {
		<-c.ctx.Done()
		c.StopCapture()
	}()

	return nil
}

// StopCapture stops system audio capture
func (c *SystemAudioCapturer) StopCapture() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isCapturing {
		return nil
	}

	slog.Info("\n=== Stopping System Audio Capture ===")

	C.SystemAudioCapturer_Stop()

	callbackMutex.Lock()
	globalCallback = nil
	callbackMutex.Unlock()

	c.isCapturing = false
	if c.cancel != nil {
		c.cancel()
	}

	slog.Info("✓ System audio capture stopped")

	return nil
}

// IsCapturing returns whether capture is in progress
func (c *SystemAudioCapturer) IsCapturing() bool {
	return C.SystemAudioCapturer_IsCapturing() == 1
}

// GetSupportedSources returns supported audio sources
func (c *SystemAudioCapturer) GetSupportedSources() []domain.AudioSource {
	return []domain.AudioSource{domain.AudioSourceSystem}
}

// Cleanup releases resources
func (c *SystemAudioCapturer) Cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isCapturing {
		c.mutex.Unlock()
		c.StopCapture()
		c.mutex.Lock()
	}

	if c.initialized {
		slog.Info("Cleaning up system audio capturer...")
		C.SystemAudioCapturer_Cleanup()
		c.initialized = false
	}
}

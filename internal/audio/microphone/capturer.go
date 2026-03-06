// Package microphone provides microphone audio capture using PortAudio
package microphone

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"noti/internal/domain"

	"github.com/gordonklaus/portaudio"
)

// Capturer implements AudioCapturer for microphone input using PortAudio
type Capturer struct {
	initialized     bool
	isCapturing     bool
	stream          *portaudio.Stream
	stopCapture     chan struct{}
	mutex           sync.Mutex
	callback        domain.AudioCallback
	framesPerBuffer []float32
	config          domain.AudioCaptureConfig
}

// NewCapturer creates a new microphone capturer
func NewCapturer() *Capturer {
	return &Capturer{}
}

// Initialize sets up PortAudio for microphone capture
func (c *Capturer) Initialize() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.initialized {
		return nil
	}

	slog.Info("=== Initializing Microphone Capturer ===")
	slog.Info("Initializing PortAudio...")

	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize portaudio: %w", err)
	}

	// Check for input devices
	devices, err := portaudio.Devices()
	if err != nil {
		portaudio.Terminate()
		return fmt.Errorf("failed to get audio devices: %w", err)
	}

	hasInput := false
	for _, device := range devices {
		if device.MaxInputChannels > 0 {
			hasInput = true
			break
		}
	}

	if !hasInput {
		portaudio.Terminate()
		return fmt.Errorf("no input devices found - please check microphone connection and permissions")
	}

	// Check default input device
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		portaudio.Terminate()
		return fmt.Errorf("no default input device: %w - please set a default microphone in system settings", err)
	}
	slog.Info("Default Input Device", "name", defaultInput.Name)

	c.initialized = true
	slog.Info("✓ Microphone capturer initialized successfully!")

	return nil
}

// GetAvailableDevices returns all available microphone devices
func (c *Capturer) GetAvailableDevices() ([]domain.AudioDevice, error) {
	if !c.initialized {
		return nil, fmt.Errorf("capturer not initialized")
	}

	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("failed to get audio devices: %w", err)
	}

	defaultInput, _ := portaudio.DefaultInputDevice()

	var audioDevices []domain.AudioDevice
	for i, device := range devices {
		if device.MaxInputChannels > 0 {
			isDefault := defaultInput != nil && device.Name == defaultInput.Name
			audioDevices = append(audioDevices, domain.AudioDevice{
				ID:         fmt.Sprintf("mic-%d", i),
				Name:       device.Name,
				Source:     domain.AudioSourceMicrophone,
				IsDefault:  isDefault,
				SampleRate: int(device.DefaultSampleRate),
				Channels:   device.MaxInputChannels,
			})
		}
	}

	return audioDevices, nil
}

// CheckPermissions checks if microphone permissions are granted
func (c *Capturer) CheckPermissions() domain.PermissionStatus {
	// On macOS, we can check by trying to access the default input device
	// If it fails, permissions are likely denied
	if !c.initialized {
		return domain.PermissionUnknown
	}

	_, err := portaudio.DefaultInputDevice()
	if err != nil {
		return domain.PermissionDenied
	}

	return domain.PermissionGranted
}

// RequestPermissions prompts the user for microphone permissions
func (c *Capturer) RequestPermissions() error {
	// On macOS, permissions are requested automatically when we try to access the microphone
	// The system will show a permission dialog
	// We can trigger this by trying to open a stream
	if !c.initialized {
		return fmt.Errorf("capturer not initialized")
	}

	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("no default input device: %w", err)
	}

	// Try to open a stream briefly to trigger permission request
	buffer := make([]float32, 1024)
	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   defaultInput,
			Channels: 1,
			Latency:  defaultInput.DefaultLowInputLatency,
		},
		SampleRate:      16000,
		FramesPerBuffer: len(buffer),
	}

	stream, err := portaudio.OpenStream(streamParams, buffer)
	if err != nil {
		return fmt.Errorf("failed to open stream (permission denied?): %w", err)
	}
	stream.Close()

	return nil
}

// StartCapture begins audio capture from the microphone
func (c *Capturer) StartCapture(ctx context.Context, config domain.AudioCaptureConfig, callback domain.AudioCallback) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.initialized {
		return fmt.Errorf("capturer not initialized")
	}

	if c.isCapturing {
		return fmt.Errorf("already capturing")
	}

	slog.Info("\n=== Starting Microphone Capture ===")

	// Get the input device
	var inputDevice *portaudio.DeviceInfo
	var err error

	if config.DeviceID != "" {
		// Find specific device by ID
		devices, err := portaudio.Devices()
		if err != nil {
			return fmt.Errorf("failed to get devices: %w", err)
		}
		for i, device := range devices {
			if fmt.Sprintf("mic-%d", i) == config.DeviceID && device.MaxInputChannels > 0 {
				inputDevice = device
				break
			}
		}
		if inputDevice == nil {
			return fmt.Errorf("device not found: %s", config.DeviceID)
		}
	} else {
		// Use default input device
		inputDevice, err = portaudio.DefaultInputDevice()
		if err != nil {
			return fmt.Errorf("no default input device found: %w", err)
		}
	}

	slog.Info("Using microphone", "name", inputDevice.Name)
	slog.Info("Audio config", "sampleRate", config.SampleRate, "channels", config.Channels)

	// Create buffer
	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	c.framesPerBuffer = make([]float32, bufferSize)
	c.config = config
	c.callback = callback

	// Create stream parameters
	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   inputDevice,
			Channels: config.Channels,
			Latency:  inputDevice.DefaultLowInputLatency,
		},
		SampleRate:      float64(config.SampleRate),
		FramesPerBuffer: bufferSize,
	}

	// Open audio stream
	slog.Info("Opening audio stream...")
	stream, err := portaudio.OpenStream(streamParams, c.framesPerBuffer)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %w", err)
	}

	c.stream = stream

	// Start the stream
	slog.Info("Starting audio capture...")
	if err := stream.Start(); err != nil {
		stream.Close()
		return fmt.Errorf("failed to start audio stream: %w", err)
	}

	c.isCapturing = true
	c.stopCapture = make(chan struct{})

	slog.Info("✓ Microphone capture started successfully!")

	// Start capture goroutine
	go c.captureLoop(ctx)

	return nil
}

// captureLoop continuously captures audio from the microphone
func (c *Capturer) captureLoop(ctx context.Context) {
	for {
		select {
		case <-c.stopCapture:
			return
		case <-ctx.Done():
			c.StopCapture()
			return
		default:
			// Read from stream
			if err := c.stream.Read(); err != nil {
				slog.Error("Error reading from stream", "error", err)
				return
			}

			// Create audio chunk and send to callback
			if c.callback != nil {
				// Copy the buffer to avoid race conditions
				data := make([]float32, len(c.framesPerBuffer))
				copy(data, c.framesPerBuffer)

				chunk := domain.AudioChunk{
					Data:       data,
					SampleRate: c.config.SampleRate,
					Channels:   c.config.Channels,
					Timestamp:  time.Now().UnixMilli(),
				}
				c.callback(chunk)
			}
		}
	}
}

// StopCapture stops audio capture
func (c *Capturer) StopCapture() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isCapturing {
		return nil
	}

	slog.Info("\n=== Stopping Microphone Capture ===")

	// Signal to stop capture
	close(c.stopCapture)

	// Give a moment for capture to stop
	time.Sleep(50 * time.Millisecond)

	// Stop and close the stream
	if c.stream != nil {
		c.stream.Stop()
		c.stream.Close()
		c.stream = nil
	}

	c.isCapturing = false
	c.callback = nil

	slog.Info("✓ Microphone capture stopped")

	return nil
}

// IsCapturing returns whether capture is in progress
func (c *Capturer) IsCapturing() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.isCapturing
}

// GetSupportedSources returns supported audio sources
func (c *Capturer) GetSupportedSources() []domain.AudioSource {
	return []domain.AudioSource{domain.AudioSourceMicrophone}
}

// Cleanup releases resources
func (c *Capturer) Cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isCapturing {
		c.mutex.Unlock()
		c.StopCapture()
		c.mutex.Lock()
	}

	if c.initialized {
		slog.Info("Cleaning up microphone capturer...")
		portaudio.Terminate()
		c.initialized = false
	}
}

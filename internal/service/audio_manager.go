// Package service provides application services
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"noti/internal/audio"
	"noti/internal/domain"
)

// AudioRingBuffer is a thread-safe ring buffer for audio samples
type AudioRingBuffer struct {
	data     []float32
	size     int
	writePos int
	readPos  int
	count    int
	mutex    sync.Mutex
}

// NewAudioRingBuffer creates a new ring buffer with the specified size
func NewAudioRingBuffer(size int) *AudioRingBuffer {
	return &AudioRingBuffer{
		data: make([]float32, size),
		size: size,
	}
}

// Write adds samples to the buffer
func (b *AudioRingBuffer) Write(samples []float32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, sample := range samples {
		b.data[b.writePos] = sample
		b.writePos = (b.writePos + 1) % b.size
		if b.count < b.size {
			b.count++
		} else {
			// Buffer is full, advance read position
			b.readPos = (b.readPos + 1) % b.size
		}
	}
}

// Read retrieves samples from the buffer
func (b *AudioRingBuffer) Read(count int) []float32 {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if count > b.count {
		count = b.count
	}

	result := make([]float32, count)
	for i := 0; i < count; i++ {
		result[i] = b.data[b.readPos]
		b.readPos = (b.readPos + 1) % b.size
		b.count--
	}
	return result
}

// Available returns the number of samples available to read
func (b *AudioRingBuffer) Available() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.count
}

// Clear resets the buffer
func (b *AudioRingBuffer) Clear() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.writePos = 0
	b.readPos = 0
	b.count = 0
}

// AudioManager manages audio capture across different sources
type AudioManager struct {
	micCapturer    domain.AudioCapturer
	systemCapturer domain.AudioCapturer
	activeCapturer domain.AudioCapturer
	activeSource   domain.AudioSource
	mixerConfig    domain.AudioMixerConfig

	// For mixed mode
	micBuffer    *AudioRingBuffer
	sysBuffer    *AudioRingBuffer
	mixerRunning bool
	mixerStop    chan struct{}
	mixerWg      sync.WaitGroup

	mutex sync.RWMutex
	ctx   context.Context
}

type mixSyncState struct {
	micStarvedTicks int
	sysStarvedTicks int
	graceTicks      int
	lastMicPartial  int
	lastSysPartial  int
	micStalledTicks int
	sysStalledTicks int
}

func updateStalledPartial(avail int, lastPartial *int, stalledTicks *int) {
	if avail <= 0 {
		*lastPartial = 0
		*stalledTicks = 0
		return
	}

	if avail == *lastPartial {
		*stalledTicks++
		return
	}

	*lastPartial = avail
	*stalledTicks = 1
}

// NewAudioManager creates a new audio manager
func NewAudioManager() *AudioManager {
	return &AudioManager{
		activeSource: domain.AudioSourceMicrophone,
		mixerConfig:  domain.DefaultMixerConfig(),
	}
}

// Initialize sets up available audio capturers
func (m *AudioManager) Initialize() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	slog.Info("Initializing Audio Manager")

	// Initialize microphone capturer - available on all platforms via PortAudio
	micCapturer := audio.NewMicrophoneCapturer()
	if err := micCapturer.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize microphone capturer: %w", err)
	}
	m.micCapturer = micCapturer
	m.activeCapturer = micCapturer // Default to microphone

	// Initialize platform-specific system audio capturer
	systemCapturer := audio.NewSystemAudioCapturer()
	if systemCapturer != nil {
		if err := systemCapturer.Initialize(); err != nil {
			// System audio not available, but mic still works
			slog.Warn("System audio capture not available", "error", err)
		} else {
			m.systemCapturer = systemCapturer
			slog.Info("System audio capture available")
		}
	} else {
		slog.Info("System audio capture not supported on this platform")
	}

	slog.Info("Audio Manager initialized")

	return nil
}

// SetContext sets the application context
func (m *AudioManager) SetContext(ctx context.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ctx = ctx
}

// GetAvailableSources returns which audio sources are available
func (m *AudioManager) GetAvailableSources() []domain.AudioSource {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sources := []domain.AudioSource{domain.AudioSourceMicrophone}

	if m.systemCapturer != nil {
		sources = append(sources, domain.AudioSourceSystem)
		// Mixed mode available when both mic and system are available
		sources = append(sources, domain.AudioSourceMixed)
	}

	return sources
}

// GetAvailableDevices returns all available audio devices
func (m *AudioManager) GetAvailableDevices() ([]domain.AudioDevice, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var devices []domain.AudioDevice

	if m.micCapturer != nil {
		micDevices, err := m.micCapturer.GetAvailableDevices()
		if err == nil {
			devices = append(devices, micDevices...)
		}
	}

	if m.systemCapturer != nil {
		sysDevices, err := m.systemCapturer.GetAvailableDevices()
		if err == nil {
			devices = append(devices, sysDevices...)
		}
	}

	return devices, nil
}

// SetAudioSource sets the active audio source
func (m *AudioManager) SetAudioSource(source domain.AudioSource) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	switch source {
	case domain.AudioSourceMicrophone:
		if m.micCapturer == nil {
			return fmt.Errorf("microphone capturer not available")
		}
		m.activeCapturer = m.micCapturer
	case domain.AudioSourceSystem:
		if m.systemCapturer == nil {
			return fmt.Errorf("system audio capturer not available")
		}
		m.activeCapturer = m.systemCapturer
	case domain.AudioSourceMixed:
		if m.micCapturer == nil {
			return fmt.Errorf("microphone capturer not available")
		}
		if m.systemCapturer == nil {
			return fmt.Errorf("system audio capturer not available")
		}
		// Mixed mode uses both capturers
		m.activeCapturer = nil // Will use special mixed mode handling
	default:
		return fmt.Errorf("unsupported audio source: %v", source)
	}

	m.activeSource = source
	slog.Info("Audio source set", "source", source.String())
	return nil
}

// GetActiveSource returns the currently active audio source
func (m *AudioManager) GetActiveSource() domain.AudioSource {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.activeSource
}

// SetMixerConfig updates the audio mixer configuration
func (m *AudioManager) SetMixerConfig(config domain.AudioMixerConfig) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.mixerConfig = config
	slog.Info("Mixer config updated", "micGain", config.MicrophoneGain, "systemGain", config.SystemGain, "mode", config.MixMode)
}

// GetMixerConfig returns the current mixer configuration
func (m *AudioManager) GetMixerConfig() domain.AudioMixerConfig {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.mixerConfig
}

// CheckPermissions checks permissions for the specified source
func (m *AudioManager) CheckPermissions(source domain.AudioSource) domain.PermissionStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	switch source {
	case domain.AudioSourceMicrophone:
		if m.micCapturer != nil {
			return m.micCapturer.CheckPermissions()
		}
	case domain.AudioSourceSystem:
		if m.systemCapturer != nil {
			return m.systemCapturer.CheckPermissions()
		}
	case domain.AudioSourceMixed:
		// For mixed mode, check both
		micPerm := domain.PermissionUnknown
		sysPerm := domain.PermissionUnknown
		if m.micCapturer != nil {
			micPerm = m.micCapturer.CheckPermissions()
		}
		if m.systemCapturer != nil {
			sysPerm = m.systemCapturer.CheckPermissions()
		}
		// Return the most restrictive permission
		if micPerm == domain.PermissionDenied || sysPerm == domain.PermissionDenied {
			return domain.PermissionDenied
		}
		if micPerm == domain.PermissionGranted && sysPerm == domain.PermissionGranted {
			return domain.PermissionGranted
		}
		return domain.PermissionUnknown
	}
	return domain.PermissionUnknown
}

// RequestPermissions requests permissions for the specified source
func (m *AudioManager) RequestPermissions(source domain.AudioSource) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	switch source {
	case domain.AudioSourceMicrophone:
		if m.micCapturer != nil {
			return m.micCapturer.RequestPermissions()
		}
	case domain.AudioSourceSystem:
		if m.systemCapturer != nil {
			return m.systemCapturer.RequestPermissions()
		}
	case domain.AudioSourceMixed:
		// Request both permissions
		if m.micCapturer != nil {
			if err := m.micCapturer.RequestPermissions(); err != nil {
				return fmt.Errorf("microphone permission: %w", err)
			}
		}
		if m.systemCapturer != nil {
			if err := m.systemCapturer.RequestPermissions(); err != nil {
				return fmt.Errorf("system audio permission: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("capturer not available for source: %v", source)
}

// StartCapture starts audio capture with the active source
func (m *AudioManager) StartCapture(config domain.AudioCaptureConfig, callback domain.AudioCallback) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	slog.Info("Starting audio capture", "source", m.activeSource.String(), "sampleRate", config.SampleRate, "channels", config.Channels, "bufferSize", config.BufferSize)

	if m.activeSource == domain.AudioSourceMixed {
		return m.startMixedCapture(config, callback)
	}

	if m.activeCapturer == nil {
		slog.Error("No active audio capturer")
		return fmt.Errorf("no active audio capturer")
	}

	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if err := m.activeCapturer.StartCapture(ctx, config, callback); err != nil {
		slog.Error("Failed to start audio capture", "source", m.activeSource.String(), "error", err)
		return err
	}

	return nil
}

// startMixedCapture starts capturing from both mic and system audio
func (m *AudioManager) startMixedCapture(config domain.AudioCaptureConfig, callback domain.AudioCallback) error {
	slog.Info("Starting Mixed Audio Capture")

	// Initialize ring buffers for mixing (2 seconds of audio)
	bufferSize := config.SampleRate * 2
	m.micBuffer = NewAudioRingBuffer(bufferSize)
	m.sysBuffer = NewAudioRingBuffer(bufferSize)
	m.mixerStop = make(chan struct{})

	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Start microphone capture
	micConfig := config
	micConfig.Source = domain.AudioSourceMicrophone
	err := m.micCapturer.StartCapture(ctx, micConfig, func(chunk domain.AudioChunk) {
		// Apply gain and write to buffer
		gain := m.GetMixerConfig().MicrophoneGain
		for i := range chunk.Data {
			chunk.Data[i] *= gain
		}
		m.micBuffer.Write(chunk.Data)
	})
	if err != nil {
		slog.Error("Failed to start microphone capture in mixed mode", "error", err)
		return fmt.Errorf("failed to start microphone capture: %w", err)
	}

	// Start system audio capture
	sysConfig := config
	sysConfig.Source = domain.AudioSourceSystem
	err = m.systemCapturer.StartCapture(ctx, sysConfig, func(chunk domain.AudioChunk) {
		// Apply gain and write to buffer
		gain := m.GetMixerConfig().SystemGain
		for i := range chunk.Data {
			chunk.Data[i] *= gain
		}
		m.sysBuffer.Write(chunk.Data)
	})
	if err != nil {
		m.micCapturer.StopCapture()
		slog.Error("Failed to start system capture in mixed mode", "error", err)
		return fmt.Errorf("failed to start system audio capture: %w", err)
	}

	// Start mixer goroutine
	m.mixerRunning = true
	m.mixerWg.Add(1)
	go m.runMixer(config, callback)

	slog.Info("Mixed audio capture started")

	return nil
}

// runMixer mixes audio from both sources and sends to callback
func (m *AudioManager) runMixer(config domain.AudioCaptureConfig, callback domain.AudioCallback) {
	defer m.mixerWg.Done()

	// Mix at regular intervals (100ms chunks)
	chunkSize := config.SampleRate / 10
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	state := mixSyncState{graceTicks: 2}

	for {
		select {
		case <-m.mixerStop:
			return
		case <-ticker.C:
			mixed, ok := m.mixNextChunk(chunkSize, &state)
			if !ok {
				continue
			}

			// Send mixed audio to callback
			callback(domain.AudioChunk{
				Data:       mixed,
				SampleRate: config.SampleRate,
				Channels:   config.Channels,
				Timestamp:  time.Now().UnixMilli(),
			})
		}
	}
}

func (m *AudioManager) mixNextChunk(chunkSize int, state *mixSyncState) ([]float32, bool) {
	micAvail := m.micBuffer.Available()
	sysAvail := m.sysBuffer.Available()

	if micAvail < chunkSize && sysAvail < chunkSize {
		return nil, false
	}

	if micAvail >= chunkSize && sysAvail >= chunkSize {
		state.micStarvedTicks = 0
		state.sysStarvedTicks = 0
		state.lastMicPartial = 0
		state.lastSysPartial = 0
		state.micStalledTicks = 0
		state.sysStalledTicks = 0
		micSamples := m.micBuffer.Read(chunkSize)
		sysSamples := m.sysBuffer.Read(chunkSize)
		return m.mixSamples(chunkSize, micSamples, sysSamples), true
	}

	if micAvail >= chunkSize {
		droppedSysPartial := 0
		state.sysStarvedTicks++
		state.micStarvedTicks = 0
		state.lastMicPartial = 0
		state.micStalledTicks = 0
		updateStalledPartial(sysAvail, &state.lastSysPartial, &state.sysStalledTicks)

		// Keep streams aligned under normal callback jitter.
		if sysAvail == 0 && state.sysStarvedTicks < state.graceTicks {
			return nil, false
		}
		if sysAvail > 0 && state.sysStalledTicks < state.graceTicks {
			return nil, false
		}

		if sysAvail > 0 {
			droppedSysPartial = sysAvail
			m.sysBuffer.Read(sysAvail)
			state.lastSysPartial = 0
			state.sysStalledTicks = 0
		}

		micSamples := m.micBuffer.Read(chunkSize)
		slog.Debug("Mixed audio chunk emitted with temporary system silence", "micSamples", len(micSamples), "sysSamples", 0, "chunkSize", chunkSize, "droppedSysPartialSamples", droppedSysPartial)
		return m.mixSamples(chunkSize, micSamples, nil), true
	}

	if sysAvail >= chunkSize {
		droppedMicPartial := 0
		state.micStarvedTicks++
		state.sysStarvedTicks = 0
		state.lastSysPartial = 0
		state.sysStalledTicks = 0
		updateStalledPartial(micAvail, &state.lastMicPartial, &state.micStalledTicks)

		if micAvail == 0 && state.micStarvedTicks < state.graceTicks {
			return nil, false
		}
		if micAvail > 0 && state.micStalledTicks < state.graceTicks {
			return nil, false
		}

		if micAvail > 0 {
			droppedMicPartial = micAvail
			m.micBuffer.Read(micAvail)
			state.lastMicPartial = 0
			state.micStalledTicks = 0
		}

		sysSamples := m.sysBuffer.Read(chunkSize)
		slog.Debug("Mixed audio chunk emitted with temporary microphone silence", "micSamples", 0, "sysSamples", len(sysSamples), "chunkSize", chunkSize, "droppedMicPartialSamples", droppedMicPartial)
		return m.mixSamples(chunkSize, nil, sysSamples), true
	}

	return nil, false
}

func (m *AudioManager) mixSamples(chunkSize int, micSamples []float32, sysSamples []float32) []float32 {
	mixed := make([]float32, chunkSize)
	m.mutex.RLock()
	mixMode := m.mixerConfig.MixMode
	m.mutex.RUnlock()

	for i := 0; i < chunkSize; i++ {
		micVal := float32(0)
		sysVal := float32(0)
		if i < len(micSamples) {
			micVal = micSamples[i]
		}
		if i < len(sysSamples) {
			sysVal = sysSamples[i]
		}

		switch mixMode {
		case "average":
			mixed[i] = (micVal + sysVal) / 2.0
		default: // "sum"
			mixed[i] = micVal + sysVal
			if mixed[i] > 1.0 {
				mixed[i] = 1.0 - (1.0 / (mixed[i] + 1.0))
			} else if mixed[i] < -1.0 {
				mixed[i] = -1.0 + (1.0 / (-mixed[i] + 1.0))
			}
		}
	}

	return mixed
}

// StopCapture stops the active audio capture
func (m *AudioManager) StopCapture() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.activeSource == domain.AudioSourceMixed {
		return m.stopMixedCapture()
	}

	if m.activeCapturer == nil {
		return nil
	}

	return m.activeCapturer.StopCapture()
}

// stopMixedCapture stops the mixed audio capture
func (m *AudioManager) stopMixedCapture() error {
	slog.Info("Stopping Mixed Audio Capture")

	if m.mixerRunning {
		close(m.mixerStop)
		m.mixerWg.Wait()
		m.mixerRunning = false
	}

	var errs []error
	if m.micCapturer != nil {
		if err := m.micCapturer.StopCapture(); err != nil {
			errs = append(errs, fmt.Errorf("mic: %w", err))
		}
	}
	if m.systemCapturer != nil {
		if err := m.systemCapturer.StopCapture(); err != nil {
			errs = append(errs, fmt.Errorf("system: %w", err))
		}
	}

	// Clear buffers
	if m.micBuffer != nil {
		m.micBuffer.Clear()
	}
	if m.sysBuffer != nil {
		m.sysBuffer.Clear()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping capture: %v", errs)
	}

	slog.Info("Mixed audio capture stopped")
	return nil
}

// IsCapturing returns whether capture is in progress
func (m *AudioManager) IsCapturing() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.activeSource == domain.AudioSourceMixed {
		return m.mixerRunning
	}

	if m.activeCapturer == nil {
		return false
	}

	return m.activeCapturer.IsCapturing()
}

// Cleanup releases all resources
func (m *AudioManager) Cleanup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	slog.Info("Cleaning up Audio Manager...")

	// Stop any active capture
	if m.mixerRunning {
		m.mutex.Unlock()
		m.stopMixedCapture()
		m.mutex.Lock()
	}

	if m.micCapturer != nil {
		m.micCapturer.Cleanup()
	}
	if m.systemCapturer != nil {
		m.systemCapturer.Cleanup()
	}

	slog.Info("Audio Manager cleanup complete")
}

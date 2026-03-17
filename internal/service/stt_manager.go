package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"noti/internal/domain"
	"noti/internal/events"
	"noti/internal/infrastructure/downloader"
	"noti/internal/stt/whisper"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// STTManager handles STT service lifecycle and model management
type STTManager struct {
	basePath     string
	ctx          context.Context
	transcriber  *whisper.Transcriber
	audioManager *AudioManager
	activeSource domain.AudioSource
}

// NewSTTManager creates a new STT manager
func NewSTTManager(basePath string) *STTManager {
	return &STTManager{
		basePath:     basePath,
		activeSource: domain.AudioSourceMicrophone,
	}
}

// SetContext sets the Wails runtime context
func (m *STTManager) SetContext(ctx context.Context) {
	m.ctx = ctx
	if m.audioManager != nil {
		m.audioManager.SetContext(ctx)
	}
}

// SetAudioManager sets the audio manager for audio capture
func (m *STTManager) SetAudioManager(audioManager *AudioManager) {
	m.audioManager = audioManager
}

// Initialize attempts to initialize the STT service with self-healing
func (m *STTManager) Initialize(config *domain.STTConfig) error {
	// Helper function to attempt STT initialization
	tryInitialize := func() bool {
		// Try to create transcriber
		transcriber, err := whisper.NewTranscriber(m.basePath, config)
		if err != nil {
			// This handles file-not-found
			return false
		}
		if err := transcriber.Initialize(); err != nil {
			// This handles model loading/corruption errors
			slog.Error("Failed to initialize STT model", "error", err)
			return false
		}
		// Success
		transcriber.SetContext(m.ctx)
		m.transcriber = transcriber

		slog.Info("STT service initialized successfully.")
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "stt:ready")
		}
		return true
	}

	if !tryInitialize() {
		// Initialization failed, likely because the model is missing or corrupt
		slog.Warn("STT initialization failed. Attempting to download or re-download model...")

		// Delete the potentially corrupt model file before downloading
		modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
		modelPath := filepath.Join(m.basePath, "models", "stt", modelFileName)
		if _, err := os.Stat(modelPath); err == nil {
			slog.Info("Deleting existing model file to ensure a clean download", "path", modelPath)
			os.Remove(modelPath)
		}

		// Download model and try to initialize again
		if err := m.DownloadModel(config); err != nil {
			slog.Error("Model download and initialization failed", "error", err)
			slog.Warn("Speech-to-text features will be disabled.")
			return err
		}
	}

	return nil
}

// DownloadModel downloads a Whisper model and initializes the STT service
func (m *STTManager) DownloadModel(config *domain.STTConfig) error {
	modelsPath := filepath.Join(m.basePath, "models", "stt")

	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	downloadID := fmt.Sprintf("stt:%s", config.ModelName)
	events.EmitDownloadEvent(m.ctx, events.DownloadEvent{
		ID:     downloadID,
		Kind:   events.DownloadKindSTTModel,
		Label:  config.ModelName,
		Status: events.DownloadStatusQueued,
	})

	opts := &downloader.DownloadOptions{
		DestDir: modelsPath,
		ProgressFunc: func(downloaded, total int64) {
			events.EmitDownloadEvent(m.ctx, events.DownloadEvent{
				ID:              downloadID,
				Kind:            events.DownloadKindSTTModel,
				Label:           config.ModelName,
				Status:          events.DownloadStatusDownloading,
				BytesDownloaded: downloaded,
				TotalBytes:      total,
				Percent:         events.CalculatePercent(downloaded, total),
			})
		},
	}

	slog.Info("Downloading Whisper model", "model", config.ModelName)
	if err := downloader.DownloadModel(ctx, config.ModelName, opts); err != nil {
		slog.Error("Model download failed", "error", err)
		events.EmitDownloadEvent(m.ctx, events.DownloadEvent{
			ID:     downloadID,
			Kind:   events.DownloadKindSTTModel,
			Label:  config.ModelName,
			Status: events.DownloadStatusError,
			Error:  "Model download failed. Check logs.",
		})
		return fmt.Errorf("model download failed: %w", err)
	}

	slog.Info("Model download complete", "model", config.ModelName)
	modelFile := filepath.Join(modelsPath, fmt.Sprintf("ggml-%s.bin", config.ModelName))
	var size int64
	if info, statErr := os.Stat(modelFile); statErr == nil {
		size = info.Size()
	}
	events.EmitDownloadEvent(m.ctx, events.DownloadEvent{
		ID:              downloadID,
		Kind:            events.DownloadKindSTTModel,
		Label:           config.ModelName,
		Status:          events.DownloadStatusCompleted,
		BytesDownloaded: size,
		TotalBytes:      size,
		Percent:         100,
	})

	// After a successful download, re-initialize the STT service
	slog.Info("Re-initializing STT service after model download...")
	transcriber, err := whisper.NewTranscriber(m.basePath, config)
	if err != nil {
		slog.Warn("STT service initialization failed after download", "error", err)
		return err
	}

	if err := transcriber.Initialize(); err != nil {
		slog.Warn("Failed to load STT model after download", "error", err)
		return err
	}

	transcriber.SetContext(m.ctx)
	m.transcriber = transcriber
	slog.Info("STT service initialized successfully after download.")

	// Notify the frontend that the service is now ready
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "stt:ready")
	}

	return nil
}

// GetService returns the underlying STT service
func (m *STTManager) GetService() *whisper.Transcriber {
	return m.transcriber
}

// GetTranscriber returns the transcriber
func (m *STTManager) GetTranscriber() *whisper.Transcriber {
	return m.transcriber
}

// IsAvailable returns whether the STT service is available
func (m *STTManager) IsAvailable() bool {
	return m.transcriber != nil
}

// SetAudioSource sets the audio source for recording
func (m *STTManager) SetAudioSource(source domain.AudioSource) error {
	if m.audioManager == nil {
		return fmt.Errorf("audio manager not set")
	}
	if err := m.audioManager.SetAudioSource(source); err != nil {
		return err
	}
	m.activeSource = source
	return nil
}

// GetAudioSource returns the current audio source
func (m *STTManager) GetAudioSource() domain.AudioSource {
	return m.activeSource
}

// GetAvailableAudioSources returns available audio sources
func (m *STTManager) GetAvailableAudioSources() []domain.AudioSource {
	if m.audioManager == nil {
		return []domain.AudioSource{domain.AudioSourceMicrophone}
	}
	return m.audioManager.GetAvailableSources()
}

// StartRecordingWithSource starts recording with the specified audio source
func (m *STTManager) StartRecordingWithSource(source domain.AudioSource) error {
	if m.audioManager == nil {
		return fmt.Errorf("audio manager not set")
	}

	if m.transcriber == nil {
		return fmt.Errorf("transcriber not initialized")
	}

	slog.Info("Starting recording request", "source", source.String())

	// Set the audio source
	if err := m.audioManager.SetAudioSource(source); err != nil {
		slog.Error("Failed to set audio source", "source", source.String(), "error", err)
		return fmt.Errorf("failed to set audio source: %w", err)
	}
	m.activeSource = source

	// Start the transcriber
	if err := m.transcriber.StartProcessing(); err != nil {
		slog.Error("Failed to start transcriber", "source", source.String(), "error", err)
		return fmt.Errorf("failed to start transcriber: %w", err)
	}

	// Configure audio capture
	config := domain.AudioCaptureConfig{
		Source:     source,
		SampleRate: 16000, // Whisper expects 16kHz
		Channels:   1,     // Mono for speech
		BufferSize: 1024,
	}

	// Start audio capture with callback to transcriber
	err := m.audioManager.StartCapture(config, func(chunk domain.AudioChunk) {
		m.transcriber.ProcessChunk(chunk)
	})
	if err != nil {
		// Cancel without emitting a transcription:done event — recording never
		// actually started so the frontend should not receive a result.
		m.transcriber.CancelProcessing()
		slog.Error("Failed to start audio capture", "source", source.String(), "sampleRate", config.SampleRate, "channels", config.Channels, "bufferSize", config.BufferSize, "error", err)
		return fmt.Errorf("failed to start audio capture: %w", err)
	}

	slog.Info("Recording started", "source", source.String())
	return nil
}

// StopRecordingWithTranscription stops recording and returns the transcription
func (m *STTManager) StopRecordingWithTranscription() (*domain.TranscriptionResult, error) {
	if m.audioManager == nil {
		return nil, fmt.Errorf("audio manager not set")
	}

	// Stop audio capture (best-effort — log but don't fail)
	if err := m.audioManager.StopCapture(); err != nil {
		slog.Warn("Error stopping audio capture", "error", err)
	}

	// Stop transcriber and get final result.
	if m.transcriber != nil {
		return m.transcriber.StopProcessing()
	}

	return nil, fmt.Errorf("transcriber not initialized")
}

// Cleanup cleans up the STT service
func (m *STTManager) Cleanup() {
	if m.transcriber != nil {
		m.transcriber.Cleanup()
	}
}

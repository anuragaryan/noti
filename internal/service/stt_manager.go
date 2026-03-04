package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"noti/internal/domain"
	"noti/internal/stt/whisper"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// STTManager handles STT service lifecycle and model management
type STTManager struct {
	basePath       string
	ctx            context.Context
	transcriber    *whisper.Transcriber
	audioManager   *AudioManager
	downloadScript []byte
	activeSource   domain.AudioSource
}

// NewSTTManager creates a new STT manager
func NewSTTManager(basePath string, downloadScript []byte) *STTManager {
	return &STTManager{
		basePath:       basePath,
		downloadScript: downloadScript,
		activeSource:   domain.AudioSourceMicrophone,
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
			fmt.Printf("Failed to initialize STT model: %v\n", err)
			return false
		}
		// Success
		transcriber.SetContext(m.ctx)
		m.transcriber = transcriber

		fmt.Println("STT service initialized successfully.")
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "stt:ready")
		}
		return true
	}

	if !tryInitialize() {
		// Initialization failed, likely because the model is missing or corrupt
		fmt.Println("STT initialization failed. Attempting to download or re-download model...")

		// Delete the potentially corrupt model file before downloading
		modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
		modelPath := filepath.Join(m.basePath, "models", modelFileName)
		if _, err := os.Stat(modelPath); err == nil {
			fmt.Printf("Deleting existing model file at %s to ensure a clean download.\n", modelPath)
			os.Remove(modelPath)
		}

		// Download model and try to initialize again
		if err := m.DownloadModel(config); err != nil {
			fmt.Printf("ERROR: Model download and initialization failed: %v\n", err)
			fmt.Println("Speech-to-text features will be disabled.")
			return err
		}
	}

	return nil
}

// DownloadModel downloads a Whisper model and initializes the STT service
func (m *STTManager) DownloadModel(config *domain.STTConfig) error {
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "download:start", config.ModelName)
	}

	// Get the models directory
	modelsPath := filepath.Join(m.basePath, "models")
	if err := os.MkdirAll(modelsPath, 0755); err != nil {
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "download:error", "Failed to create models directory")
		}
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	// Define the script path inside the models directory
	scriptPath := filepath.Join(modelsPath, ".download-ggml-model.sh")

	// Write the embedded script to the destination and make it executable
	if err := os.WriteFile(scriptPath, m.downloadScript, 0755); err != nil {
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "download:error", "Failed to write download script")
		}
		return fmt.Errorf("failed to write download script: %w", err)
	}
	// Ensure the script is cleaned up
	defer os.Remove(scriptPath)

	// Run the script from within the models directory
	cmd := exec.Command(scriptPath, config.ModelName)
	cmd.Dir = modelsPath // Set the working directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Model download script failed:\n%s\n", string(output))
		if m.ctx != nil {
			runtime.EventsEmit(m.ctx, "download:error", "Model download failed. Check logs.")
		}
		return fmt.Errorf("model download script failed: %w", err)
	}

	fmt.Printf("Model download script output:\n%s\n", string(output))
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "download:finish", config.ModelName)
	}

	// After a successful download, re-initialize the STT service
	fmt.Println("Re-initializing STT service after model download...")
	transcriber, err := whisper.NewTranscriber(m.basePath, config)
	if err != nil {
		fmt.Printf("Warning: STT service initialization failed after download: %v\n", err)
		return err
	}

	if err := transcriber.Initialize(); err != nil {
		fmt.Printf("Warning: Failed to load STT model after download: %v\n", err)
		return err
	}

	transcriber.SetContext(m.ctx)
	m.transcriber = transcriber
	fmt.Println("STT service initialized successfully after download.")

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

	// Set the audio source
	if err := m.audioManager.SetAudioSource(source); err != nil {
		return fmt.Errorf("failed to set audio source: %w", err)
	}
	m.activeSource = source

	// Start the transcriber
	if err := m.transcriber.StartProcessing(); err != nil {
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
		return fmt.Errorf("failed to start audio capture: %w", err)
	}

	fmt.Printf("Recording started with source: %s\n", source.String())
	return nil
}

// StopRecordingWithTranscription stops recording and returns the transcription
func (m *STTManager) StopRecordingWithTranscription() (*domain.TranscriptionResult, error) {
	if m.audioManager == nil {
		return nil, fmt.Errorf("audio manager not set")
	}

	// Stop audio capture (best-effort — log but don't fail)
	if err := m.audioManager.StopCapture(); err != nil {
		fmt.Printf("Warning: error stopping audio capture: %v\n", err)
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

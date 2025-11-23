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
	sttService     STTService
	downloadScript []byte
}

// NewSTTManager creates a new STT manager
func NewSTTManager(basePath string, downloadScript []byte) *STTManager {
	return &STTManager{
		basePath:       basePath,
		downloadScript: downloadScript,
	}
}

// SetContext sets the Wails runtime context
func (m *STTManager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Initialize attempts to initialize the STT service with self-healing
func (m *STTManager) Initialize(config *domain.STTConfig) error {
	// Helper function to attempt STT initialization
	tryInitialize := func() bool {
		sttService, err := whisper.NewService(m.basePath, config)
		if err != nil {
			// This handles file-not-found
			return false
		}
		if err := sttService.Initialize(); err != nil {
			// This handles model loading/corruption errors
			fmt.Printf("Failed to initialize STT model: %v\n", err)
			return false
		}
		// Success
		sttService.SetContext(m.ctx)
		m.sttService = sttService
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
	sttService, err := whisper.NewService(m.basePath, config)
	if err != nil {
		fmt.Printf("Warning: STT service initialization failed after download: %v\n", err)
		return err
	}

	if err := sttService.Initialize(); err != nil {
		fmt.Printf("Warning: Failed to load STT model after download: %v\n", err)
		return err
	}

	sttService.SetContext(m.ctx)
	m.sttService = sttService
	fmt.Println("STT service initialized successfully after download.")

	// Notify the frontend that the service is now ready
	if m.ctx != nil {
		runtime.EventsEmit(m.ctx, "stt:ready")
	}

	return nil
}

// GetService returns the underlying STT service
func (m *STTManager) GetService() STTService {
	return m.sttService
}

// IsAvailable returns whether the STT service is available
func (m *STTManager) IsAvailable() bool {
	return m.sttService != nil
}

// Cleanup cleans up the STT service
func (m *STTManager) Cleanup() {
	if m.sttService != nil {
		m.sttService.Cleanup()
	}
}

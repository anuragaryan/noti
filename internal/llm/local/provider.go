// Package local provides an LLM provider implementation using local models via llama-server
package local

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
	"noti/internal/events"
	"noti/internal/infrastructure/downloader"
	"noti/internal/llm/shared"
)

// Provider implements LLM using local models via llama-server
type Provider struct {
	client          *shared.Client
	streamingClient *shared.StreamingClient
	config          *domain.LLMConfig
	serverManager   *ServerManager
	modelPath       string
	basePath        string
	available       bool
	mutex           sync.Mutex
	ctx             context.Context
}

// NewProvider creates a new local LLM provider
func NewProvider(basePath string, config *domain.LLMConfig) (*Provider, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required for local provider")
	}

	return &Provider{
		config:    config,
		basePath:  basePath,
		available: false,
	}, nil
}

// Initialize loads the LLM model and starts llama-server
func (p *Provider) Initialize() error {
	slog.Info("=== Initializing Local LLM Provider ===")
	slog.Info("Model", "name", p.config.ModelName)
	slog.Info("Base path", "path", p.basePath)

	// Determine model file path
	modelFileName := p.config.ModelName
	if !strings.HasSuffix(modelFileName, ".gguf") {
		modelFileName = modelFileName + ".gguf"
	}

	modelsDir := filepath.Join(p.basePath, "models", "llm")
	modelPath := filepath.Join(modelsDir, modelFileName)

	// Check if model exists and validate
	needsDownload := false
	fileInfo, err := os.Stat(modelPath)
	if err != nil {
		slog.Info("✗ Model file not found", "path", modelPath)
		needsDownload = true
	} else {
		// Validate model file size (GGUF files should be at least 1MB)
		if fileInfo.Size() < 1024*1024 {
			slog.Warn("✗ Model file appears to be corrupt or incomplete", "size", fileInfo.Size())
			slog.Info("Removing corrupt model file", "path", modelPath)

			// Remove corrupt file
			if err := os.Remove(modelPath); err != nil {
				slog.Warn("Could not remove corrupt file", "error", err)
			}
			needsDownload = true
		}
	}

	// Auto-download model if needed
	if needsDownload {
		slog.Info("Attempting to download model", "model", p.config.ModelName)
		if err := p.downloadModel(); err != nil {
			return fmt.Errorf("failed to download model: %w", err)
		}

		// Verify download succeeded
		fileInfo, err = os.Stat(modelPath)
		if err != nil {
			return fmt.Errorf("model file still not found after download: %w", err)
		}
		if fileInfo.Size() < 1024*1024 {
			return fmt.Errorf("downloaded model file appears invalid (size: %d bytes)", fileInfo.Size())
		}
	}

	p.modelPath = modelPath
	slog.Info("✓ Found valid model", "path", modelPath, "sizeMB", float64(fileInfo.Size())/(1024*1024))

	// Initialize server manager (will be created with download script in main.go)
	if p.serverManager == nil {
		slog.Info("✗ Server manager not set")
		return fmt.Errorf("server manager not set - this is a bug in the initialization code")
	}

	// Ensure llama-server binary is available
	slog.Info("Ensuring llama-server binary is available...")
	if err := p.serverManager.EnsureBinary(); err != nil {
		slog.Error("Failed to ensure binary", "error", err)
		return fmt.Errorf("failed to ensure llama-server binary: %w", err)
	}

	// Start llama-server
	slog.Info("Starting llama-server...")
	if err := p.serverManager.Start(modelPath); err != nil {
		slog.Error("Failed to start server", "error", err)
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	// Verify server is healthy
	slog.Info("Performing health check...")
	if err := p.serverManager.HealthCheck(); err != nil {
		slog.Error("Health check failed", "error", err)
		p.serverManager.Stop()
		return fmt.Errorf("llama-server health check failed: %w", err)
	}

	// Create HTTP clients for the server
	endpoint := fmt.Sprintf("%s/v1/chat/completions", p.serverManager.GetEndpoint())
	p.client = shared.NewClient(endpoint, "", 120*time.Second)
	p.streamingClient = shared.NewStreamingClient(endpoint, "", 300*time.Second)

	p.mutex.Lock()
	p.available = true
	p.mutex.Unlock()

	slog.Info("✓ Local LLM provider initialized successfully!")
	slog.Info("✓ Model", "name", p.config.ModelName)
	slog.Info("✓ Server endpoint", "endpoint", p.serverManager.GetEndpoint())
	slog.Info("✓ Temperature", "temperature", p.config.Temperature)
	slog.Info("✓ Max tokens", "tokens", p.config.MaxTokens)
	slog.Info("✓ Provider available", "available", p.available)

	return nil
}

// SetServerManager sets the llama-server manager
func (p *Provider) SetServerManager(manager *ServerManager) {
	p.serverManager = manager
	if manager != nil {
		manager.SetContext(p.ctx)
	}
}

// SetContext passes the runtime context down to the provider and server manager.
func (p *Provider) SetContext(ctx context.Context) {
	p.ctx = ctx
	if p.serverManager != nil {
		p.serverManager.SetContext(ctx)
	}
}

// Generate produces text based on the request
func (p *Provider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	p.mutex.Lock()
	if !p.available {
		p.mutex.Unlock()
		return nil, fmt.Errorf("provider not initialized")
	}
	p.mutex.Unlock()

	if !p.serverManager.IsRunning() {
		return nil, fmt.Errorf("llama-server is not running")
	}

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create API request (llama-server uses OpenAI-compatible API)
	apiReq := &shared.ChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
	}

	slog.Debug("[Local LLM] Generating response for prompt", "length", len(request.Prompt))
	slog.Debug("[Local LLM] Generation params", "temperature", p.config.Temperature, "maxTokens", p.config.MaxTokens)

	// Send request using shared client
	apiResp, err := p.client.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	// Validate response
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in API response")
	}

	choice := apiResp.Choices[0]
	text := strings.TrimSpace(choice.Message.Content)

	slog.Info("[Local LLM] Generated response", "chars", len(text), "tokens", apiResp.Usage.TotalTokens)

	return &domain.LLMResponse{
		Text:         text,
		TokensUsed:   apiResp.Usage.TotalTokens,
		Model:        apiResp.Model,
		FinishReason: choice.FinishReason,
	}, nil
}

// GenerateStream produces text with streaming response
func (p *Provider) GenerateStream(ctx context.Context, request *domain.LLMRequest, callback domain.StreamCallback) error {
	p.mutex.Lock()
	if !p.available {
		p.mutex.Unlock()
		return fmt.Errorf("provider not initialized")
	}
	p.mutex.Unlock()

	if !p.serverManager.IsRunning() {
		return fmt.Errorf("llama-server is not running")
	}

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create streaming API request (llama-server uses OpenAI-compatible API)
	streamReq := &shared.StreamChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
		Stream:      true,
	}

	slog.Info("[Local LLM] Starting streaming response for prompt", "length", len(request.Prompt))
	slog.Info("[Local LLM] Generation params", "temperature", p.config.Temperature, "maxTokens", p.config.MaxTokens)

	// Track chunk index
	chunkIndex := 0

	// Use streaming client
	err := p.streamingClient.StreamChatCompletion(ctx, streamReq,
		func(text string, reasoningText string, done bool, finishReason string) error {
			chunk := &domain.StreamChunk{
				Text:          text,
				ReasoningText: reasoningText,
				Index:         chunkIndex,
				FinishReason:  finishReason,
				Done:          done,
			}
			chunkIndex++
			return callback(chunk)
		})

	if err != nil {
		return fmt.Errorf("streaming failed: %w", err)
	}

	slog.Info("[Local LLM] Streaming completed", "chunks", chunkIndex)
	return nil
}

// IsAvailable returns whether the provider is ready to use
func (p *Provider) IsAvailable() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.available && p.serverManager != nil && p.serverManager.IsRunning()
}

// SupportsStreaming returns whether the provider supports streaming
func (p *Provider) SupportsStreaming() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.available && p.serverManager != nil && p.serverManager.IsRunning()
}

// Cleanup releases resources
func (p *Provider) Cleanup() {
	slog.Info("Cleaning up local LLM provider...")
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		p.client.Close()
	}

	if p.streamingClient != nil {
		p.streamingClient.Close()
	}

	if p.serverManager != nil {
		p.serverManager.Stop()
	}

	p.available = false
	slog.Info("✓ Local LLM provider cleanup complete")
}

// GetModelInfo returns information about the current model
func (p *Provider) GetModelInfo() map[string]interface{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	info := map[string]interface{}{
		"provider":    "local",
		"model":       p.config.ModelName,
		"available":   p.available,
		"temperature": p.config.Temperature,
		"maxTokens":   p.config.MaxTokens,
	}

	if p.serverManager != nil {
		info["serverRunning"] = p.serverManager.IsRunning()
		info["endpoint"] = p.serverManager.GetEndpoint()
		info["modelPath"] = p.serverManager.GetModelPath()
	}

	return info
}

// downloadModel downloads the LLM model using the downloader package
func (p *Provider) downloadModel() error {
	modelsDir := filepath.Join(p.basePath, "models", "llm")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	downloadID := fmt.Sprintf("llm:%s", p.config.ModelName)
	events.EmitDownloadEvent(p.ctx, events.DownloadEvent{
		ID:     downloadID,
		Kind:   events.DownloadKindLLMModel,
		Label:  p.config.ModelName,
		Status: events.DownloadStatusQueued,
	})

	slog.Info("Downloading LLM model", "model", p.config.ModelName)
	opts := &downloader.DownloadOptions{
		DestDir: modelsDir,
		ProgressFunc: func(downloaded, total int64) {
			events.EmitDownloadEvent(p.ctx, events.DownloadEvent{
				ID:              downloadID,
				Kind:            events.DownloadKindLLMModel,
				Label:           p.config.ModelName,
				Status:          events.DownloadStatusDownloading,
				BytesDownloaded: downloaded,
				TotalBytes:      total,
				Percent:         events.CalculatePercent(downloaded, total),
			})
		},
	}
	// Use Background so model downloads continue even if the UI context is cancelled;
	// progress events still go through p.ctx for the active session.
	result, err := downloader.DownloadLLM(context.Background(), p.config.ModelName, opts)
	if err != nil {
		events.EmitDownloadEvent(p.ctx, events.DownloadEvent{
			ID:     downloadID,
			Kind:   events.DownloadKindLLMModel,
			Label:  p.config.ModelName,
			Status: events.DownloadStatusError,
			Error:  err.Error(),
		})
		return fmt.Errorf("download failed: %w", err)
	}

	if result.Skipped {
		slog.Info("Model already present", "path", result.DestPath)
	} else {
		slog.Info("Model downloaded", "path", result.DestPath, "sizeMB", float64(result.SizeBytes)/(1024*1024))
	}

	events.EmitDownloadEvent(p.ctx, events.DownloadEvent{
		ID:              downloadID,
		Kind:            events.DownloadKindLLMModel,
		Label:           p.config.ModelName,
		Status:          events.DownloadStatusCompleted,
		BytesDownloaded: result.SizeBytes,
		TotalBytes:      result.SizeBytes,
		Percent:         100,
	})

	return nil
}

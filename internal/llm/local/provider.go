// Package local provides an LLM provider implementation using local models via llama-server
package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
	"noti/internal/infrastructure/downloader"
	"noti/internal/llm/shared"
)

// Provider implements LLM using local models via llama-server
type Provider struct {
	client        *shared.Client
	config        *domain.LLMConfig
	serverManager *ServerManager
	modelPath     string
	basePath      string
	downloader    *downloader.Downloader
	available     bool
	mutex         sync.Mutex
}

// NewProvider creates a new local LLM provider
func NewProvider(basePath string, config *domain.LLMConfig, downloadScript []byte) (*Provider, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required for local provider")
	}

	return &Provider{
		config:     config,
		basePath:   basePath,
		downloader: downloader.NewDownloader(downloadScript),
		available:  false,
	}, nil
}

// Initialize loads the LLM model and starts llama-server
func (p *Provider) Initialize() error {
	fmt.Println("=== Initializing Local LLM Provider ===")
	fmt.Printf("Model: %s\n", p.config.ModelName)
	fmt.Printf("Base path: %s\n", p.basePath)

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
		fmt.Printf("✗ Model file not found: %s\n", modelPath)
		needsDownload = true
	} else {
		// Validate model file size (GGUF files should be at least 1MB)
		if fileInfo.Size() < 1024*1024 {
			fmt.Printf("✗ Model file appears to be corrupt or incomplete (size: %d bytes)\n", fileInfo.Size())
			fmt.Printf("Removing corrupt model file: %s\n", modelPath)

			// Remove corrupt file
			if err := os.Remove(modelPath); err != nil {
				fmt.Printf("Warning: Could not remove corrupt file: %v\n", err)
			}
			needsDownload = true
		}
	}

	// Auto-download model if needed
	if needsDownload {
		fmt.Printf("Attempting to download model: %s\n", p.config.ModelName)
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
	fmt.Printf("✓ Found valid model at: %s (size: %.2f MB)\n", modelPath, float64(fileInfo.Size())/(1024*1024))

	// Initialize server manager (will be created with download script in main.go)
	if p.serverManager == nil {
		fmt.Println("✗ Server manager not set")
		return fmt.Errorf("server manager not set - this is a bug in the initialization code")
	}

	// Ensure llama-server binary is available
	fmt.Println("Ensuring llama-server binary is available...")
	if err := p.serverManager.EnsureBinary(); err != nil {
		fmt.Printf("✗ Failed to ensure binary: %v\n", err)
		return fmt.Errorf("failed to ensure llama-server binary: %w", err)
	}

	// Start llama-server
	fmt.Println("Starting llama-server...")
	if err := p.serverManager.Start(modelPath); err != nil {
		fmt.Printf("✗ Failed to start server: %v\n", err)
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	// Verify server is healthy
	fmt.Println("Performing health check...")
	if err := p.serverManager.HealthCheck(); err != nil {
		fmt.Printf("✗ Health check failed: %v\n", err)
		p.serverManager.Stop()
		return fmt.Errorf("llama-server health check failed: %w", err)
	}

	// Create HTTP client for the server
	endpoint := fmt.Sprintf("%s/v1/chat/completions", p.serverManager.GetEndpoint())
	p.client = shared.NewClient(endpoint, "", 120*time.Second)

	p.mutex.Lock()
	p.available = true
	p.mutex.Unlock()

	fmt.Println("✓ Local LLM provider initialized successfully!")
	fmt.Printf("✓ Model: %s\n", p.config.ModelName)
	fmt.Printf("✓ Server endpoint: %s\n", p.serverManager.GetEndpoint())
	fmt.Printf("✓ Temperature: %.2f\n", p.config.Temperature)
	fmt.Printf("✓ Max tokens: %d\n", p.config.MaxTokens)
	fmt.Printf("✓ Provider available: %v\n\n", p.available)

	return nil
}

// SetServerManager sets the llama-server manager
func (p *Provider) SetServerManager(manager *ServerManager) {
	p.serverManager = manager
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

	// Use request-specific parameters or fall back to config defaults
	temperature := shared.GetEffectiveTemperature(request.Temperature, p.config.Temperature)
	maxTokens := shared.GetEffectiveMaxTokens(request.MaxTokens, p.config.MaxTokens)

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create API request (llama-server uses OpenAI-compatible API)
	apiReq := &shared.ChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	fmt.Printf("[Local LLM] Generating response for prompt (length: %d chars)\n", len(request.Prompt))
	fmt.Printf("[Local LLM] Temperature: %.2f, Max tokens: %d\n", temperature, maxTokens)

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

	fmt.Printf("[Local LLM] Generated %d characters, used %d tokens\n", len(text), apiResp.Usage.TotalTokens)

	return &domain.LLMResponse{
		Text:         text,
		TokensUsed:   apiResp.Usage.TotalTokens,
		Model:        apiResp.Model,
		FinishReason: choice.FinishReason,
	}, nil
}

// IsAvailable returns whether the provider is ready to use
func (p *Provider) IsAvailable() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.available && p.serverManager != nil && p.serverManager.IsRunning()
}

// Cleanup releases resources
func (p *Provider) Cleanup() {
	fmt.Println("Cleaning up local LLM provider...")
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		p.client.Close()
	}

	if p.serverManager != nil {
		p.serverManager.Stop()
	}

	p.available = false
	fmt.Println("✓ Local LLM provider cleanup complete")
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

// downloadModel downloads the LLM model using the embedded script
func (p *Provider) downloadModel() error {
	modelsDir := filepath.Join(p.basePath, "models", "llm")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	// Download using the downloader
	fmt.Printf("Running model download script for: %s\n", p.config.ModelName)
	if err := p.downloader.Download(modelsDir, p.config.ModelName); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	return nil
}

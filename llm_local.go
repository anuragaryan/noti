package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
)

// LocalLLMProvider implements LLM using local models via llama-server
type LocalLLMProvider struct {
	modelMutex     sync.Mutex
	modelPath      string
	config         *domain.LLMConfig
	available      bool
	serverManager  *LlamaServerManager
	client         *http.Client
	basePath       string
	downloadScript []byte
}

// NewLocalLLMProvider creates a new local LLM provider
func NewLocalLLMProvider(basePath string, config *domain.LLMConfig, downloadScript []byte) (*LocalLLMProvider, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required for local provider")
	}

	return &LocalLLMProvider{
		modelPath: "",
		config:    config,
		available: false,
		client: &http.Client{
			Timeout: 120 * time.Second, // Local inference can take longer
		},
		basePath:       basePath,
		downloadScript: downloadScript,
	}, nil
}

// Initialize loads the LLM model and starts llama-server
func (p *LocalLLMProvider) Initialize() error {
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

	p.available = true
	fmt.Println("✓ Local LLM provider initialized successfully!")
	fmt.Printf("✓ Model: %s\n", p.config.ModelName)
	fmt.Printf("✓ Server endpoint: %s\n", p.serverManager.GetEndpoint())
	fmt.Printf("✓ Temperature: %.2f\n", p.config.Temperature)
	fmt.Printf("✓ Max tokens: %d\n", p.config.MaxTokens)
	fmt.Printf("✓ Provider available: %v\n\n", p.available)

	return nil
}

// SetServerManager sets the llama-server manager
func (p *LocalLLMProvider) SetServerManager(manager *LlamaServerManager) {
	p.serverManager = manager
}

// Generate produces text based on the request
func (p *LocalLLMProvider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	if !p.available {
		return nil, fmt.Errorf("provider not initialized")
	}

	if !p.serverManager.IsRunning() {
		return nil, fmt.Errorf("llama-server is not running")
	}

	// Use request-specific parameters or fall back to config defaults
	temperature := request.Temperature
	if temperature == 0 {
		temperature = p.config.Temperature
	}

	maxTokens := request.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.config.MaxTokens
	}

	// Build messages array
	messages := []apiMessage{}

	// Add system message if provided
	if request.SystemPrompt != "" {
		messages = append(messages, apiMessage{
			Role:    "system",
			Content: request.SystemPrompt,
		})
	}

	// Add user message
	messages = append(messages, apiMessage{
		Role:    "user",
		Content: request.Prompt,
	})

	// Create API request (llama-server uses OpenAI-compatible API)
	apiReq := apiRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	fmt.Printf("[Local LLM] Generating response for prompt (length: %d chars)\n", len(request.Prompt))
	fmt.Printf("[Local LLM] Temperature: %.2f, Max tokens: %d\n", temperature, maxTokens)

	// Thread-safe API call
	p.modelMutex.Lock()
	defer p.modelMutex.Unlock()

	// Marshal request
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request to llama-server
	endpoint := fmt.Sprintf("%s/v1/chat/completions", p.serverManager.GetEndpoint())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
func (p *LocalLLMProvider) IsAvailable() bool {
	return p.available && p.serverManager != nil && p.serverManager.IsRunning()
}

// Cleanup releases resources
func (p *LocalLLMProvider) Cleanup() {
	fmt.Println("Cleaning up local LLM provider...")
	p.modelMutex.Lock()
	defer p.modelMutex.Unlock()

	if p.serverManager != nil {
		p.serverManager.Stop()
	}

	p.available = false
	fmt.Println("✓ Local LLM provider cleanup complete")
}

// GetModelInfo returns information about the current model
func (p *LocalLLMProvider) GetModelInfo() map[string]interface{} {
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
func (p *LocalLLMProvider) downloadModel() error {
	modelsDir := filepath.Join(p.basePath, "models", "llm")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	// Write the download script
	scriptPath := filepath.Join(modelsDir, ".download-llm-model.sh")
	if err := os.WriteFile(scriptPath, p.downloadScript, 0755); err != nil {
		return fmt.Errorf("failed to write download script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Run the download script
	fmt.Printf("Running model download script for: %s\n", p.config.ModelName)
	cmd := exec.Command("/bin/bash", scriptPath, p.config.ModelName)
	cmd.Dir = modelsDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	// Always print output
	fmt.Printf("Download script output:\n%s\n", string(output))

	if err != nil {
		return fmt.Errorf("download script failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// Package api provides an LLM provider implementation using OpenAI-compatible APIs
package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
	"noti/internal/llm/shared"
)

// Provider implements LLM using OpenAI-compatible API endpoints
type Provider struct {
	client          *shared.Client
	streamingClient *shared.StreamingClient
	endpoint        string
	config          *domain.LLMConfig
	available       bool
	mutex           sync.Mutex
}

// NewProvider creates a new API-based LLM provider
func NewProvider(config *domain.LLMConfig) (*Provider, error) {
	if config.APIEndpoint == "" {
		return nil, fmt.Errorf("API endpoint is required")
	}

	// Validate endpoint URL
	if !strings.HasPrefix(config.APIEndpoint, "http://") && !strings.HasPrefix(config.APIEndpoint, "https://") {
		return nil, fmt.Errorf("API endpoint must start with http:// or https://")
	}

	// Ensure endpoint has the proper path for OpenAI-compatible APIs
	endpoint := strings.TrimRight(config.APIEndpoint, "/")
	if !strings.HasSuffix(endpoint, "/v1/chat/completions") && !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint = endpoint + "/v1/chat/completions"
	}

	client := shared.NewClient(endpoint, config.APIKey, 60*time.Second)
	streamingClient := shared.NewStreamingClient(endpoint, config.APIKey, 120*time.Second)

	return &Provider{
		client:          client,
		streamingClient: streamingClient,
		endpoint:        endpoint,
		config:          config,
		available:       false,
	}, nil
}

// SetContext satisfies the LLMProvider interface. API providers currently do not
// emit download events, so the context is not used.
func (p *Provider) SetContext(ctx context.Context) {}

// Initialize validates the API connection
func (p *Provider) Initialize() error {
	slog.Info("=== Initializing API LLM ===\n")
	slog.Info("Endpoint", "endpoint", p.endpoint)
	slog.Info("Model", "name", p.config.ModelName)

	// Set available to true temporarily for the test
	p.mutex.Lock()
	p.available = true
	p.mutex.Unlock()

	// Test the connection with a simple request
	testRequest := &domain.LLMRequest{
		Prompt: "ping",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, testRequest)
	if err != nil {
		p.mutex.Lock()
		p.available = false
		p.mutex.Unlock()
		return fmt.Errorf("API connection test failed: %w", err)
	}

	slog.Info("✓ API LLM provider initialized successfully!")
	slog.Info("✓ Endpoint", "endpoint", p.endpoint)
	slog.Info("✓ Model", "name", p.config.ModelName)
	slog.Info("✓ Temperature", "temperature", p.config.Temperature)
	slog.Info("✓ Max tokens", "tokens", p.config.MaxTokens)

	return nil
}

// Generate produces text based on the request
func (p *Provider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	p.mutex.Lock()
	if !p.available {
		p.mutex.Unlock()
		return nil, fmt.Errorf("provider not initialized")
	}
	p.mutex.Unlock()

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create API request
	apiReq := &shared.ChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
	}

	slog.Info("[LLM API] Generating response for prompt", "length", len(request.Prompt))
	slog.Info("[LLM API] Generation params", "temperature", p.config.Temperature, "maxTokens", p.config.MaxTokens)

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

	slog.Info("[LLM API] Generated response", "chars", len(text), "tokens", apiResp.Usage.TotalTokens)

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

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create streaming API request
	streamReq := &shared.StreamChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
		Stream:      true,
	}

	slog.Info("[LLM API] Starting streaming response for prompt", "length", len(request.Prompt))
	slog.Info("[LLM API] Generation params", "temperature", p.config.Temperature, "maxTokens", p.config.MaxTokens)

	// Track chunk index
	chunkIndex := 0

	// Use streaming client
	err := p.streamingClient.StreamChatCompletion(ctx, streamReq,
		func(text string, done bool, finishReason string) error {
			chunk := &domain.StreamChunk{
				Text:         text,
				Index:        chunkIndex,
				FinishReason: finishReason,
				Done:         done,
			}
			chunkIndex++
			return callback(chunk)
		})

	if err != nil {
		return fmt.Errorf("streaming failed: %w", err)
	}

	slog.Info("[LLM API] Streaming completed", "chunks", chunkIndex)
	return nil
}

// IsAvailable returns whether the provider is ready to use
func (p *Provider) IsAvailable() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.available
}

// SupportsStreaming returns whether the provider supports streaming
// API providers always support streaming when available (OpenAI-compatible APIs support SSE)
func (p *Provider) SupportsStreaming() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.available
}

// Cleanup releases resources
func (p *Provider) Cleanup() {
	slog.Info("Cleaning up API LLM provider...")
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		p.client.Close()
	}
	if p.streamingClient != nil {
		p.streamingClient.Close()
	}
	p.available = false
}

// GetModelInfo returns information about the current model
func (p *Provider) GetModelInfo() map[string]interface{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return map[string]interface{}{
		"provider":    "api",
		"model":       p.config.ModelName,
		"endpoint":    p.endpoint,
		"available":   p.available,
		"temperature": p.config.Temperature,
		"maxTokens":   p.config.MaxTokens,
		"hasApiKey":   p.config.APIKey != "",
	}
}

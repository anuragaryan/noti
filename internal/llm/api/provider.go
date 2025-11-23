// Package api provides an LLM provider implementation using OpenAI-compatible APIs
package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
	"noti/internal/llm/shared"
)

// Provider implements LLM using OpenAI-compatible API endpoints
type Provider struct {
	client    *shared.Client
	endpoint  string
	config    *domain.LLMConfig
	available bool
	mutex     sync.Mutex
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

	return &Provider{
		client:    client,
		endpoint:  endpoint,
		config:    config,
		available: false,
	}, nil
}

// Initialize validates the API connection
func (p *Provider) Initialize() error {
	fmt.Printf("=== Initializing API LLM ===\n")
	fmt.Printf("Endpoint: %s\n", p.endpoint)
	fmt.Printf("Model: %s\n", p.config.ModelName)

	// Set available to true temporarily for the test
	p.mutex.Lock()
	p.available = true
	p.mutex.Unlock()

	// Test the connection with a simple request
	testRequest := &domain.LLMRequest{
		Prompt:    "Hello",
		MaxTokens: 5,
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

	fmt.Println("✓ API LLM provider initialized successfully!")
	fmt.Printf("✓ Endpoint: %s\n", p.endpoint)
	fmt.Printf("✓ Model: %s\n", p.config.ModelName)
	fmt.Printf("✓ Temperature: %.2f\n", p.config.Temperature)
	fmt.Printf("✓ Max tokens: %d\n\n", p.config.MaxTokens)

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

	// Use request-specific parameters or fall back to config defaults
	temperature := shared.GetEffectiveTemperature(request.Temperature, p.config.Temperature)
	maxTokens := shared.GetEffectiveMaxTokens(request.MaxTokens, p.config.MaxTokens)

	// Build messages array
	messages := shared.BuildMessages(request)

	// Create API request
	apiReq := &shared.ChatRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	fmt.Printf("[LLM API] Generating response for prompt (length: %d chars)\n", len(request.Prompt))
	fmt.Printf("[LLM API] Temperature: %.2f, Max tokens: %d\n", temperature, maxTokens)

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

	fmt.Printf("[LLM API] Generated %d characters, used %d tokens\n", len(text), apiResp.Usage.TotalTokens)

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
	return p.available
}

// Cleanup releases resources
func (p *Provider) Cleanup() {
	fmt.Println("Cleaning up API LLM provider...")
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		p.client.Close()
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

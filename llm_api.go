package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
)

// APILLMProvider implements LLM using OpenAI-compatible API endpoints
type APILLMProvider struct {
	client     *http.Client
	endpoint   string
	apiKey     string
	config     *domain.LLMConfig
	available  bool
	modelMutex sync.Mutex
}

// OpenAI API request/response structures
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Temperature float32      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
}

type apiChoice struct {
	Index        int        `json:"index"`
	Message      apiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type apiResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []apiChoice `json:"choices"`
	Usage   apiUsage    `json:"usage"`
}

type apiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// NewAPILLMProvider creates a new API-based LLM provider
func NewAPILLMProvider(config *domain.LLMConfig) (*APILLMProvider, error) {
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

	return &APILLMProvider{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		endpoint:  endpoint,
		apiKey:    config.APIKey,
		config:    config,
		available: false,
	}, nil
}

// Initialize validates the API connection
func (p *APILLMProvider) Initialize() error {
	fmt.Printf("=== Initializing API LLM ===\n")
	fmt.Printf("Endpoint: %s\n", p.endpoint)
	fmt.Printf("Model: %s\n", p.config.ModelName)

	// Set available to true temporarily for the test
	p.available = true

	// Test the connection with a simple request
	testRequest := &domain.LLMRequest{
		Prompt:    "Hello",
		MaxTokens: 5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, testRequest)
	if err != nil {
		p.available = false // Reset on failure
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
func (p *APILLMProvider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	if !p.available {
		return nil, fmt.Errorf("provider not initialized")
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

	// Create API request
	apiReq := apiRequest{
		Model:       p.config.ModelName,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	fmt.Printf("[LLM API] Generating response for prompt (length: %d chars)\n", len(request.Prompt))
	fmt.Printf("[LLM API] Temperature: %.2f, Max tokens: %d\n", temperature, maxTokens)

	// Thread-safe API call
	p.modelMutex.Lock()
	defer p.modelMutex.Unlock()

	// Marshal request
	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

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

	fmt.Printf("[LLM API] Generated %d characters, used %d tokens\n", len(text), apiResp.Usage.TotalTokens)

	return &domain.LLMResponse{
		Text:         text,
		TokensUsed:   apiResp.Usage.TotalTokens,
		Model:        apiResp.Model,
		FinishReason: choice.FinishReason,
	}, nil
}

// IsAvailable returns whether the provider is ready to use
func (p *APILLMProvider) IsAvailable() bool {
	return p.available
}

// Cleanup releases resources
func (p *APILLMProvider) Cleanup() {
	fmt.Println("Cleaning up API LLM provider...")
	p.modelMutex.Lock()
	defer p.modelMutex.Unlock()

	// Close HTTP client connections
	p.client.CloseIdleConnections()
	p.available = false
}

// GetModelInfo returns information about the current model
func (p *APILLMProvider) GetModelInfo() map[string]interface{} {
	return map[string]interface{}{
		"provider":    "api",
		"model":       p.config.ModelName,
		"endpoint":    p.endpoint,
		"available":   p.available,
		"temperature": p.config.Temperature,
		"maxTokens":   p.config.MaxTokens,
		"hasApiKey":   p.apiKey != "",
	}
}

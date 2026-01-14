// Package shared provides common types and utilities for LLM providers
package shared

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StreamingClient handles SSE streaming for LLM APIs
type StreamingClient struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
}

// NewStreamingClient creates a new streaming client for LLM APIs
func NewStreamingClient(endpoint, apiKey string, timeout time.Duration) *StreamingClient {
	return &StreamingClient{
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   endpoint,
		apiKey:     apiKey,
	}
}

// StreamChatRequest is the request with streaming enabled
type StreamChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// StreamDelta represents the delta in a streaming response
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// StreamChoice represents a choice in streaming response
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

// StreamResponse represents a single SSE event
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// ChunkCallback is called for each parsed chunk
type ChunkCallback func(text string, done bool, finishReason string) error

// StreamChatCompletion sends a streaming chat completion request
func (c *StreamingClient) StreamChatCompletion(
	ctx context.Context,
	req *StreamChatRequest,
	callback ChunkCallback,
) error {
	// Ensure streaming is enabled
	req.Stream = true

	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr ErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read SSE stream with minimal buffering to avoid delays
	reader := bufio.NewReaderSize(resp.Body, 1) // Use size 1 to minimize buffering
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// Stream ended
				return nil
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		// Parse SSE line
		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		// Handle SSE data lines
		if strings.HasPrefix(lineStr, "data: ") {
			data := strings.TrimPrefix(lineStr, "data: ")

			// Check for stream end
			if data == "[DONE]" {
				return callback("", true, "stop")
			}

			// Parse JSON response
			var streamResp StreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				// Skip malformed JSON lines
				fmt.Printf("[StreamingClient] Warning: failed to parse SSE data: %v\n", err)
				continue
			}

			// Extract content from choices
			if len(streamResp.Choices) > 0 {
				choice := streamResp.Choices[0]
				content := choice.Delta.Content
				finishReason := choice.FinishReason

				// Check if this is the final chunk
				done := finishReason != ""

				if content != "" || done {
					if err := callback(content, done, finishReason); err != nil {
						return fmt.Errorf("callback error: %w", err)
					}
				}

				// If we got a finish reason, we're done
				if done {
					return nil
				}
			}
		}
	}
}

// Close closes idle connections
func (c *StreamingClient) Close() {
	c.httpClient.CloseIdleConnections()
}

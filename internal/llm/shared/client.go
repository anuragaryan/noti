package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides HTTP client functionality for LLM APIs
type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
}

// NewClient creates a new HTTP client for LLM APIs
func NewClient(endpoint, apiKey string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   endpoint,
		apiKey:     apiKey,
	}
}

// ChatCompletion sends a chat completion request to the API
func (c *Client) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	ApplyAuthHeaders(httpReq.Header, c.apiKey)

	// Send request
	resp, err := c.httpClient.Do(httpReq)
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
		var apiErr ErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml") {
		preview := strings.TrimSpace(string(body))
		if len(preview) > 220 {
			preview = preview[:220] + "..."
		}
		return nil, fmt.Errorf("API returned HTML instead of JSON. Check API endpoint/base URL and auth. response preview: %s", preview)
	}

	// Parse response
	var apiResp ChatResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		preview := strings.TrimSpace(string(body))
		if len(preview) > 220 {
			preview = preview[:220] + "..."
		}
		return nil, fmt.Errorf("failed to parse response: %w (response preview: %s)", err, preview)
	}

	return &apiResp, nil
}

// Close closes idle connections
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}

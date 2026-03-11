package domain

// LLMConfig represents the LLM configuration
type LLMConfig struct {
	Provider    string  `json:"provider"`    // "local" or "api"
	ModelName   string  `json:"modelName"`   // e.g., "gemma-2b-it-q4_k_m" or "gpt-4"
	APIEndpoint string  `json:"apiEndpoint"` // For API provider
	APIKey      string  `json:"apiKey"`      // For API provider
	Temperature float32 `json:"temperature"` // 0.0 to 2.0
	MaxTokens   int     `json:"maxTokens"`   // Max response length
}

// LLMRequest represents a text generation request
type LLMRequest struct {
	Prompt       string `json:"prompt"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
}

// LLMResponse represents the generated text response
type LLMResponse struct {
	Text         string `json:"text"`
	TokensUsed   int    `json:"tokensUsed"`
	Model        string `json:"model"`
	FinishReason string `json:"finishReason"`
}

// StreamChunk represents a single chunk from streaming response
type StreamChunk struct {
	Text          string `json:"text"`
	ReasoningText string `json:"reasoningText,omitempty"`
	Index         int    `json:"index"`
	FinishReason  string `json:"finishReason,omitempty"`
	Done          bool   `json:"done"`
}

// StreamCallback is called for each chunk during streaming
type StreamCallback func(chunk *StreamChunk) error

package domain

import "time"

// Prompt represents a custom LLM prompt template
type Prompt struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"systemPrompt"`
	UserPrompt   string    `json:"userPrompt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// PromptExecutionRequest represents a request to execute a prompt on content
type PromptExecutionRequest struct {
	PromptID string `json:"promptId"`
	Content  string `json:"content"`
}

// PromptExecutionResult represents the result of executing a prompt
type PromptExecutionResult struct {
	PromptName  string       `json:"promptName"`
	Input       string       `json:"input"`
	Output      string       `json:"output"`
	TokensUsed  int          `json:"tokensUsed"`
	ExecutedAt  time.Time    `json:"executedAt"`
	LLMResponse *LLMResponse `json:"llmResponse"`
}

package shared

import (
	"net/http"
	"noti/internal/domain"
	"strings"
)

// BuildMessages creates a messages array from an LLM request
// This eliminates duplicate code between API and Local providers
func BuildMessages(request *domain.LLMRequest) []Message {
	messages := []Message{}

	// Add system message if provided
	if request.SystemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: request.SystemPrompt,
		})
	}

	// Add user message
	messages = append(messages, Message{
		Role:    "user",
		Content: request.Prompt,
	})

	return messages
}

func ApplyAuthHeaders(headers http.Header, apiKey string) {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return
	}

	bearer := trimmed
	if !strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
		bearer = "Bearer " + trimmed
	}

	headers.Set("Authorization", bearer)
	headers.Set("api-key", trimmed)
	headers.Set("x-api-key", trimmed)
}

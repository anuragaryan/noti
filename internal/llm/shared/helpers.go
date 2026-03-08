package shared

import "noti/internal/domain"

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

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

// GetEffectiveTemperature returns the request temperature or falls back to config default
func GetEffectiveTemperature(requestTemp, configTemp float32) float32 {
	if requestTemp == 0 {
		return configTemp
	}
	return requestTemp
}

// GetEffectiveMaxTokens returns the request max tokens or falls back to config default
func GetEffectiveMaxTokens(requestTokens, configTokens int) int {
	if requestTokens == 0 {
		return configTokens
	}
	return requestTokens
}

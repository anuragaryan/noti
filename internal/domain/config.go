package domain

// Config represents the application configuration
type Config struct {
	RealtimeTranscriptionChunkSeconds int       `json:"realtimeTranscriptionChunkSeconds"`
	ModelName                         string    `json:"modelName"`
	LLM                               LLMConfig `json:"llm"`
}

package domain

// STTConfig represents STT configuration
type STTConfig struct {
	ModelName         string `json:"modelName"`
	ChunkDurationSecs int    `json:"chunkDurationSecs"`
}

// TranscriptionResult represents a transcription with metadata
type TranscriptionResult struct {
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Duration  float64 `json:"duration"`
	Timestamp string  `json:"timestamp"`
	IsPartial bool    `json:"isPartial"` // True for real-time chunks
}

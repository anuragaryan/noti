package domain

// Config represents the application configuration
type Config struct {
	RealtimeTranscriptionChunkSeconds int           `json:"realtimeTranscriptionChunkSeconds"`
	ModelName                         string        `json:"modelName"`
	LLM                               LLMConfig     `json:"llm"`
	Audio                             AudioSettings `json:"audio"`
}

// AudioSettings holds audio capture configuration
type AudioSettings struct {
	// DefaultSource is the default audio source: "microphone", "system", or "mixed"
	DefaultSource string `json:"defaultSource"`
	// SampleRate is the audio sample rate in Hz (default 16000 for speech)
	SampleRate int `json:"sampleRate"`
	// Mixer holds the audio mixing configuration
	Mixer AudioMixerConfig `json:"mixer"`
}

// DefaultAudioSettings returns the default audio settings
func DefaultAudioSettings() AudioSettings {
	return AudioSettings{
		DefaultSource: "microphone",
		SampleRate:    16000,
		Mixer:         DefaultMixerConfig(),
	}
}

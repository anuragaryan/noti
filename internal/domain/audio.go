// Package domain contains core domain types and interfaces
package domain

// AudioSource represents the type of audio input
type AudioSource int

const (
	// AudioSourceMicrophone captures audio from the microphone
	AudioSourceMicrophone AudioSource = iota
	// AudioSourceSystem captures system audio output
	AudioSourceSystem
	// AudioSourceMixed captures both microphone and system audio simultaneously
	AudioSourceMixed
)

// String returns the string representation of AudioSource
func (s AudioSource) String() string {
	switch s {
	case AudioSourceMicrophone:
		return "microphone"
	case AudioSourceSystem:
		return "system"
	case AudioSourceMixed:
		return "mixed"
	default:
		return "unknown"
	}
}

// DisplayName returns a human-readable name for the audio source
func (s AudioSource) DisplayName() string {
	switch s {
	case AudioSourceMicrophone:
		return "Microphone"
	case AudioSourceSystem:
		return "System Audio"
	case AudioSourceMixed:
		return "Mixed (Mic + System)"
	default:
		return "Unknown"
	}
}

// ParseAudioSource converts a string to AudioSource
func ParseAudioSource(s string) AudioSource {
	switch s {
	case "microphone":
		return AudioSourceMicrophone
	case "system":
		return AudioSourceSystem
	case "mixed":
		return AudioSourceMixed
	default:
		return AudioSourceMicrophone
	}
}

// AudioSourceFromString is an alias for ParseAudioSource for convenience
func AudioSourceFromString(s string) AudioSource {
	return ParseAudioSource(s)
}

// AudioMixerConfig holds configuration for mixing multiple audio sources
type AudioMixerConfig struct {
	// MicrophoneGain is the gain multiplier for microphone audio (0.0 to 2.0, default 1.0)
	MicrophoneGain float32 `json:"microphoneGain"`
	// SystemGain is the gain multiplier for system audio (0.0 to 2.0, default 1.0)
	SystemGain float32 `json:"systemGain"`
	// MixMode determines how audio sources are combined: "sum" or "average"
	MixMode string `json:"mixMode"`
}

// DefaultMixerConfig returns the default mixer configuration
func DefaultMixerConfig() AudioMixerConfig {
	return AudioMixerConfig{
		MicrophoneGain: 1.0,
		SystemGain:     1.0,
		MixMode:        "sum",
	}
}

// AudioDevice represents an available audio device
type AudioDevice struct {
	// ID is the unique identifier for the device
	ID string `json:"id"`
	// Name is the human-readable name of the device
	Name string `json:"name"`
	// Source indicates whether this is a microphone or system audio device
	Source AudioSource `json:"source"`
	// IsDefault indicates if this is the default device for its type
	IsDefault bool `json:"isDefault"`
	// SampleRate is the device's native sample rate in Hz
	SampleRate int `json:"sampleRate"`
	// Channels is the number of audio channels
	Channels int `json:"channels"`
}

// AudioCaptureConfig holds audio capture configuration
type AudioCaptureConfig struct {
	// Source specifies which audio source to capture
	Source AudioSource `json:"source"`
	// DeviceID optionally specifies a specific device to use
	DeviceID string `json:"deviceId,omitempty"`
	// SampleRate is the desired sample rate in Hz (typically 16000 for speech)
	SampleRate int `json:"sampleRate"`
	// Channels is the number of audio channels (typically 1 for speech)
	Channels int `json:"channels"`
	// BufferSize is the size of the audio buffer in samples
	BufferSize int `json:"bufferSize"`
}

// DefaultCaptureConfig returns the default capture configuration for speech
func DefaultCaptureConfig() AudioCaptureConfig {
	return AudioCaptureConfig{
		Source:     AudioSourceMicrophone,
		SampleRate: 16000, // Whisper expects 16kHz
		Channels:   1,     // Mono for speech
		BufferSize: 1024,
	}
}

// AudioChunk represents a chunk of captured audio data
type AudioChunk struct {
	// Data contains the audio samples as float32 values (-1.0 to 1.0)
	Data []float32
	// SampleRate is the sample rate of this chunk in Hz
	SampleRate int
	// Channels is the number of audio channels
	Channels int
	// Timestamp is the Unix timestamp in milliseconds when this chunk was captured
	Timestamp int64
}

// AudioCallback is called when audio data is available
type AudioCallback func(chunk AudioChunk)

// PermissionStatus represents the state of audio permissions
type PermissionStatus int

const (
	// PermissionUnknown indicates the permission status is not known
	PermissionUnknown PermissionStatus = iota
	// PermissionGranted indicates permission has been granted
	PermissionGranted
	// PermissionDenied indicates permission has been denied
	PermissionDenied
	// PermissionRestricted indicates permission is restricted by system policy
	PermissionRestricted
)

// String returns the string representation of PermissionStatus
func (p PermissionStatus) String() string {
	switch p {
	case PermissionGranted:
		return "granted"
	case PermissionDenied:
		return "denied"
	case PermissionRestricted:
		return "restricted"
	default:
		return "unknown"
	}
}

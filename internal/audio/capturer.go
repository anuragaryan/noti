// Package audio provides audio capture functionality
package audio

import (
	"runtime"

	"noti/internal/audio/microphone"
	"noti/internal/domain"
)

// NewMicrophoneCapturer creates a new microphone capturer
func NewMicrophoneCapturer() domain.AudioCapturer {
	return microphone.NewCapturer()
}

// NewSystemAudioCapturer creates a new system audio capturer for the current platform
// Returns nil if system audio capture is not supported on the current platform
func NewSystemAudioCapturer() domain.AudioCapturer {
	switch runtime.GOOS {
	case "darwin":
		return newDarwinSystemCapturer()
	case "windows":
		// TODO: Implement Windows WASAPI system audio capture
		return nil
	case "linux":
		// TODO: Implement Linux PulseAudio/PipeWire system audio capture
		return nil
	default:
		return nil
	}
}

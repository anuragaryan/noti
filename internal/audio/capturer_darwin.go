//go:build darwin

package audio

import (
	"noti/internal/audio/darwin"
	"noti/internal/domain"
)

// newDarwinSystemCapturer creates a new macOS system audio capturer
func newDarwinSystemCapturer() domain.AudioCapturer {
	return darwin.NewSystemAudioCapturer()
}

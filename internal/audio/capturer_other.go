//go:build !darwin

package audio

import "noti/internal/domain"

// newDarwinSystemCapturer is a stub for non-darwin platforms
func newDarwinSystemCapturer() domain.AudioCapturer {
	return nil
}

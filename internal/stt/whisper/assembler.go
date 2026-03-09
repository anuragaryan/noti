package whisper

import (
	"strings"
	"time"
)

// cleanTranscription removes known model artifacts and surrounding whitespace.
func cleanTranscription(text string) string {
	text = strings.ReplaceAll(text, "[BLANK_AUDIO]", "")
	return strings.TrimSpace(text)
}

// joinTranscript appends next to base while normalizing boundary whitespace.
func joinTranscript(base, next, joiner string) string {
	next = strings.TrimLeft(next, " \t\n")
	if next == "" {
		return strings.TrimRight(base, " \t\n")
	}
	if base == "" {
		return next
	}
	return strings.TrimRight(base, " \t\n") + joiner + next
}

// joinerFromPause returns a newline when the current pause is longer than the
// running average, otherwise a space.
func joinerFromPause(silence, pauseTotal time.Duration, pauseCount int) string {
	if pauseCount == 0 {
		return " "
	}
	avgPause := pauseTotal / time.Duration(pauseCount)
	if silence > avgPause {
		return "\n"
	}
	return " "
}

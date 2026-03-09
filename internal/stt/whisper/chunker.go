package whisper

import "time"

// shouldTranscribe decides whether the current segment should be transcribed
// and returns the joiner to use for any appended text. Caller must hold
// bufferMutex.
func (t *Transcriber) shouldTranscribe(now time.Time) (bool, bool, string, time.Duration) {
	silenceDuration := now.Sub(t.lastSpeechAt)
	segmentDuration := now.Sub(t.segmentStartAt)
	pauseTriggered := t.hasSpeechInSegment && silenceDuration > t.options.PauseAfter
	shouldTranscribe := t.baselineReady && (pauseTriggered ||
		(segmentDuration > t.options.MaxSegmentDuration && len(t.audioBuffer) > t.processedSamples))

	joiner := " "
	if pauseTriggered {
		joiner = joinerFromPause(silenceDuration, t.pauseTotal, t.pauseCount)
	}

	return shouldTranscribe, pauseTriggered, joiner, silenceDuration
}

// takeNextChunk copies the next transcribable chunk and advances processing
// pointers. Caller must hold bufferMutex.
func (t *Transcriber) takeNextChunk() ([]float32, bool) {
	totalSamples := len(t.audioBuffer)
	if totalSamples <= t.processedSamples {
		return nil, false
	}

	if totalSamples-t.processedSamples < t.options.MinTranscribeSamples {
		return nil, false
	}

	chunkStart := t.processedSamples
	chunkEnd := totalSamples
	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, t.audioBuffer[chunkStart:chunkEnd])

	t.processedSamples = chunkEnd
	t.hasSpeechInSegment = false
	t.segmentStartAt = t.clock.Now()

	return chunk, true
}

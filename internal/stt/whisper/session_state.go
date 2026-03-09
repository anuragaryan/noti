package whisper

import "time"

type stopSnapshot struct {
	buffer     []float32
	processed  int
	silence    time.Duration
	pauseTotal time.Duration
	pauseCount int
	baseText   string
	isRunning  bool
}

// resetSession clears all per-recording mutable state. Caller must hold
// bufferMutex.
func (t *Transcriber) resetSession(now time.Time) {
	t.audioBuffer = t.audioBuffer[:0]
	t.processedSamples = 0
	t.accumulatedTranscript = ""
	t.lastFinalText = ""
	t.isProcessing = true

	t.lastSpeechAt = now
	t.segmentStartAt = now
	t.hasSpeechInSegment = false
	t.noiseFloorRMS = 0
	t.speechThresholdRMS = 0
	t.baselineBuffer = nil
	t.baselineReady = false

	t.pauseTotal = 0
	t.pauseCount = 0
}

// stopSnapshotLocked creates an immutable snapshot for background finalization.
// Caller must hold bufferMutex.
func (t *Transcriber) stopSnapshotLocked(now time.Time) stopSnapshot {
	buffer := make([]float32, len(t.audioBuffer))
	copy(buffer, t.audioBuffer)

	return stopSnapshot{
		buffer:     buffer,
		processed:  t.processedSamples,
		silence:    now.Sub(t.lastSpeechAt),
		pauseTotal: t.pauseTotal,
		pauseCount: t.pauseCount,
		baseText:   t.accumulatedTranscript,
		isRunning:  t.isProcessing,
	}
}

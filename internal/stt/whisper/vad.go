package whisper

import "time"

// updateBaseline collects initial ambient samples and initializes adaptive
// speech thresholds. Caller must hold bufferMutex.
func (t *Transcriber) updateBaseline(chunk []float32) bool {
	if t.baselineReady {
		return true
	}

	t.baselineBuffer = append(t.baselineBuffer, chunk...)
	if len(t.baselineBuffer) < t.options.BaselineSamplesMin {
		return false
	}

	t.noiseFloorRMS = calculateRMS(t.baselineBuffer)
	t.speechThresholdRMS = clampSpeechThreshold(t.noiseFloorRMS*t.options.NoiseFloorMultiplier, t.options)
	t.baselineReady = true
	t.baselineBuffer = nil
	return true
}

// updateSpeechState updates pause detection state from the latest chunk. Caller
// must hold bufferMutex.
func (t *Transcriber) updateSpeechState(chunk []float32, now time.Time) {
	rms := calculateRMS(chunk)
	if rms <= t.speechThresholdRMS {
		return
	}

	t.lastSpeechAt = now
	t.hasSpeechInSegment = true
	t.noiseFloorRMS = t.noiseFloorRMS*0.9 + rms*0.1
	t.speechThresholdRMS = clampSpeechThreshold(t.noiseFloorRMS*t.options.NoiseFloorMultiplier, t.options)
}

// clampSpeechThreshold keeps the adaptive threshold within configured bounds.
func clampSpeechThreshold(threshold float32, opts Options) float32 {
	if threshold < opts.MinSpeechThresholdRMS {
		return opts.MinSpeechThresholdRMS
	}
	if threshold > opts.MaxSpeechThresholdRMS {
		return opts.MaxSpeechThresholdRMS
	}
	return threshold
}

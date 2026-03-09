package whisper

import "time"

// Clock provides time for deterministic tests.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Options controls segmentation and VAD behavior.
type Options struct {
	SampleRate            int
	PauseAfter            time.Duration
	MaxSegmentDuration    time.Duration
	PauseCheckInterval    time.Duration
	MinSpeechThresholdRMS float32
	MaxSpeechThresholdRMS float32
	NoiseFloorMultiplier  float32
	BaselineSamplesMin    int
	MinTranscribeSamples  int
	MinTailSamples        int
	Language              string
	Threads               uint
}

// DefaultOptions returns production defaults for Whisper transcription.
func DefaultOptions() Options {
	return Options{
		SampleRate:            16000,
		PauseAfter:            1 * time.Second,
		MaxSegmentDuration:    10 * time.Second,
		PauseCheckInterval:    100 * time.Millisecond,
		MinSpeechThresholdRMS: 0.005,
		MaxSpeechThresholdRMS: 0.1,
		NoiseFloorMultiplier:  2.0,
		BaselineSamplesMin:    16000 / 2,
		MinTranscribeSamples:  16000 / 4,
		MinTailSamples:        16000 / 2,
		Language:              "en",
		Threads:               2,
	}
}

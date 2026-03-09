// Package whisper provides speech-to-text functionality using the Whisper model.
package whisper

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	sampleRate            = 16000
	pauseAfter            = 1 * time.Second
	maxSegmentDuration    = 10 * time.Second
	pauseCheckInterval    = 100 * time.Millisecond
	minSpeechThresholdRMS = 0.005
	maxSpeechThresholdRMS = 0.1
	noiseFloorMultiplier  = 2.0
	baselineSamplesMin    = sampleRate / 2
)

// Transcriber performs real-time speech-to-text transcription. Audio data is
// fed via ProcessChunk; it is transcribed when the user pauses for >1 second
// or after a maximum segment duration. Each chunk's text is appended to a
// running transcript so the frontend always receives the complete session text.
type Transcriber struct {
	model      whisper.Model
	modelMutex sync.Mutex
	config     *domain.STTConfig
	modelPath  string
	ctx        context.Context

	// bufferMutex guards audioBuffer, processedSamples, accumulatedTranscript,
	// isProcessing, and stopProcessing. Must not be held while waiting on
	// processingWg to prevent deadlocks.
	bufferMutex           sync.Mutex
	audioBuffer           []float32
	processedSamples      int
	accumulatedTranscript string

	processingWg   sync.WaitGroup
	backgroundWg   sync.WaitGroup
	lastFinalText  string
	isProcessing   bool
	stopProcessing chan struct{}

	lastSpeechAt       time.Time
	segmentStartAt     time.Time
	hasSpeechInSegment bool
	noiseFloorRMS      float32
	speechThresholdRMS float32
	baselineBuffer     []float32
	baselineReady      bool
}

// NewTranscriber creates a Transcriber for the given model. It only checks
// that the model file exists; call Initialize to load it into memory.
func NewTranscriber(basePath string, config *domain.STTConfig) (*Transcriber, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
	modelPath := filepath.Join(basePath, "models", "stt", modelFileName)

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	return &Transcriber{
		config:      config,
		modelPath:   modelPath,
		audioBuffer: []float32{},
	}, nil
}

// SetContext sets the Wails runtime context used for emitting frontend events.
func (t *Transcriber) SetContext(ctx context.Context) {
	t.ctx = ctx
}

// Initialize loads the Whisper model into memory.
func (t *Transcriber) Initialize() error {
	model, err := whisper.New(t.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load whisper model: %w", err)
	}
	t.model = model
	return nil
}

// StartProcessing begins the real-time transcription loop. If a previous
// session is still active it is torn down first; the old goroutine exits via
// its closed stopProcessing channel.
//
// Callers must not call StartProcessing concurrently.
func (t *Transcriber) StartProcessing() error {
	t.bufferMutex.Lock()

	if t.isProcessing {
		close(t.stopProcessing)
		t.isProcessing = false
	}

	// Release the lock while draining previous-session goroutines so they can
	// call processingWg.Done() without contending on bufferMutex.
	t.bufferMutex.Unlock()
	t.processingWg.Wait()
	t.bufferMutex.Lock()

	t.audioBuffer = []float32{}
	t.processedSamples = 0
	t.accumulatedTranscript = ""
	t.stopProcessing = make(chan struct{})
	t.isProcessing = true

	t.lastSpeechAt = time.Now()
	t.segmentStartAt = time.Now()
	t.hasSpeechInSegment = false
	t.noiseFloorRMS = 0
	t.speechThresholdRMS = 0
	t.baselineBuffer = nil
	t.baselineReady = false

	t.bufferMutex.Unlock()

	go t.processAudioRealtime()
	return nil
}

// StopProcessing halts the real-time loop immediately and returns without waiting
// for final transcription. The transcription runs asynchronously in the background
// and emits a "transcription:done" event when complete.
func (t *Transcriber) StopProcessing() (*domain.TranscriptionResult, error) {
	t.bufferMutex.Lock()

	if !t.isProcessing {
		t.bufferMutex.Unlock()
		return &domain.TranscriptionResult{
			Language:  "en",
			IsPartial: false,
			Timestamp: time.Now().Format(time.RFC3339),
		}, nil
	}

	close(t.stopProcessing)
	t.isProcessing = false

	fullBuffer := make([]float32, len(t.audioBuffer))
	copy(fullBuffer, t.audioBuffer)
	processedAtStop := t.processedSamples

	t.bufferMutex.Unlock()

	// Spawn background goroutine for final transcription; return immediately.
	t.backgroundWg.Add(1)
	go func(buffer []float32, processed int) {
		defer t.backgroundWg.Done()
		// Wait for the last in-flight chunk goroutine to finish
		t.processingWg.Wait()

		t.bufferMutex.Lock()
		base := t.accumulatedTranscript
		t.bufferMutex.Unlock()

		// Transcribe the tail — audio recorded after the last full chunk boundary.
		remain := len(buffer) - processed
		if remain > sampleRate/2 {
			tailChunk := make([]float32, remain)
			copy(tailChunk, buffer[processed:])

			tailText, err := t.TranscribeThreadSafe(tailChunk)
			if err != nil {
				slog.Error("[stt] tail transcription error", "error", err)
			} else {
				tailText = cleanTranscription(tailText)
				if tailText != "" {
					if base != "" {
						base = strings.TrimRight(base, " \t\n")
						tailText = strings.TrimLeft(tailText, " \t\n")
						if tailText != "" {
							base = base + " " + tailText
						}
					} else {
						base = tailText
					}
				}
			}
		}

		// Fallback: if nothing was transcribed during the whole session
		if base == "" && processed == 0 && len(buffer) > sampleRate {
			text, err := t.TranscribeThreadSafe(buffer)
			if err != nil {
				slog.Error("[stt] full-buffer transcription error", "error", err)
			} else {
				base = cleanTranscription(text)
			}
		}

		// Record the final text so tests (which have no Wails context) can
		// inspect it after backgroundWg.Wait().
		t.bufferMutex.Lock()
		t.lastFinalText = base
		t.bufferMutex.Unlock()

		// Emit final transcription event to the frontend.
		if t.ctx != nil {
			evt := domain.TranscriptionResult{
				Text:      base,
				Language:  "en",
				IsPartial: false,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			runtime.EventsEmit(t.ctx, "transcription:done", evt)
		} else {
			slog.Info("[stt] transcription:done skipped: context not set", "text", base)
		}
	}(fullBuffer, processedAtStop)

	return &domain.TranscriptionResult{
		Language:  "en",
		IsPartial: false,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// ProcessChunk appends an incoming audio chunk to the internal buffer and
// analyzes it for speech activity. It is a no-op when processing has not been started.
func (t *Transcriber) ProcessChunk(chunk domain.AudioChunk) {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if !t.isProcessing {
		return
	}

	if !t.baselineReady {
		t.baselineBuffer = append(t.baselineBuffer, chunk.Data...)
		if len(t.baselineBuffer) >= baselineSamplesMin {
			t.noiseFloorRMS = calculateRMS(t.baselineBuffer)
			t.speechThresholdRMS = t.noiseFloorRMS * noiseFloorMultiplier
			if t.speechThresholdRMS < minSpeechThresholdRMS {
				t.speechThresholdRMS = minSpeechThresholdRMS
			} else if t.speechThresholdRMS > maxSpeechThresholdRMS {
				t.speechThresholdRMS = maxSpeechThresholdRMS
			}
			t.baselineReady = true
			t.baselineBuffer = nil
		}
		t.audioBuffer = append(t.audioBuffer, chunk.Data...)
		return
	}

	rms := calculateRMS(chunk.Data)
	if rms > t.speechThresholdRMS {
		t.lastSpeechAt = time.Now()
		t.hasSpeechInSegment = true
		t.noiseFloorRMS = t.noiseFloorRMS*0.9 + rms*0.1
		t.speechThresholdRMS = t.noiseFloorRMS * noiseFloorMultiplier
		if t.speechThresholdRMS < minSpeechThresholdRMS {
			t.speechThresholdRMS = minSpeechThresholdRMS
		} else if t.speechThresholdRMS > maxSpeechThresholdRMS {
			t.speechThresholdRMS = maxSpeechThresholdRMS
		}
	}

	t.audioBuffer = append(t.audioBuffer, chunk.Data...)
}

// processAudioRealtime monitors for speech pauses and triggers transcription.
// It fires when the user pauses for >1 second or after max segment duration.
func (t *Transcriber) processAudioRealtime() {
	t.bufferMutex.Lock()
	stop := t.stopProcessing
	t.segmentStartAt = time.Now()
	t.bufferMutex.Unlock()

	ticker := time.NewTicker(pauseCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			t.bufferMutex.Lock()
			now := time.Now()
			silenceDuration := now.Sub(t.lastSpeechAt)
			segmentDuration := now.Sub(t.segmentStartAt)
			baselineReady := t.baselineReady

			shouldTranscribe := baselineReady && ((t.hasSpeechInSegment && silenceDuration > pauseAfter) ||
				(segmentDuration > maxSegmentDuration && len(t.audioBuffer) > t.processedSamples))
			t.bufferMutex.Unlock()

			if shouldTranscribe {
				t.processNextChunk()
			}
		}
	}
}

// processNextChunk transcribes all unprocessed audio in the buffer.
// It is called when pause is detected or max segment duration is reached.
// Chunk goroutines are serialised through modelMutex so that transcript order
// is always preserved regardless of scheduling.
func (t *Transcriber) processNextChunk() {
	t.bufferMutex.Lock()

	totalSamples := len(t.audioBuffer)
	if totalSamples <= t.processedSamples {
		t.bufferMutex.Unlock()
		return
	}

	minTranscribeSamples := sampleRate / 4
	if totalSamples-t.processedSamples < minTranscribeSamples {
		t.bufferMutex.Unlock()
		return
	}

	chunkStart := t.processedSamples
	chunkEnd := totalSamples

	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, t.audioBuffer[chunkStart:chunkEnd])
	t.processedSamples = chunkEnd
	t.hasSpeechInSegment = false
	t.segmentStartAt = time.Now()

	t.bufferMutex.Unlock()

	t.processingWg.Add(1)
	go func(audioChunk []float32) {
		defer t.processingWg.Done()

		fullTranscript, err := t.transcribeAndAccumulate(audioChunk)
		if err != nil {
			slog.Error("[stt] chunk transcription error", "error", err)
			return
		}

		if fullTranscript == "" {
			return
		}

		if t.ctx != nil {
			result := domain.TranscriptionResult{
				Text:      fullTranscript,
				Language:  "en",
				IsPartial: true,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			runtime.EventsEmit(t.ctx, "transcription:partial", result)
		}
	}(chunk)
}

// TranscribeThreadSafe serialises access to the Whisper model, which is not
// safe for concurrent use, and delegates to transcribe.
func (t *Transcriber) TranscribeThreadSafe(audioData []float32) (string, error) {
	t.modelMutex.Lock()
	defer t.modelMutex.Unlock()
	return t.transcribe(audioData)
}

// transcribeAndAccumulate runs Whisper inference and appends the result to
// accumulatedTranscript under a single lock to guarantee ordering.
// Returns the full accumulated transcript (or empty if no new text) and any error.
func (t *Transcriber) transcribeAndAccumulate(audioChunk []float32) (string, error) {
	t.modelMutex.Lock()
	defer t.modelMutex.Unlock()

	text, err := t.transcribe(audioChunk)
	if err != nil {
		return "", err
	}

	text = cleanTranscription(text)
	if text == "" {
		return "", nil
	}

	t.bufferMutex.Lock()
	prev := t.accumulatedTranscript
	if prev != "" {
		prev = strings.TrimRight(prev, " \t\n")
		text = strings.TrimLeft(text, " \t\n")
		if text != "" {
			t.accumulatedTranscript = prev + " " + text
		} else {
			t.accumulatedTranscript = prev
		}
	} else {
		t.accumulatedTranscript = text
	}
	fullTranscript := t.accumulatedTranscript
	t.bufferMutex.Unlock()

	return fullTranscript, nil
}

// transcribe runs Whisper inference on audioData. Callers must hold modelMutex.
func (t *Transcriber) transcribe(audioData []float32) (string, error) {
	if t.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	ctx, err := t.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("failed to create whisper context: %w", err)
	}

	ctx.SetLanguage("en")
	ctx.SetTranslate(false)
	ctx.SetThreads(2)

	if err := ctx.Process(audioData, nil, nil, nil); err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	var sb strings.Builder
	for {
		segment, err := ctx.NextSegment()
		if err != nil {
			break
		}
		sb.WriteString(segment.Text)
	}

	return sb.String(), nil
}

// cleanTranscription removes known Whisper output artifacts (e.g. silence
// tokens) and trims surrounding whitespace.
func cleanTranscription(text string) string {
	text = strings.ReplaceAll(text, "[BLANK_AUDIO]", "")
	return strings.TrimSpace(text)
}

func calculateRMS(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sum float32
	for _, s := range samples {
		sum += s * s
	}
	return float32(math.Sqrt(float64(sum / float32(len(samples)))))
}

// CancelProcessing halts the real-time loop without emitting a
// "transcription:done" event. Use this when tearing down a session that should
// not produce a final result (e.g. when audio capture fails to start).
func (t *Transcriber) CancelProcessing() {
	t.bufferMutex.Lock()
	if t.isProcessing {
		close(t.stopProcessing)
		t.isProcessing = false
	}
	t.bufferMutex.Unlock()
	t.processingWg.Wait()
}

// IsProcessing returns whether the real-time transcription loop is active.
func (t *Transcriber) IsProcessing() bool {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	return t.isProcessing
}

// Cleanup waits for in-flight transcription goroutines to finish and closes
// the Whisper model.
func (t *Transcriber) Cleanup() {
	t.processingWg.Wait()
	t.backgroundWg.Wait()
	if t.model != nil {
		t.model.Close()
	}
}

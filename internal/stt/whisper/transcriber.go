// Package whisper provides speech-to-text functionality using the Whisper model.
package whisper

import (
	"context"
	"fmt"
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
	transcriptionSampleRate = 16000
)

// Transcriber performs real-time speech-to-text transcription. Audio data is
// fed via ProcessChunk; it is sliced into fixed-size windows and transcribed
// sequentially. Each chunk's text is appended to a running transcript so the
// frontend always receives the complete session text, not just the last chunk.
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
	samplesPerChunk       int
	// processingWg tracks in-flight chunk goroutines. Chunk goroutines must
	// run sequentially (serialised by modelMutex) so transcript order is
	// preserved; processingWg lets StopProcessing wait for the last one.
	processingWg   sync.WaitGroup
	isProcessing   bool
	stopProcessing chan struct{}
}

// NewTranscriber creates a Transcriber for the given model. It only checks
// that the model file exists; call Initialize to load it into memory.
func NewTranscriber(basePath string, config *domain.STTConfig) (*Transcriber, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
	modelPath := filepath.Join(basePath, "models", modelFileName)

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	return &Transcriber{
		config:          config,
		modelPath:       modelPath,
		audioBuffer:     []float32{},
		samplesPerChunk: transcriptionSampleRate * config.ChunkDurationSecs,
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

	t.bufferMutex.Unlock()

	go t.processAudioRealtime()
	return nil
}

// StopProcessing halts the real-time loop, waits for in-flight transcription
// goroutines to finish, transcribes any remaining tail audio, and returns the
// complete session transcript. It is idempotent when called while not active.
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

	// Wait for the last in-flight chunk goroutine to finish writing to
	// accumulatedTranscript before we read it.
	t.processingWg.Wait()

	t.bufferMutex.Lock()
	base := t.accumulatedTranscript
	t.bufferMutex.Unlock()

	// Transcribe the tail — audio recorded after the last full chunk boundary.
	remainingSamples := len(fullBuffer) - processedAtStop
	if remainingSamples > transcriptionSampleRate/2 {
		tailChunk := make([]float32, remainingSamples)
		copy(tailChunk, fullBuffer[processedAtStop:])

		tailText, err := t.TranscribeThreadSafe(tailChunk)
		if err != nil {
			fmt.Printf("[stt] tail transcription error: %v\n", err)
		} else {
			tailText = cleanTranscription(tailText)
			if tailText != "" {
				if base != "" {
					base = base + " " + tailText
				} else {
					base = tailText
				}
			}
		}
	}

	// If nothing was transcribed during the whole session (no chunks fired and
	// the tail was too short), fall back to the full buffer.
	if base == "" && processedAtStop == 0 && len(fullBuffer) > transcriptionSampleRate {
		text, err := t.TranscribeThreadSafe(fullBuffer)
		if err != nil {
			fmt.Printf("[stt] full-buffer transcription error: %v\n", err)
		} else {
			base = cleanTranscription(text)
		}
	}

	return &domain.TranscriptionResult{
		Text:      base,
		Language:  "en",
		IsPartial: false,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// ProcessChunk appends an incoming audio chunk to the internal buffer. It is a
// no-op when processing has not been started.
func (t *Transcriber) ProcessChunk(chunk domain.AudioChunk) {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if !t.isProcessing {
		return
	}
	t.audioBuffer = append(t.audioBuffer, chunk.Data...)
}

// processAudioRealtime fires a transcription pass every ChunkDurationSecs
// seconds until stopProcessing is closed.
func (t *Transcriber) processAudioRealtime() {
	// Capture the channel once so reads never race with StartProcessing
	// overwriting t.stopProcessing when resetting for a new session.
	t.bufferMutex.Lock()
	stop := t.stopProcessing
	t.bufferMutex.Unlock()

	ticker := time.NewTicker(time.Duration(t.config.ChunkDurationSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			t.processNextChunk()
		}
	}
}

// processNextChunk extracts the next unseen chunk from the audio buffer,
// transcribes it, appends the result to accumulatedTranscript, and emits the
// full running transcript as a partial event. It is a no-op when fewer than
// samplesPerChunk new samples are available.
//
// Chunk goroutines are serialised through modelMutex so that transcript order
// is always preserved regardless of scheduling.
func (t *Transcriber) processNextChunk() {
	t.bufferMutex.Lock()

	totalSamples := len(t.audioBuffer)
	if totalSamples < t.processedSamples+t.samplesPerChunk {
		t.bufferMutex.Unlock()
		return
	}

	chunkStart := t.processedSamples
	chunkEnd := chunkStart + t.samplesPerChunk
	if chunkEnd > totalSamples {
		chunkEnd = totalSamples
	}

	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, t.audioBuffer[chunkStart:chunkEnd])
	t.processedSamples = chunkEnd

	t.bufferMutex.Unlock()

	t.processingWg.Add(1)
	go func(audioChunk []float32) {
		defer t.processingWg.Done()

		// modelMutex serialises both inference and transcript accumulation so
		// chunks are always appended in the order they were recorded.
		text, err := t.TranscribeThreadSafe(audioChunk)
		if err != nil {
			fmt.Printf("[stt] chunk transcription error: %v\n", err)
			return
		}

		text = cleanTranscription(text)
		if text == "" {
			return
		}

		t.bufferMutex.Lock()
		if t.accumulatedTranscript != "" {
			t.accumulatedTranscript += " " + text
		} else {
			t.accumulatedTranscript = text
		}
		fullTranscript := t.accumulatedTranscript
		t.bufferMutex.Unlock()

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
	if t.model != nil {
		t.model.Close()
	}
}

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

	gowhisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

const sampleRate = 16000

// Transcriber performs real-time speech-to-text transcription.
type Transcriber struct {
	lifecycleMu sync.Mutex
	emitterMu   sync.RWMutex

	model      gowhisper.Model
	modelMutex sync.Mutex
	config     *domain.STTConfig
	modelPath  string
	ctx        context.Context

	options Options
	clock   Clock
	engine  speechEngine
	emitter transcriptionEmitter

	configuredLanguage string
	autoDetectLanguage bool
	lockedLanguage     string

	defaultsOnce sync.Once

	// bufferMutex guards session state.
	bufferMutex           sync.Mutex
	audioBuffer           []float32
	processedSamples      int
	accumulatedTranscript string

	processingWg sync.WaitGroup
	backgroundWg sync.WaitGroup
	realtimeWg   sync.WaitGroup

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

	pauseTotal time.Duration
	pauseCount int
}

// NewTranscriber creates a Transcriber for the given model path.
func NewTranscriber(basePath string, config *domain.STTConfig) (*Transcriber, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
	modelPath := filepath.Join(basePath, "models", "stt", modelFileName)

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	options := DefaultOptions()
	language, autoDetect := resolveSTTLanguage(config)
	options.Language = language

	return &Transcriber{
		config:             config,
		modelPath:          modelPath,
		audioBuffer:        []float32{},
		options:            options,
		clock:              realClock{},
		emitter:            noopEmitter{},
		configuredLanguage: language,
		autoDetectLanguage: autoDetect,
	}, nil
}

// SetContext sets the Wails runtime context used for emitting frontend events.
func (t *Transcriber) SetContext(ctx context.Context) {
	t.ensureDefaults()
	t.emitterMu.Lock()
	defer t.emitterMu.Unlock()
	t.ctx = ctx
	t.emitter = newWailsEmitter(ctx)
}

// Initialize loads the Whisper model into memory.
func (t *Transcriber) Initialize() error {
	model, err := gowhisper.New(t.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load whisper model: %w", err)
	}
	t.model = model
	t.engine = newWhisperEngine(model, t.options)
	return nil
}

// StartProcessing begins the real-time transcription loop.
func (t *Transcriber) StartProcessing() error {
	t.ensureDefaults()
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	t.bufferMutex.Lock()

	if t.isProcessing {
		close(t.stopProcessing)
		t.isProcessing = false
	}

	t.bufferMutex.Unlock()
	t.processingWg.Wait()
	t.realtimeWg.Wait()
	t.bufferMutex.Lock()

	now := t.clock.Now()
	t.resetSession(now)
	t.lockedLanguage = ""
	t.options.Language = t.configuredLanguage
	if t.model != nil {
		t.engine = newWhisperEngine(t.model, t.options)
	}
	t.stopProcessing = make(chan struct{})

	t.bufferMutex.Unlock()

	t.realtimeWg.Add(1)
	go t.processAudioRealtime()
	return nil
}

// StopProcessing halts the loop and schedules final transcription.
func (t *Transcriber) StopProcessing() (*domain.TranscriptionResult, error) {
	t.ensureDefaults()
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	t.bufferMutex.Lock()
	if !t.isProcessing {
		t.bufferMutex.Unlock()
		return t.newResult("", false), nil
	}
	close(t.stopProcessing)
	t.isProcessing = false
	t.bufferMutex.Unlock()
	t.realtimeWg.Wait()

	t.bufferMutex.Lock()
	snap := t.stopSnapshotLocked(t.clock.Now())
	t.bufferMutex.Unlock()

	t.backgroundWg.Add(1)
	go t.finalizeInBackground(snap)

	return t.newResult("", false), nil
}

// ProcessChunk appends and analyzes incoming audio.
func (t *Transcriber) ProcessChunk(chunk domain.AudioChunk) {
	t.ensureDefaults()
	if !t.validChunk(chunk) {
		return
	}

	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if !t.isProcessing {
		return
	}

	ready := t.updateBaseline(chunk.Data)
	t.audioBuffer = append(t.audioBuffer, chunk.Data...)
	if !ready {
		return
	}

	t.updateSpeechState(chunk.Data, t.clock.Now())
}

func (t *Transcriber) validChunk(chunk domain.AudioChunk) bool {
	if len(chunk.Data) == 0 {
		return false
	}

	if chunk.SampleRate != 0 && chunk.SampleRate != t.options.SampleRate {
		slog.Warn("[stt] dropping chunk: unexpected sample rate", "got", chunk.SampleRate, "want", t.options.SampleRate)
		return false
	}

	if chunk.Channels != 0 && chunk.Channels != 1 {
		slog.Warn("[stt] dropping chunk: unexpected channel count", "got", chunk.Channels, "want", 1)
		return false
	}

	return true
}

func (t *Transcriber) processAudioRealtime() {
	defer t.realtimeWg.Done()

	t.bufferMutex.Lock()
	stop := t.stopProcessing
	t.segmentStartAt = t.clock.Now()
	t.bufferMutex.Unlock()

	ticker := time.NewTicker(t.options.PauseCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			t.bufferMutex.Lock()
			now := t.clock.Now()
			shouldTranscribe, pauseTriggered, joiner, silenceDuration := t.shouldTranscribe(now)
			t.bufferMutex.Unlock()

			if !shouldTranscribe {
				continue
			}

			accepted := t.processNextChunk(joiner)
			if accepted && pauseTriggered {
				t.bufferMutex.Lock()
				t.pauseTotal += silenceDuration
				t.pauseCount++
				t.bufferMutex.Unlock()
			}
		}
	}
}

// processNextChunk transcribes all unprocessed audio in the buffer.
func (t *Transcriber) processNextChunk(joiner string) bool {
	t.ensureDefaults()
	t.bufferMutex.Lock()
	chunk, accepted := t.takeNextChunk()
	t.bufferMutex.Unlock()
	if !accepted {
		return false
	}

	t.processingWg.Add(1)
	go func(audioChunk []float32, joiner string) {
		defer t.processingWg.Done()

		fullTranscript, err := t.transcribeAndAccumulate(audioChunk, joiner)
		if err != nil {
			slog.Error("[stt] chunk transcription error", "error", err)
			return
		}

		if fullTranscript == "" {
			return
		}

		t.currentEmitter().EmitPartial(*t.newResult(fullTranscript, true))
	}(chunk, joiner)

	return true
}

// TranscribeThreadSafe serialises access to Whisper model.
func (t *Transcriber) TranscribeThreadSafe(audioData []float32) (string, error) {
	t.ensureDefaults()
	t.modelMutex.Lock()
	defer t.modelMutex.Unlock()
	return t.transcribe(audioData)
}

// transcribeAndAccumulate appends recognized text to transcript in order.
func (t *Transcriber) transcribeAndAccumulate(audioChunk []float32, joiner string) (string, error) {
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
	t.accumulatedTranscript = joinTranscript(t.accumulatedTranscript, text, joiner)
	fullTranscript := t.accumulatedTranscript
	t.bufferMutex.Unlock()

	return fullTranscript, nil
}

// transcribe runs Whisper inference. Callers must hold modelMutex.
func (t *Transcriber) transcribe(audioData []float32) (string, error) {
	t.ensureDefaults()
	if t.engine == nil && t.model != nil {
		t.engine = newWhisperEngine(t.model, t.options)
	}
	if t.engine == nil {
		return "", fmt.Errorf("model not initialized")
	}

	text, err := t.engine.Transcribe(audioData)
	if err != nil {
		return "", err
	}

	t.lockDetectedLanguage()
	return text, nil
}

func (t *Transcriber) finalizeInBackground(snap stopSnapshot) {
	defer t.backgroundWg.Done()

	t.processingWg.Wait()

	text := snap.baseText
	remain := len(snap.buffer) - snap.processed
	if remain > t.options.MinTailSamples {
		tailChunk := make([]float32, remain)
		copy(tailChunk, snap.buffer[snap.processed:])

		tailText, err := t.TranscribeThreadSafe(tailChunk)
		if err != nil {
			slog.Error("[stt] tail transcription error", "error", err)
		} else {
			tailText = cleanTranscription(tailText)
			if tailText != "" {
				text = joinTranscript(text, tailText, joinerFromPause(snap.silence, snap.pauseTotal, snap.pauseCount))
			}
		}
	}

	if text == "" && snap.processed == 0 && len(snap.buffer) > t.options.SampleRate {
		fullText, err := t.TranscribeThreadSafe(snap.buffer)
		if err != nil {
			slog.Error("[stt] full-buffer transcription error", "error", err)
		} else {
			text = cleanTranscription(fullText)
		}
	}

	t.bufferMutex.Lock()
	t.lastFinalText = text
	t.bufferMutex.Unlock()

	ctx, emitter := t.eventTargets()
	if ctx == nil {
		slog.Info("[stt] transcription:done skipped: context not set", "text", text)
		return
	}

	emitter.EmitDone(*t.newResult(text, false))
}

func (t *Transcriber) newResult(text string, partial bool) *domain.TranscriptionResult {
	t.ensureDefaults()
	language := t.options.Language
	if t.lockedLanguage != "" {
		language = t.lockedLanguage
	}
	return &domain.TranscriptionResult{
		Text:      text,
		Language:  language,
		IsPartial: partial,
		Timestamp: t.clock.Now().Format(time.RFC3339),
	}
}

// CancelProcessing halts the real-time loop without done event.
func (t *Transcriber) CancelProcessing() {
	t.ensureDefaults()
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	t.bufferMutex.Lock()
	if t.isProcessing {
		close(t.stopProcessing)
		t.isProcessing = false
	}
	t.bufferMutex.Unlock()
	t.processingWg.Wait()
	t.realtimeWg.Wait()
}

// IsProcessing returns whether the real-time loop is active.
func (t *Transcriber) IsProcessing() bool {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	return t.isProcessing
}

// Cleanup waits for in-flight transcription goroutines and closes the model.
func (t *Transcriber) Cleanup() {
	t.ensureDefaults()
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	t.bufferMutex.Lock()
	if t.isProcessing {
		close(t.stopProcessing)
		t.isProcessing = false
	}
	t.bufferMutex.Unlock()
	t.processingWg.Wait()
	t.realtimeWg.Wait()
	t.backgroundWg.Wait()
	if t.engine != nil {
		if err := t.engine.Close(); err != nil {
			slog.Warn("[stt] failed to close whisper engine", "error", err)
		}
		return
	}
	if t.model != nil {
		_ = t.model.Close()
	}
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

func (t *Transcriber) ensureDefaults() {
	t.defaultsOnce.Do(func() {
		if t.options.SampleRate == 0 {
			t.options = DefaultOptions()
		}
		if t.clock == nil {
			t.clock = realClock{}
		}
	})
}

func (t *Transcriber) eventTargets() (context.Context, transcriptionEmitter) {
	t.emitterMu.RLock()
	defer t.emitterMu.RUnlock()

	emitter := t.emitter
	if emitter == nil {
		emitter = noopEmitter{}
	}

	return t.ctx, emitter
}

func (t *Transcriber) currentEmitter() transcriptionEmitter {
	_, emitter := t.eventTargets()
	return emitter
}

func (t *Transcriber) lockDetectedLanguage() {
	if !t.autoDetectLanguage || t.lockedLanguage != "" || t.engine == nil {
		return
	}

	detected := strings.TrimSpace(strings.ToLower(t.engine.DetectedLanguage()))
	if detected == "" || detected == "auto" {
		return
	}

	t.lockedLanguage = detected
	t.options.Language = detected
	if t.model != nil {
		t.engine = newWhisperEngine(t.model, t.options)
	}
}

func resolveSTTLanguage(config *domain.STTConfig) (string, bool) {
	if config == nil {
		return "en", false
	}

	if strings.Contains(config.ModelName, ".en") {
		return "en", false
	}

	language := strings.TrimSpace(strings.ToLower(config.Language))
	if language == "" {
		return "en", false
	}

	if language == "auto" {
		return "auto", true
	}

	return language, false
}

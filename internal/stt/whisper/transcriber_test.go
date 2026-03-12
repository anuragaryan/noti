package whisper

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"noti/internal/domain"

	gowhisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// ── Mock types ────────────────────────────────────────────────────────────────

// mockModel implements whisper.Model for testing.
type mockModel struct {
	newContextFn func() (gowhisper.Context, error)
	closed       bool
}

func (m *mockModel) NewContext() (gowhisper.Context, error) {
	if m.newContextFn != nil {
		return m.newContextFn()
	}
	return &mockContext{}, nil
}

func (m *mockModel) Close() error {
	m.closed = true
	return nil
}
func (m *mockModel) IsMultilingual() bool { return false }
func (m *mockModel) Language() string     { return "en" }
func (m *mockModel) Languages() []string  { return []string{"en"} }

// mockContext implements whisper.Context for testing.
type mockContext struct {
	segments []gowhisper.Segment
	idx      int
}

func (c *mockContext) SetLanguage(lang string) error { return nil }
func (c *mockContext) SetTranslate(translate bool)   {}
func (c *mockContext) SetThreads(threads uint)       {}
func (c *mockContext) Process(data []float32, encCb gowhisper.EncoderBeginCallback, cb gowhisper.SegmentCallback, progressCb gowhisper.ProgressCallback) error {
	return nil
}
func (c *mockContext) NextSegment() (gowhisper.Segment, error) {
	if c.idx >= len(c.segments) {
		return gowhisper.Segment{}, os.ErrNotExist // any error stops iteration
	}
	seg := c.segments[c.idx]
	c.idx++
	return seg, nil
}
func (c *mockContext) ResetTimings()                       {}
func (c *mockContext) PrintTimings()                       {}
func (c *mockContext) Free()                               {}
func (c *mockContext) IsMultilingual() bool                { return false }
func (c *mockContext) Language() string                    { return "en" }
func (c *mockContext) DetectedLanguage() string            { return "en" }
func (c *mockContext) SetOffset(time.Duration)             {}
func (c *mockContext) SetDuration(time.Duration)           {}
func (c *mockContext) SetSplitOnWord(bool)                 {}
func (c *mockContext) SetTokenThreshold(float32)           {}
func (c *mockContext) SetTokenSumThreshold(float32)        {}
func (c *mockContext) SetMaxSegmentLength(uint)            {}
func (c *mockContext) SetTokenTimestamps(bool)             {}
func (c *mockContext) SetMaxTokensPerSegment(uint)         {}
func (c *mockContext) SetAudioCtx(uint)                    {}
func (c *mockContext) SetMaxContext(n int)                 {}
func (c *mockContext) SetBeamSize(n int)                   {}
func (c *mockContext) SetEntropyThold(t float32)           {}
func (c *mockContext) SetInitialPrompt(prompt string)      {}
func (c *mockContext) SetTemperature(t float32)            {}
func (c *mockContext) SetTemperatureFallback(t float32)    {}
func (c *mockContext) IsBEG(gowhisper.Token) bool          { return false }
func (c *mockContext) IsSOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsEOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsPREV(gowhisper.Token) bool         { return false }
func (c *mockContext) IsSOLM(gowhisper.Token) bool         { return false }
func (c *mockContext) IsNOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsLANG(gowhisper.Token, string) bool { return false }
func (c *mockContext) IsText(gowhisper.Token) bool         { return false }
func (c *mockContext) SystemInfo() string                  { return "" }

// ── Test helpers ──────────────────────────────────────────────────────────────

// modelWithSegments returns a mockModel whose NewContext always yields the
// given text segments.
func modelWithSegments(texts ...string) *mockModel {
	segs := make([]gowhisper.Segment, len(texts))
	for i, t := range texts {
		segs[i] = gowhisper.Segment{Text: t}
	}
	return &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: segs}, nil
	}}
}

// buildTranscriber returns a Transcriber backed by m, bypassing the
// constructor to avoid needing a real model file.
func buildTranscriber(t *testing.T, m *mockModel) *Transcriber {
	t.Helper()
	cfg := &domain.STTConfig{ModelName: "tiny", ChunkDurationSecs: 1}
	tr := &Transcriber{
		config:      cfg,
		audioBuffer: []float32{},
	}
	if m != nil {
		tr.model = m
	}
	return tr
}

// startProcessingTranscriber returns a Transcriber that has isProcessing=true
// and a live stopProcessing channel, simulating a mid-session state without
// launching the real-time goroutine.
func startProcessingTranscriber(t *testing.T, m *mockModel) *Transcriber {
	t.Helper()
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})
	return tr
}

// stopAndWait calls StopProcessing and waits for the background goroutine to
// finish, then returns lastFinalText under the buffer lock.
func stopAndWait(t *testing.T, tr *Transcriber) (string, error) {
	t.Helper()
	_, err := tr.StopProcessing()
	if err != nil {
		return "", err
	}
	tr.backgroundWg.Wait()
	tr.bufferMutex.Lock()
	got := tr.lastFinalText
	tr.bufferMutex.Unlock()
	return got, nil
}

// cleanupStartProcessing registers a t.Cleanup that tears down an active
// processing session, preventing goroutine leaks in StartProcessing tests.
func cleanupStartProcessing(t *testing.T, tr *Transcriber) {
	t.Helper()
	t.Cleanup(func() {
		tr.bufferMutex.Lock()
		if tr.isProcessing {
			close(tr.stopProcessing)
			tr.isProcessing = false
		}
		tr.bufferMutex.Unlock()
		tr.processingWg.Wait()
	})
}

// writeDummyModel creates the models directory under base and writes a dummy
// model file for modelName, returning the full path.
func writeDummyModel(t *testing.T, base, modelName string) string {
	t.Helper()
	dir := filepath.Join(base, "models", "stt")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ggml-"+modelName+".bin")
	if err := os.WriteFile(path, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── NewTranscriber ────────────────────────────────────────────────────────────

func TestNewTranscriber_ModelNotFound(t *testing.T) {
	cfg := &domain.STTConfig{ModelName: "missing", ChunkDurationSecs: 1}
	_, err := NewTranscriber(t.TempDir(), cfg)
	if err == nil {
		t.Fatal("expected error when model file is missing, got nil")
	}
}

func TestNewTranscriber_Success(t *testing.T) {
	base := t.TempDir()
	writeDummyModel(t, base, "tiny")

	cfg := &domain.STTConfig{ModelName: "tiny", ChunkDurationSecs: 3}
	tr, err := NewTranscriber(base, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil Transcriber")
	}
	if tr.config == nil || tr.config.ModelName != "tiny" {
		t.Error("expected config to be set with correct model name")
	}
}

// ── IsProcessing ──────────────────────────────────────────────────────────────

func TestIsProcessing_DefaultFalse(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	if tr.IsProcessing() {
		t.Error("IsProcessing() should be false on a new Transcriber")
	}
}

// ── ProcessChunk ──────────────────────────────────────────────────────────────

func TestProcessChunk_DropsWhenNotProcessing(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	tr.ProcessChunk(domain.AudioChunk{Data: []float32{0.1, 0.2, 0.3}})

	if len(tr.audioBuffer) != 0 {
		t.Errorf("audioBuffer should remain empty when not processing, got %d samples", len(tr.audioBuffer))
	}
}

func TestProcessChunk_AppendsWhenProcessing(t *testing.T) {
	tr := startProcessingTranscriber(t, &mockModel{})

	tr.ProcessChunk(domain.AudioChunk{Data: []float32{0.1, 0.2, 0.3}})

	if len(tr.audioBuffer) != 3 {
		t.Errorf("audioBuffer length = %d, want 3", len(tr.audioBuffer))
	}
}

func TestProcessChunk_AppendsConcurrently(t *testing.T) {
	tr := startProcessingTranscriber(t, &mockModel{})

	const goroutines = 20
	const samplesEach = 50

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.ProcessChunk(domain.AudioChunk{Data: make([]float32, samplesEach)})
		}()
	}
	wg.Wait()

	tr.bufferMutex.Lock()
	got := len(tr.audioBuffer)
	tr.bufferMutex.Unlock()

	if got != goroutines*samplesEach {
		t.Errorf("audioBuffer length = %d, want %d", got, goroutines*samplesEach)
	}
}

// ── transcribe (via TranscribeThreadSafe) ─────────────────────────────────────

func TestTranscriberTranscribe_ModelNil(t *testing.T) {
	tr := buildTranscriber(t, nil)

	_, err := tr.transcribe([]float32{0.1})
	if err == nil {
		t.Fatal("expected error when model is nil")
	}
}

func TestTranscriberTranscribe_ConcatenatesSegments(t *testing.T) {
	tr := buildTranscriber(t, modelWithSegments("foo ", "bar"))

	got, err := tr.transcribe(make([]float32, 100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "foo bar" {
		t.Errorf("transcribe() = %q, want %q", got, "foo bar")
	}
}

func TestTranscribeThreadSafe_SerializesConcurrentAccess(t *testing.T) {
	tr := buildTranscriber(t, modelWithSegments("ok"))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, err := tr.TranscribeThreadSafe(make([]float32, 100))
			if err != nil {
				t.Errorf("TranscribeThreadSafe error: %v", err)
			}
			if text != "ok" {
				t.Errorf("TranscribeThreadSafe() = %q, want %q", text, "ok")
			}
		}()
	}
	wg.Wait()
}

// ── StopProcessing ────────────────────────────────────────────────────────────

func TestStopProcessing_Idempotent(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if result.IsPartial {
		t.Error("result should not be partial")
	}
	if result.Language != "en" {
		t.Errorf("Language = %q, want %q", result.Language, "en")
	}
}

func TestStopProcessing_ReturnsEmptyWhenBufferEmpty(t *testing.T) {
	tr := startProcessingTranscriber(t, &mockModel{})

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("lastFinalText = %q, want empty", got)
	}
}

func TestStopProcessing_ProcessesTailBuffer(t *testing.T) {
	// 1.8 s in buffer, 1 s already processed → 0.8 s tail (> 0.5 s threshold).
	tr := startProcessingTranscriber(t, modelWithSegments("tail"))
	tr.audioBuffer = make([]float32, int(1.8*float64(sampleRate)))
	tr.processedSamples = sampleRate
	tr.hasSpeechInSegment = true

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tail" {
		t.Errorf("lastFinalText = %q, want %q", got, "tail")
	}
}

func TestStopProcessing_FallbackFullBuffer(t *testing.T) {
	// Nothing processed yet and buffer > 1 s → full-buffer fallback.
	tr := startProcessingTranscriber(t, modelWithSegments("full"))
	tr.audioBuffer = make([]float32, sampleRate*2)
	tr.processedSamples = 0
	tr.speechDetected = true

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "full" {
		t.Errorf("lastFinalText = %q, want %q", got, "full")
	}
}

func TestStopProcessing_FallbackUsesMinThresholdWithoutVAD(t *testing.T) {
	tr := startProcessingTranscriber(t, modelWithSegments("full"))
	tr.audioBuffer = make([]float32, sampleRate*2)
	for i := range tr.audioBuffer {
		tr.audioBuffer[i] = 0.01
	}
	tr.processedSamples = 0
	tr.speechThresholdRMS = 0.05 // Simulate inflated adaptive threshold.

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "full" {
		t.Errorf("lastFinalText = %q, want %q", got, "full")
	}
}

func TestStopProcessing_SkipsTailWhenNoSpeechEvidence(t *testing.T) {
	tailCalled := false
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		tailCalled = true
		return &mockContext{segments: []gowhisper.Segment{{Text: "tail"}}}, nil
	}}
	tr := startProcessingTranscriber(t, m)
	tr.audioBuffer = make([]float32, int(1.8*float64(sampleRate)))
	tr.processedSamples = sampleRate

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tailCalled {
		t.Fatal("expected tail transcription to be skipped without speech evidence")
	}
	if got != "" {
		t.Errorf("lastFinalText = %q, want empty", got)
	}
}

func TestStopProcessing_SkipsFallbackWhenNoSpeechEvidence(t *testing.T) {
	fullCalled := false
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		fullCalled = true
		return &mockContext{segments: []gowhisper.Segment{{Text: "full"}}}, nil
	}}
	tr := startProcessingTranscriber(t, m)
	tr.audioBuffer = make([]float32, sampleRate*2)
	tr.processedSamples = 0

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullCalled {
		t.Fatal("expected full-buffer fallback to be skipped without speech evidence")
	}
	if got != "" {
		t.Errorf("lastFinalText = %q, want empty", got)
	}
}

func TestStopProcessing_ShortTailSkipped(t *testing.T) {
	// Tail is exactly 0.5 s — not strictly greater — so it is skipped.
	tr := startProcessingTranscriber(t, modelWithSegments("should not appear"))
	// 1 s processed; total = 1.5 s → tail = 0.5 s (not > 0.5 s)
	tr.audioBuffer = make([]float32, int(1.5*float64(sampleRate)))
	tr.processedSamples = sampleRate

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// processedSamples != 0 so the full-buffer fallback is also skipped.
	if got != "" {
		t.Errorf("lastFinalText = %q, want empty (tail too short)", got)
	}
}

// ── StartProcessing ───────────────────────────────────────────────────────────

func TestStartProcessing_SetsIsProcessing(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})

	if err := tr.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing() error: %v", err)
	}
	cleanupStartProcessing(t, tr)

	if !tr.IsProcessing() {
		t.Error("IsProcessing() should be true after StartProcessing()")
	}
}

func TestStartProcessing_ResetsBuffer(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	tr.audioBuffer = make([]float32, 1000)
	tr.processedSamples = 500
	tr.accumulatedTranscript = "leftover text"

	if err := tr.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing() error: %v", err)
	}
	cleanupStartProcessing(t, tr)

	tr.bufferMutex.Lock()
	bufLen := len(tr.audioBuffer)
	processed := tr.processedSamples
	accumulated := tr.accumulatedTranscript
	tr.bufferMutex.Unlock()

	if bufLen != 0 {
		t.Errorf("audioBuffer length = %d, want 0 after reset", bufLen)
	}
	if processed != 0 {
		t.Errorf("processedSamples = %d, want 0 after reset", processed)
	}
	if accumulated != "" {
		t.Errorf("accumulatedTranscript = %q, want empty after reset", accumulated)
	}
}

func TestStartProcessing_ResetRunningSession(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})

	if err := tr.StartProcessing(); err != nil {
		t.Fatalf("first StartProcessing() error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- tr.StartProcessing() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("second StartProcessing() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second StartProcessing() timed out – possible deadlock")
	}

	tr.bufferMutex.Lock()
	if tr.isProcessing {
		close(tr.stopProcessing)
		tr.isProcessing = false
	}
	tr.bufferMutex.Unlock()
	tr.processingWg.Wait()
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

func TestTranscriberCleanup_ClosesModel(t *testing.T) {
	m := &mockModel{}
	tr := buildTranscriber(t, m)

	tr.Cleanup()

	if !m.closed {
		t.Error("Cleanup() should close the model")
	}
}

func TestTranscriberCleanup_NilModel(t *testing.T) {
	tr := buildTranscriber(t, nil)
	// Should not panic.
	tr.Cleanup()
}

// ── SetContext ────────────────────────────────────────────────────────────────

func TestTranscriberSetContext(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	if tr.ctx != nil {
		t.Error("ctx should be nil initially")
	}
	tr.SetContext(nil)
	if tr.ctx != nil {
		t.Error("ctx should remain nil after SetContext(nil)")
	}
}

// ── processNextChunk ──────────────────────────────────────────────────────────

func TestTranscriberProcessNextChunk_SkipsWhenInsufficient(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	tr.audioBuffer = make([]float32, sampleRate/4-1)

	tr.processNextChunk(" ")
	tr.processingWg.Wait()

	if tr.processedSamples != 0 {
		t.Errorf("processedSamples = %d, want 0 when buffer is too small", tr.processedSamples)
	}
}

func TestTranscriberProcessNextChunk_AdvancesPointer(t *testing.T) {
	tr := buildTranscriber(t, modelWithSegments("chunk"))
	tr.audioBuffer = make([]float32, sampleRate*2)

	tr.processNextChunk(" ")
	tr.processingWg.Wait()

	if tr.processedSamples != sampleRate*2 {
		t.Errorf("processedSamples = %d, want %d", tr.processedSamples, sampleRate*2)
	}
}

// ── Transcript accumulation ───────────────────────────────────────────────────

func TestProcessNextChunk_AccumulatesTranscript(t *testing.T) {
	// With pause-based detection, processNextChunk processes all available audio at once.
	// This test verifies that transcribing a large buffer produces accumulated text.
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "hello world"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.audioBuffer = make([]float32, sampleRate*2)

	tr.processNextChunk(" ")
	tr.processingWg.Wait()

	tr.bufferMutex.Lock()
	got := tr.accumulatedTranscript
	tr.bufferMutex.Unlock()

	if got != "hello world" {
		t.Errorf("accumulatedTranscript = %q, want %q", got, "hello world")
	}
}

func TestStopProcessing_ReturnsCumulativeText(t *testing.T) {
	// Simulate a session where one chunk was already processed and its text
	// accumulated, then a tail remains to be transcribed on stop.
	tailCalled := false
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		tailCalled = true
		return &mockContext{segments: []gowhisper.Segment{{Text: "tail"}}}, nil
	}}
	tr := startProcessingTranscriber(t, m)

	// Pre-populate as if one chunk was already transcribed.
	tr.accumulatedTranscript = "first chunk"
	tr.audioBuffer = make([]float32, int(1.8*float64(sampleRate)))
	tr.processedSamples = sampleRate // 1 s processed, 0.8 s tail
	tr.hasSpeechInSegment = true

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tailCalled {
		t.Error("expected tail to be transcribed")
	}
	// Final text should be accumulated + tail joined by a space.
	want := "first chunk tail"
	if got != want {
		t.Errorf("lastFinalText = %q, want %q", got, want)
	}
}

func TestStopProcessing_AccumulatedOnlyNoTail(t *testing.T) {
	// Tail is too short to transcribe; result should be accumulated text only.
	tr := startProcessingTranscriber(t, modelWithSegments("should not appear"))
	tr.accumulatedTranscript = "only this"
	// Tail = 0.5 s exactly, which is not > 0.5 s so it is skipped.
	tr.audioBuffer = make([]float32, int(1.5*float64(sampleRate)))
	tr.processedSamples = sampleRate

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "only this" {
		t.Errorf("lastFinalText = %q, want %q", got, "only this")
	}
}

// ── Pause-based line breaking ───────────────────────────────────────────────────

func TestTranscribeAndAccumulate_JoinsWithSpace(t *testing.T) {
	m := modelWithSegments("hello world")
	tr := buildTranscriber(t, m)
	tr.accumulatedTranscript = "previous"
	tr.audioBuffer = make([]float32, sampleRate*2)

	tr.processNextChunk(" ")
	tr.processingWg.Wait()

	tr.bufferMutex.Lock()
	got := tr.accumulatedTranscript
	tr.bufferMutex.Unlock()

	want := "previous hello world"
	if got != want {
		t.Errorf("accumulatedTranscript = %q, want %q", got, want)
	}
}

func TestTranscribeAndAccumulate_JoinsWithNewline(t *testing.T) {
	m := modelWithSegments("second line")
	tr := buildTranscriber(t, m)
	tr.accumulatedTranscript = "first line"
	tr.audioBuffer = make([]float32, sampleRate*2)

	tr.processNextChunk("\n")
	tr.processingWg.Wait()

	tr.bufferMutex.Lock()
	got := tr.accumulatedTranscript
	tr.bufferMutex.Unlock()

	want := "first line\nsecond line"
	if got != want {
		t.Errorf("accumulatedTranscript = %q, want %q", got, want)
	}
}

func TestProcessNextChunk_ReturnsFalseWhenInsufficient(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	tr.audioBuffer = make([]float32, sampleRate/4-1)

	accepted := tr.processNextChunk(" ")

	if accepted {
		t.Error("processNextChunk should return false when insufficient samples")
	}
}

func TestProcessNextChunk_ReturnsTrueWhenAccepted(t *testing.T) {
	tr := buildTranscriber(t, modelWithSegments("text"))
	tr.audioBuffer = make([]float32, sampleRate*2)

	accepted := tr.processNextChunk(" ")

	if !accepted {
		t.Error("processNextChunk should return true when chunk is accepted")
	}
}

func TestStopProcessing_TailUsesNewlineAfterLongPause(t *testing.T) {
	tr := startProcessingTranscriber(t, modelWithSegments("tail"))
	tr.accumulatedTranscript = "first line"
	tr.audioBuffer = make([]float32, int(1.8*float64(sampleRate)))
	tr.processedSamples = sampleRate
	tr.hasSpeechInSegment = true
	tr.pauseTotal = 2 * time.Second
	tr.pauseCount = 2 // avg pause = 1s
	tr.lastSpeechAt = time.Now().Add(-1500 * time.Millisecond)

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "first line\ntail"
	if got != want {
		t.Errorf("lastFinalText = %q, want %q", got, want)
	}
}

func TestStopProcessing_TailUsesSpaceWhenPauseNotLongerThanAverage(t *testing.T) {
	tr := startProcessingTranscriber(t, modelWithSegments("tail"))
	tr.accumulatedTranscript = "first line"
	tr.audioBuffer = make([]float32, int(1.8*float64(sampleRate)))
	tr.processedSamples = sampleRate
	tr.hasSpeechInSegment = true
	tr.pauseTotal = 4 * time.Second
	tr.pauseCount = 2 // avg pause = 2s
	tr.lastSpeechAt = time.Now().Add(-1 * time.Second)

	got, err := stopAndWait(t, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "first line tail"
	if got != want {
		t.Errorf("lastFinalText = %q, want %q", got, want)
	}
}

func TestResolveSTTLanguage_DefaultsToEnglish(t *testing.T) {
	language, auto := resolveSTTLanguage(&domain.STTConfig{ModelName: "large-v3", Language: ""})
	if language != "en" {
		t.Fatalf("language = %q, want %q", language, "en")
	}
	if auto {
		t.Fatal("auto detect should be false")
	}
}

func TestResolveSTTLanguage_AllowsAutoForMultilingualModels(t *testing.T) {
	language, auto := resolveSTTLanguage(&domain.STTConfig{ModelName: "large-v3", Language: "auto"})
	if language != "auto" {
		t.Fatalf("language = %q, want %q", language, "auto")
	}
	if !auto {
		t.Fatal("auto detect should be true")
	}
}

func TestResolveSTTLanguage_ForcesEnglishForEnglishOnlyModels(t *testing.T) {
	language, auto := resolveSTTLanguage(&domain.STTConfig{ModelName: "small.en-q8_0", Language: "auto"})
	if language != "en" {
		t.Fatalf("language = %q, want %q", language, "en")
	}
	if auto {
		t.Fatal("auto detect should be false for .en model")
	}
}

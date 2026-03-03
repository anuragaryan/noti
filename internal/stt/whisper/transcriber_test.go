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

// buildTranscriber returns a Transcriber backed by m, bypassing the
// constructor to avoid needing a real model file.
func buildTranscriber(t *testing.T, m *mockModel) *Transcriber {
	t.Helper()
	cfg := &domain.STTConfig{ModelName: "tiny", ChunkDurationSecs: 1}
	tr := &Transcriber{
		config:          cfg,
		audioBuffer:     []float32{},
		samplesPerChunk: transcriptionSampleRate * cfg.ChunkDurationSecs,
	}
	if m != nil {
		tr.model = m
	}
	return tr
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
	if err := os.MkdirAll(filepath.Join(base, "models"), 0755); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(base, "models", "ggml-tiny.bin")
	if err := os.WriteFile(modelPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &domain.STTConfig{ModelName: "tiny", ChunkDurationSecs: 3}
	tr, err := NewTranscriber(base, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil Transcriber")
	}
	if tr.samplesPerChunk != transcriptionSampleRate*3 {
		t.Errorf("samplesPerChunk = %d, want %d", tr.samplesPerChunk, transcriptionSampleRate*3)
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
	tr := buildTranscriber(t, &mockModel{})
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})

	tr.ProcessChunk(domain.AudioChunk{Data: []float32{0.1, 0.2, 0.3}})

	if len(tr.audioBuffer) != 3 {
		t.Errorf("audioBuffer length = %d, want 3", len(tr.audioBuffer))
	}
}

func TestProcessChunk_AppendsConcurrently(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})

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
	tr.model = nil

	_, err := tr.transcribe([]float32{0.1})
	if err == nil {
		t.Fatal("expected error when model is nil")
	}
}

func TestTranscriberTranscribe_ConcatenatesSegments(t *testing.T) {
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{
			{Text: "foo "},
			{Text: "bar"},
	}}, nil }}
	tr := buildTranscriber(t, m)

	got, err := tr.transcribe(make([]float32, 100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "foo bar" {
		t.Errorf("transcribe() = %q, want %q", got, "foo bar")
	}
}

func TestTranscribeThreadSafe_SerializesConcurrentAccess(t *testing.T) {
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "ok"}}}, nil
	}}
	tr := buildTranscriber(t, m)

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
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("Text = %q, want empty", result.Text)
	}
}

func TestStopProcessing_ProcessesTailBuffer(t *testing.T) {
	// 1.8 s in buffer, 1 s already processed → 0.8 s tail (> 0.5 s threshold).
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "tail"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})
	tr.audioBuffer = make([]float32, int(1.8*float64(transcriptionSampleRate)))
	tr.processedSamples = transcriptionSampleRate

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "tail" {
		t.Errorf("Text = %q, want %q", result.Text, "tail")
	}
}

func TestStopProcessing_FallbackFullBuffer(t *testing.T) {
	// Nothing processed yet and buffer > 1 s → full-buffer fallback.
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "full"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})
	tr.audioBuffer = make([]float32, transcriptionSampleRate*2)
	tr.processedSamples = 0

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "full" {
		t.Errorf("Text = %q, want %q", result.Text, "full")
	}
}

func TestStopProcessing_ShortTailSkipped(t *testing.T) {
	// Tail is exactly 0.5 s — not strictly greater — so it is skipped.
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "should not appear"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})
	// 1 s processed; total = 1.5 s → tail = 0.5 s (not > 0.5 s)
	tr.audioBuffer = make([]float32, int(1.5*float64(transcriptionSampleRate)))
	tr.processedSamples = transcriptionSampleRate

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// processedSamples != 0 so the full-buffer fallback is also skipped.
	if result.Text != "" {
		t.Errorf("Text = %q, want empty (tail too short)", result.Text)
	}
}

// ── StartProcessing ───────────────────────────────────────────────────────────

func TestStartProcessing_SetsIsProcessing(t *testing.T) {
	tr := buildTranscriber(t, &mockModel{})

	if err := tr.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing() error: %v", err)
	}
	t.Cleanup(func() {
		tr.bufferMutex.Lock()
		if tr.isProcessing {
			close(tr.stopProcessing)
			tr.isProcessing = false
		}
		tr.bufferMutex.Unlock()
		tr.processingWg.Wait()
	})

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
	t.Cleanup(func() {
		tr.bufferMutex.Lock()
		if tr.isProcessing {
			close(tr.stopProcessing)
			tr.isProcessing = false
		}
		tr.bufferMutex.Unlock()
		tr.processingWg.Wait()
	})

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
	tr.model = nil
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
	tr.audioBuffer = make([]float32, transcriptionSampleRate/2)
	tr.samplesPerChunk = transcriptionSampleRate

	tr.processNextChunk()
	tr.processingWg.Wait()

	if tr.processedSamples != 0 {
		t.Errorf("processedSamples = %d, want 0 when buffer is too small", tr.processedSamples)
	}
}

func TestTranscriberProcessNextChunk_AdvancesPointer(t *testing.T) {
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "chunk"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.audioBuffer = make([]float32, transcriptionSampleRate*2)
	tr.samplesPerChunk = transcriptionSampleRate

	tr.processNextChunk()
	tr.processingWg.Wait()

	if tr.processedSamples != transcriptionSampleRate {
		t.Errorf("processedSamples = %d, want %d", tr.processedSamples, transcriptionSampleRate)
	}
}

// ── Transcript accumulation ───────────────────────────────────────────────────

func TestProcessNextChunk_AccumulatesTranscript(t *testing.T) {
	// First chunk returns "hello", second returns "world".
	// accumulatedTranscript should be "hello world" after two calls.
	callCount := 0
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		callCount++
		text := "hello"
		if callCount > 1 {
			text = "world"
		}
		return &mockContext{segments: []gowhisper.Segment{{Text: text}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.audioBuffer = make([]float32, transcriptionSampleRate*3)
	tr.samplesPerChunk = transcriptionSampleRate

	tr.processNextChunk() // chunk 1 → "hello"
	tr.processingWg.Wait()
	tr.processNextChunk() // chunk 2 → "world"
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
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})

	// Pre-populate as if one chunk was already transcribed.
	tr.accumulatedTranscript = "first chunk"
	tr.audioBuffer = make([]float32, int(1.8*float64(transcriptionSampleRate)))
	tr.processedSamples = transcriptionSampleRate // 1 s processed, 0.8 s tail

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tailCalled {
		t.Error("expected tail to be transcribed")
	}
	// Final text should be accumulated + tail joined by a space.
	want := "first chunk tail"
	if result.Text != want {
		t.Errorf("Text = %q, want %q", result.Text, want)
	}
}

func TestStopProcessing_AccumulatedOnlyNoTail(t *testing.T) {
	// Tail is too short to transcribe; result should be accumulated text only.
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "should not appear"}}}, nil
	}}
	tr := buildTranscriber(t, m)
	tr.isProcessing = true
	tr.stopProcessing = make(chan struct{})
	tr.accumulatedTranscript = "only this"
	// Tail = 0.5 s exactly, which is not > 0.5 s so it is skipped.
	tr.audioBuffer = make([]float32, int(1.5*float64(transcriptionSampleRate)))
	tr.processedSamples = transcriptionSampleRate

	result, err := tr.StopProcessing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "only this" {
		t.Errorf("Text = %q, want %q", result.Text, "only this")
	}
}

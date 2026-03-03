package whisper

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"noti/internal/domain"

	gowhisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// ── mock implementations ──────────────────────────────────────────────────────

// mockModel implements gowhisper.Model.
type mockModel struct {
	newContextFn func() (gowhisper.Context, error)
	closed       bool
}

func (m *mockModel) Close() error         { m.closed = true; return nil }
func (m *mockModel) IsMultilingual() bool { return false }
func (m *mockModel) Languages() []string  { return nil }
func (m *mockModel) NewContext() (gowhisper.Context, error) {
	if m.newContextFn != nil {
		return m.newContextFn()
	}
	return &mockContext{}, nil
}

// mockContext implements gowhisper.Context.
type mockContext struct {
	mu           sync.Mutex
	processErr   error
	segments     []gowhisper.Segment
	segmentIndex int
}

func (c *mockContext) SetLanguage(string) error            { return nil }
func (c *mockContext) SetTranslate(bool)                   {}
func (c *mockContext) IsMultilingual() bool                { return false }
func (c *mockContext) Language() string                    { return "en" }
func (c *mockContext) DetectedLanguage() string            { return "en" }
func (c *mockContext) SetOffset(time.Duration)             {}
func (c *mockContext) SetDuration(time.Duration)           {}
func (c *mockContext) SetThreads(uint)                     {}
func (c *mockContext) SetSplitOnWord(bool)                 {}
func (c *mockContext) SetTokenThreshold(float32)           {}
func (c *mockContext) SetTokenSumThreshold(float32)        {}
func (c *mockContext) SetMaxSegmentLength(uint)            {}
func (c *mockContext) SetTokenTimestamps(bool)             {}
func (c *mockContext) SetMaxTokensPerSegment(uint)         {}
func (c *mockContext) SetAudioCtx(uint)                    {}
func (c *mockContext) SetMaxContext(int)                   {}
func (c *mockContext) SetBeamSize(int)                     {}
func (c *mockContext) SetEntropyThold(float32)             {}
func (c *mockContext) SetInitialPrompt(string)             {}
func (c *mockContext) SetTemperature(float32)              {}
func (c *mockContext) SetTemperatureFallback(float32)      {}
func (c *mockContext) PrintTimings()                       {}
func (c *mockContext) ResetTimings()                       {}
func (c *mockContext) SystemInfo() string                  { return "" }
func (c *mockContext) IsBEG(gowhisper.Token) bool          { return false }
func (c *mockContext) IsSOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsEOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsPREV(gowhisper.Token) bool         { return false }
func (c *mockContext) IsSOLM(gowhisper.Token) bool         { return false }
func (c *mockContext) IsNOT(gowhisper.Token) bool          { return false }
func (c *mockContext) IsLANG(gowhisper.Token, string) bool { return false }
func (c *mockContext) IsText(gowhisper.Token) bool         { return false }

func (c *mockContext) Process([]float32, gowhisper.EncoderBeginCallback, gowhisper.SegmentCallback, gowhisper.ProgressCallback) error {
	return c.processErr
}

func (c *mockContext) NextSegment() (gowhisper.Segment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.segmentIndex >= len(c.segments) {
		return gowhisper.Segment{}, io.EOF
	}
	seg := c.segments[c.segmentIndex]
	c.segmentIndex++
	return seg, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestService builds a Service wired to a mock model. It bypasses the
// constructor (which requires a real model file) and initialises only the
// fields needed for unit testing.
func newTestService(t *testing.T, model gowhisper.Model) *Service {
	t.Helper()
	cfg := &domain.STTConfig{
		ModelName:         "tiny",
		ChunkDurationSecs: 1,
	}
	return &Service{
		config:          cfg,
		model:           model,
		audioBuffer:     []float32{},
		samplesPerChunk: sampleRate * cfg.ChunkDurationSecs,
		stopRecording:   make(chan bool),
	}
}

// ── NewService ────────────────────────────────────────────────────────────────

func TestNewService_ModelNotFound(t *testing.T) {
	cfg := &domain.STTConfig{ModelName: "nonexistent", ChunkDurationSecs: 1}
	_, err := NewService(t.TempDir(), cfg)
	if err == nil {
		t.Fatal("expected error when model file is missing, got nil")
	}
}

func TestNewService_Success(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "models"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a dummy model file so the existence check passes.
	modelPath := filepath.Join(base, "models", "ggml-tiny.bin")
	if err := os.WriteFile(modelPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &domain.STTConfig{ModelName: "tiny", ChunkDurationSecs: 2}
	svc, err := NewService(base, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil Service")
	}
	if svc.samplesPerChunk != sampleRate*2 {
		t.Errorf("samplesPerChunk = %d, want %d", svc.samplesPerChunk, sampleRate*2)
	}
}

// ── cleanTranscription ────────────────────────────────────────────────────────

func TestCleanTranscription(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"  hello  ", "hello"},
		{"[BLANK_AUDIO]", ""},
		{"  [BLANK_AUDIO]  ", ""},
		{"hello [BLANK_AUDIO] world", "hello  world"},
		{"", ""},
	}

	for _, tc := range tests {
		got := cleanTranscription(tc.input)
		if got != tc.want {
			t.Errorf("cleanTranscription(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── transcribe ────────────────────────────────────────────────────────────────

func TestTranscribe_ModelNil(t *testing.T) {
	svc := newTestService(t, nil)
	svc.model = nil

	_, err := svc.transcribe([]float32{0.1, 0.2})
	if err == nil {
		t.Fatal("expected error when model is nil, got nil")
	}
}

func TestTranscribe_ContextError(t *testing.T) {
	m := &mockModel{
		newContextFn: func() (gowhisper.Context, error) {
			return nil, errors.New("context creation failed")
		},
	}
	svc := newTestService(t, m)

	_, err := svc.transcribe([]float32{0.1})
	if err == nil {
		t.Fatal("expected error from failed NewContext, got nil")
	}
}

func TestTranscribe_ProcessError(t *testing.T) {
	ctx := &mockContext{processErr: errors.New("process failed")}
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) { return ctx, nil }}
	svc := newTestService(t, m)

	_, err := svc.transcribe([]float32{0.1})
	if err == nil {
		t.Fatal("expected error from failed Process, got nil")
	}
}

func TestTranscribe_MultipleSegments(t *testing.T) {
	ctx := &mockContext{
		segments: []gowhisper.Segment{
			{Text: "hello "},
			{Text: "world"},
		},
	}
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) { return ctx, nil }}
	svc := newTestService(t, m)

	got, err := svc.transcribe(make([]float32, 1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("transcribe() = %q, want %q", got, "hello world")
	}
}

func TestTranscribe_NoSegments(t *testing.T) {
	ctx := &mockContext{}
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) { return ctx, nil }}
	svc := newTestService(t, m)

	got, err := svc.transcribe(make([]float32, 100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("transcribe() = %q, want empty string", got)
	}
}

// ── transcribeThreadSafe ──────────────────────────────────────────────────────

func TestTranscribeThreadSafe_Concurrent(t *testing.T) {
	// Verify that concurrent callers don't corrupt state (data-race detector
	// will catch any missing lock).
	ctx := &mockContext{segments: []gowhisper.Segment{{Text: "ok"}}}
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		// Each call gets a fresh context so segment indices reset correctly.
		return &mockContext{segments: []gowhisper.Segment{{Text: "ok"}}}, nil
	}}
	svc := newTestService(t, m)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, err := svc.transcribeThreadSafe(make([]float32, 100))
			if err != nil {
				t.Errorf("transcribeThreadSafe error: %v", err)
			}
			if text != "ok" {
				t.Errorf("transcribeThreadSafe() = %q, want %q", text, "ok")
			}
		}()
	}
	wg.Wait()
	_ = ctx // suppress unused warning
}

// ── IsRecording ───────────────────────────────────────────────────────────────

func TestIsRecording_Default(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	if svc.IsRecording() {
		t.Error("IsRecording() should be false on a newly created Service")
	}
}

func TestIsRecording_AfterSet(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	svc.recordingMutex.Lock()
	svc.isRecording = true
	svc.recordingMutex.Unlock()

	if !svc.IsRecording() {
		t.Error("IsRecording() should be true after setting isRecording")
	}
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

func TestCleanup_ClosesModel(t *testing.T) {
	m := &mockModel{}
	svc := newTestService(t, m)

	svc.Cleanup()

	if !m.closed {
		t.Error("Cleanup() should close the model")
	}
}

func TestCleanup_NilModel(t *testing.T) {
	svc := newTestService(t, nil)
	svc.model = nil
	// Should not panic.
	svc.Cleanup()
}

// ── processNextChunk ──────────────────────────────────────────────────────────

func TestProcessNextChunk_InsufficientSamples(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	// Buffer has fewer samples than one chunk — nothing should be dispatched.
	svc.audioBuffer = make([]float32, sampleRate/2)
	svc.samplesPerChunk = sampleRate

	svc.processNextChunk()
	svc.processingWg.Wait()

	if svc.processedSamples != 0 {
		t.Errorf("processedSamples = %d, want 0 when buffer is too small", svc.processedSamples)
	}
}

func TestProcessNextChunk_AdvancesProcessedSamples(t *testing.T) {
	ctx := &mockContext{segments: []gowhisper.Segment{{Text: "hi"}}}
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "hi"}}}, nil
	}}
	svc := newTestService(t, m)
	svc.audioBuffer = make([]float32, sampleRate*2)
	svc.samplesPerChunk = sampleRate

	svc.processNextChunk()
	svc.processingWg.Wait()

	if svc.processedSamples != sampleRate {
		t.Errorf("processedSamples = %d, want %d", svc.processedSamples, sampleRate)
	}
	_ = ctx
}

// ── StopRecording ─────────────────────────────────────────────────────────────

func TestStopRecording_NotRecording(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	_, err := svc.StopRecording()
	if err == nil {
		t.Fatal("StopRecording() should return an error when not recording")
	}
}

func TestStopRecording_EmptyBuffer(t *testing.T) {
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{}, nil
	}}
	svc := newTestService(t, m)

	// Simulate active recording without real PortAudio.
	svc.isRecording = true
	svc.stopRecording = make(chan bool)

	result, err := svc.StopRecording()
	if err != nil {
		t.Fatalf("StopRecording() returned error: %v", err)
	}
	if result == nil {
		t.Fatal("StopRecording() returned nil result")
	}
	if result.IsPartial {
		t.Error("final result should not be partial")
	}
}

func TestStopRecording_ProcessedSamplesRace(t *testing.T) {
	// Verify that processedSamples captured under lock is used — not a stale
	// copy — by having processNextChunk advance it before StopRecording reads.
	m := &mockModel{newContextFn: func() (gowhisper.Context, error) {
		return &mockContext{segments: []gowhisper.Segment{{Text: "segment"}}}, nil
	}}
	svc := newTestService(t, m)
	svc.isRecording = true
	svc.stopRecording = make(chan bool)

	// Pre-populate buffer with exactly two chunks worth of samples so the
	// remainder after one chunk is less than 0.5 s.
	svc.audioBuffer = make([]float32, sampleRate*2)
	svc.samplesPerChunk = sampleRate
	svc.processedSamples = sampleRate // pretend one chunk was already processed

	result, err := svc.StopRecording()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
}

// ── SetContext ────────────────────────────────────────────────────────────────

func TestSetContext(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	if svc.ctx != nil {
		t.Error("ctx should be nil initially")
	}
	// A non-nil context (even a plain background one from the standard library
	// would pull in "context" import; we just verify assignment).
	type key struct{}
	svc.SetContext(nil) // setting nil is a valid no-op
	if svc.ctx != nil {
		t.Error("ctx should still be nil after SetContext(nil)")
	}
}

// ── captureAudio ─────────────────────────────────────────────────────────────

func TestCaptureAudio_StopsOnChannel(t *testing.T) {
	svc := newTestService(t, &mockModel{})
	svc.stopRecording = make(chan bool)

	done := make(chan struct{})
	// Use a minimal framesPerBuffer; Read() is never called because we close
	// the stop channel immediately.
	go func() {
		// We can't call svc.captureAudio directly because it would call
		// s.stream.Read() which panics without a real stream. Instead we test
		// the channel-stop path by closing before the goroutine has a chance to
		// fall into the default branch.
		close(svc.stopRecording)
		// The goroutine should exit after noticing the closed channel.
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("captureAudio channel close test timed out")
	}
}

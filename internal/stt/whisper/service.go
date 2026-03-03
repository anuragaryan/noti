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
	"github.com/gordonklaus/portaudio"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	sampleRate = 16000
)

// Service handles speech-to-text operations using Whisper backed by PortAudio
// for microphone capture. It is the legacy recording path; new code should
// prefer Transcriber + AudioManager.
type Service struct {
	model          whisper.Model
	modelMutex     sync.Mutex
	recordingMutex sync.Mutex
	isRecording    bool
	audioBuffer    []float32
	// processedSamples is only mutated under recordingMutex.
	processedSamples int
	stream           *portaudio.Stream
	stopRecording    chan bool
	processingWg     sync.WaitGroup
	config           *domain.STTConfig
	modelPath        string
	recordingsPath   string
	ctx              context.Context
	samplesPerChunk  int
}

// NewService creates a new Whisper STT service. It verifies that the model
// file exists and creates the recordings directory if needed.
func NewService(basePath string, config *domain.STTConfig) (*Service, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
	modelPath := filepath.Join(basePath, "models", modelFileName)
	recordingsPath := filepath.Join(basePath, "recordings")

	if err := os.MkdirAll(recordingsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recordings directory: %w", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	return &Service{
		config:          config,
		modelPath:       modelPath,
		recordingsPath:  recordingsPath,
		stopRecording:   make(chan bool),
		audioBuffer:     []float32{},
		samplesPerChunk: sampleRate * config.ChunkDurationSecs,
	}, nil
}

// SetContext sets the Wails runtime context used for emitting frontend events.
func (s *Service) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// Initialize loads the Whisper model and validates that a default audio input
// device is available via PortAudio.
func (s *Service) Initialize() error {
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize portaudio: %w", err)
	}

	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get audio devices: %w", err)
	}

	hasInput := false
	for _, device := range devices {
		if device.MaxInputChannels > 0 {
			hasInput = true
			break
		}
	}
	if !hasInput {
		return fmt.Errorf("no input devices found – check microphone connection and permissions")
	}

	if _, err := portaudio.DefaultInputDevice(); err != nil {
		return fmt.Errorf("no default input device: %w – set a default microphone in system settings", err)
	}

	model, err := whisper.New(s.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load whisper model: %w", err)
	}

	s.model = model
	return nil
}

// Cleanup waits for in-flight transcription goroutines to finish, closes the
// Whisper model, and terminates PortAudio.
func (s *Service) Cleanup() {
	s.processingWg.Wait()

	if s.model != nil {
		s.model.Close()
	}
	portaudio.Terminate()
}

// StartRecording begins capturing audio from the default microphone. Two
// goroutines are launched: one to read raw PCM frames and one to transcribe
// accumulated chunks on a regular interval.
func (s *Service) StartRecording() error {
	s.recordingMutex.Lock()
	defer s.recordingMutex.Unlock()

	if s.isRecording {
		return fmt.Errorf("already recording")
	}

	s.audioBuffer = []float32{}
	s.processedSamples = 0

	inputChannels := 1
	framesPerBuffer := make([]float32, 1024)

	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("no default input device: %w", err)
	}

	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   defaultInput,
			Channels: inputChannels,
			Latency:  defaultInput.DefaultLowInputLatency,
		},
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: len(framesPerBuffer),
	}

	stream, err := portaudio.OpenStream(streamParams, framesPerBuffer)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %w", err)
	}

	if err := stream.Start(); err != nil {
		stream.Close()
		return fmt.Errorf("failed to start audio stream: %w", err)
	}

	s.stream = stream
	s.isRecording = true
	s.stopRecording = make(chan bool)

	go s.captureAudio(framesPerBuffer)
	go s.processAudioRealtime()

	return nil
}

// captureAudio reads PCM frames from the PortAudio stream and appends them to
// the shared audio buffer. It exits when stopRecording is closed.
func (s *Service) captureAudio(framesPerBuffer []float32) {
	for {
		select {
		case <-s.stopRecording:
			return
		default:
			if err := s.stream.Read(); err != nil {
				fmt.Printf("audio read error: %v\n", err)
				return
			}

			s.recordingMutex.Lock()
			s.audioBuffer = append(s.audioBuffer, framesPerBuffer...)
			s.recordingMutex.Unlock()
		}
	}
}

// processAudioRealtime fires a transcription pass every ChunkDurationSecs
// seconds while recording is active.
func (s *Service) processAudioRealtime() {
	ticker := time.NewTicker(time.Duration(s.config.ChunkDurationSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopRecording:
			return
		case <-ticker.C:
			s.processNextChunk()
		}
	}
}

// processNextChunk extracts the next unseen chunk from the audio buffer and
// transcribes it in a background goroutine. It is a no-op when fewer than
// samplesPerChunk new samples are available.
func (s *Service) processNextChunk() {
	s.recordingMutex.Lock()

	totalSamples := len(s.audioBuffer)
	if totalSamples < s.processedSamples+s.samplesPerChunk {
		s.recordingMutex.Unlock()
		return
	}

	chunkStart := s.processedSamples
	chunkEnd := chunkStart + s.samplesPerChunk
	if chunkEnd > totalSamples {
		chunkEnd = totalSamples
	}

	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, s.audioBuffer[chunkStart:chunkEnd])
	s.processedSamples = chunkEnd

	s.recordingMutex.Unlock()

	s.processingWg.Add(1)
	go func(audioChunk []float32, start, end int) {
		defer s.processingWg.Done()

		text, err := s.transcribeThreadSafe(audioChunk)
		if err != nil {
			fmt.Printf("[stt] chunk transcription error: %v\n", err)
			return
		}

		text = cleanTranscription(text)
		if text == "" {
			return
		}

		if s.ctx != nil {
			result := domain.TranscriptionResult{
				Text:      text,
				Language:  "en",
				IsPartial: true,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			runtime.EventsEmit(s.ctx, "transcription:partial", result)
		}
	}(chunk, chunkStart, chunkEnd)
}

// StopRecording stops audio capture, waits for in-flight transcription
// goroutines to finish, and processes any audio that was not yet transcribed.
// It returns an empty (non-nil) result when there is nothing left to transcribe.
func (s *Service) StopRecording() (*domain.TranscriptionResult, error) {
	s.recordingMutex.Lock()

	if !s.isRecording {
		s.recordingMutex.Unlock()
		return nil, fmt.Errorf("not currently recording")
	}

	close(s.stopRecording)
	time.Sleep(100 * time.Millisecond)

	if s.stream != nil {
		s.stream.Stop()
		s.stream.Close()
	}

	s.isRecording = false

	fullBuffer := make([]float32, len(s.audioBuffer))
	copy(fullBuffer, s.audioBuffer)
	// Capture processedSamples under the lock before releasing it.
	processedAtStop := s.processedSamples

	s.recordingMutex.Unlock()

	// Wait for in-flight chunk goroutines before processing the tail.
	s.processingWg.Wait()

	totalSamples := len(fullBuffer)
	remainingSamples := totalSamples - processedAtStop

	var finalText string

	if remainingSamples > sampleRate/2 {
		finalChunk := make([]float32, remainingSamples)
		copy(finalChunk, fullBuffer[processedAtStop:])

		text, err := s.transcribeThreadSafe(finalChunk)
		if err != nil {
			fmt.Printf("[stt] final chunk transcription error: %v\n", err)
		} else {
			finalText = cleanTranscription(text)
		}
	}

	// If nothing was transcribed at all, attempt the full buffer as a fallback.
	if processedAtStop == 0 && totalSamples > sampleRate {
		text, err := s.transcribeThreadSafe(fullBuffer)
		if err != nil {
			fmt.Printf("[stt] full-buffer transcription error: %v\n", err)
		} else {
			finalText = cleanTranscription(text)
		}
	}

	return &domain.TranscriptionResult{
		Text:      finalText,
		Language:  "en",
		IsPartial: false,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// IsRecording returns whether audio capture is currently active.
func (s *Service) IsRecording() bool {
	s.recordingMutex.Lock()
	defer s.recordingMutex.Unlock()
	return s.isRecording
}

// cleanTranscription removes known Whisper output artifacts (e.g. silence
// tokens) and trims surrounding whitespace.
func cleanTranscription(text string) string {
	text = strings.ReplaceAll(text, "[BLANK_AUDIO]", "")
	return strings.TrimSpace(text)
}

// transcribeThreadSafe serialises access to the Whisper model which is not
// safe for concurrent use.
func (s *Service) transcribeThreadSafe(audioData []float32) (string, error) {
	s.modelMutex.Lock()
	defer s.modelMutex.Unlock()
	return s.transcribe(audioData)
}

// transcribe runs inference on audioData. Callers must hold modelMutex.
func (s *Service) transcribe(audioData []float32) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	ctx, err := s.model.NewContext()
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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/gordonklaus/portaudio"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	sampleRate = 16000
)

// STTService handles speech-to-text operations
type STTService struct {
	model                whisper.Model
	modelMutex           sync.Mutex // CRITICAL: Protect model access
	isRecording          bool
	recordingMutex       sync.Mutex
	audioBuffer          []float32
	processedSamples     int
	stream               *portaudio.Stream
	stopRecording        chan bool
	processingWg         sync.WaitGroup // Wait for all processing to complete
	modelPath            string
	recordingsPath       string
	ctx                  context.Context // Wails runtime context for events
	chunkDurationSeconds int
	samplesPerChunk      int
}

// TranscriptionResult represents a transcription with metadata
type TranscriptionResult struct {
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Duration  float64 `json:"duration"`
	Timestamp string  `json:"timestamp"`
	IsPartial bool    `json:"isPartial"` // True for real-time chunks
}

// NewSTTService creates a new STT service
func NewSTTService(notesPath string, chunkDuration int, modelName string) (*STTService, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", modelName)
	modelPath := filepath.Join(notesPath, "models", modelFileName)
	recordingsPath := filepath.Join(notesPath, "recordings")

	// Create recordings directory
	if err := os.MkdirAll(recordingsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recordings directory: %w", err)
	}

	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	return &STTService{
		modelPath:            modelPath,
		recordingsPath:       recordingsPath,
		stopRecording:        make(chan bool),
		audioBuffer:          []float32{},
		chunkDurationSeconds: chunkDuration,
		samplesPerChunk:      sampleRate * chunkDuration,
	}, nil
}

// SetContext sets the Wails runtime context for emitting events
func (s *STTService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// Initialize loads the Whisper model
func (s *STTService) Initialize() error {
	// Initialize PortAudio
	fmt.Println("=== Initializing Audio System ===")
	fmt.Println("Initializing PortAudio...")
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize portaudio: %w", err)
	}

	// Check for input devices
	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get audio devices: %w", err)
	}

	fmt.Printf("Found %d audio devices:\n", len(devices))
	hasInput := false
	for i, device := range devices {
		if device.MaxInputChannels > 0 {
			fmt.Printf("  [%d] Input Device: %s\n", i, device.Name)
			fmt.Printf("      Channels: %d, Sample Rate: %.0f Hz\n",
				device.MaxInputChannels, device.DefaultSampleRate)
			hasInput = true
		}
	}

	if !hasInput {
		return fmt.Errorf("no input devices found - please check microphone connection and permissions")
	}

	// Check default input device
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("no default input device: %w - please set a default microphone in system settings", err)
	}
	fmt.Printf("\nDefault Input Device: %s\n", defaultInput.Name)

	// Load Whisper model
	fmt.Printf("\n=== Loading Whisper Model ===\n")
	fmt.Printf("Model path: %s\n", s.modelPath)

	model, err := whisper.New(s.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load whisper model: %w", err)
	}

	s.model = model
	fmt.Println("✓ Whisper model loaded successfully!")
	fmt.Println("✓ Real-time transcription enabled")
	fmt.Printf("✓ Processing chunks every %d seconds\n\n", s.chunkDurationSeconds)

	return nil
}

// Cleanup releases resources
func (s *STTService) Cleanup() {
	fmt.Println("Cleaning up STT service...")

	// Wait for all processing to complete
	s.processingWg.Wait()

	if s.model != nil {
		s.model.Close()
	}
	portaudio.Terminate()
}

// StartRecording begins capturing audio from the microphone with real-time transcription
func (s *STTService) StartRecording() error {
	s.recordingMutex.Lock()
	defer s.recordingMutex.Unlock()

	if s.isRecording {
		return fmt.Errorf("already recording")
	}

	fmt.Println("\n=== Starting Real-time Audio Recording ===")

	// Clear previous buffer
	s.audioBuffer = []float32{}
	s.processedSamples = 0

	// Define audio parameters
	inputChannels := 1
	framesPerBuffer := make([]float32, 1024)

	// Get default input device
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("no default input device found: %w\n\nPlease check:\n1. Microphone is connected\n2. System Settings → Privacy → Microphone permissions\n3. Default input device is set", err)
	}

	fmt.Printf("Using microphone: %s\n", defaultInput.Name)
	fmt.Printf("Sample rate: %d Hz, Channels: %d\n", sampleRate, inputChannels)
	fmt.Printf("Real-time mode: Processing every %d seconds\n", s.chunkDurationSeconds)

	// Create stream parameters with explicit device
	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   defaultInput,
			Channels: inputChannels,
			Latency:  defaultInput.DefaultLowInputLatency,
		},
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: len(framesPerBuffer),
	}

	// Open audio stream with parameters
	fmt.Println("Opening audio stream...")
	stream, err := portaudio.OpenStream(streamParams, framesPerBuffer)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %w\n\nPossible causes:\n1. Another app is using the microphone\n2. Microphone permissions not granted\n3. PortAudio not installed correctly", err)
	}

	s.stream = stream

	// Start the stream
	fmt.Println("Starting audio capture...")
	if err := stream.Start(); err != nil {
		stream.Close()
		return fmt.Errorf("failed to start audio stream: %w\n\nTry:\n1. Closing other apps using the microphone\n2. Restarting the application\n3. Checking system audio settings", err)
	}

	s.isRecording = true
	s.stopRecording = make(chan bool)

	fmt.Println("✓ Real-time recording started successfully!")
	fmt.Println("Speak into your microphone... (transcription will appear in real-time)")

	// Start audio capture goroutine
	go s.captureAudio(framesPerBuffer)

	// Start real-time processing goroutine
	go s.processAudioRealtime()

	return nil
}

// captureAudio continuously captures audio from the microphone
func (s *STTService) captureAudio(framesPerBuffer []float32) {
	for {
		select {
		case <-s.stopRecording:
			return
		default:
			// Read from stream
			if err := s.stream.Read(); err != nil {
				fmt.Printf("Error reading from stream: %v\n", err)
				return
			}

			// Append to buffer
			s.recordingMutex.Lock()
			s.audioBuffer = append(s.audioBuffer, framesPerBuffer...)
			s.recordingMutex.Unlock()
		}
	}
}

// processAudioRealtime processes audio chunks in real-time
func (s *STTService) processAudioRealtime() {
	ticker := time.NewTicker(time.Duration(s.chunkDurationSeconds) * time.Second)
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

// processNextChunk transcribes the next audio chunk
func (s *STTService) processNextChunk() {
	s.recordingMutex.Lock()

	// Check if we have enough samples for a chunk
	totalSamples := len(s.audioBuffer)
	if totalSamples < s.processedSamples+s.samplesPerChunk {
		s.recordingMutex.Unlock()
		fmt.Println("[Real-time] Not enough samples yet for next chunk")
		return
	}

	// Extract chunk to process
	chunkStart := s.processedSamples
	chunkEnd := chunkStart + s.samplesPerChunk
	if chunkEnd > totalSamples {
		chunkEnd = totalSamples
	}

	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, s.audioBuffer[chunkStart:chunkEnd])

	s.processedSamples = chunkEnd

	s.recordingMutex.Unlock()

	// Increment wait group for this processing task
	s.processingWg.Add(1)

	// Process this chunk in background
	go func(audioChunk []float32, start, end int) {
		defer s.processingWg.Done()

		fmt.Printf("\n[Real-time] Processing chunk: %.1f-%.1f seconds (samples: %d-%d)\n",
			float64(start)/float64(sampleRate),
			float64(end)/float64(sampleRate),
			start, end)

		// CRITICAL: Use mutex when accessing the model
		text, err := s.transcribeThreadSafe(audioChunk)
		if err != nil {
			fmt.Printf("[Real-time] Transcription error: %v\n", err)
			return
		}

		// Clean and check if we have meaningful text
		text = cleanTranscription(text)

		if text != "" {
			fmt.Printf("[Real-time] Transcribed: '%s'\n", text)

			// Emit real-time transcription event to frontend
			if s.ctx != nil {
				result := TranscriptionResult{
					Text:      text,
					Language:  "en",
					IsPartial: true,
					Timestamp: time.Now().Format(time.RFC3339),
				}

				fmt.Printf("[Real-time] Emitting event: transcription:partial with text: '%s'\n", result.Text)
				runtime.EventsEmit(s.ctx, "transcription:partial", result)
				fmt.Println("[Real-time] Event emitted successfully")
			} else {
				fmt.Println("[Real-time] WARNING: Context is nil, cannot emit event!")
			}
		} else {
			fmt.Println("[Real-time] No text transcribed (silence or noise)")
		}
	}(chunk, chunkStart, chunkEnd)
}

// StopRecording stops audio capture and processes remaining audio
func (s *STTService) StopRecording() (*TranscriptionResult, error) {
	s.recordingMutex.Lock()

	if !s.isRecording {
		s.recordingMutex.Unlock()
		return nil, fmt.Errorf("not currently recording")
	}

	fmt.Println("\n=== Stopping Recording ===")

	// Signal to stop recording
	close(s.stopRecording)

	// Give a moment for audio capture to stop
	time.Sleep(100 * time.Millisecond)

	// Stop and close the stream
	if s.stream != nil {
		s.stream.Stop()
		s.stream.Close()
	}

	s.isRecording = false

	// Copy the full buffer for safety
	fullBuffer := make([]float32, len(s.audioBuffer))
	copy(fullBuffer, s.audioBuffer)

	// Process any remaining audio
	totalSamples := len(fullBuffer)
	remainingSamples := totalSamples - s.processedSamples

	fmt.Printf("Total recorded: %.2f seconds (%d samples)\n", float64(totalSamples)/float64(sampleRate), totalSamples)
	fmt.Printf("Already processed: %.2f seconds (%d samples)\n", float64(s.processedSamples)/float64(sampleRate), s.processedSamples)
	fmt.Printf("Remaining: %.2f seconds (%d samples)\n", float64(remainingSamples)/float64(sampleRate), remainingSamples)

	s.recordingMutex.Unlock()

	// Wait for any ongoing processing to complete before final chunk
	fmt.Println("Waiting for ongoing processing to complete...")
	s.processingWg.Wait()
	fmt.Println("All ongoing processing complete")

	var finalText string

	// Process remaining audio even if less than 1 second
	if remainingSamples > sampleRate/2 { // At least 0.5 seconds remaining
		fmt.Printf("Processing final chunk of %d samples...\n", remainingSamples)
		finalChunk := make([]float32, remainingSamples)
		copy(finalChunk, fullBuffer[s.processedSamples:])

		text, err := s.transcribeThreadSafe(finalChunk)
		if err != nil {
			fmt.Printf("Final transcription error: %v\n", err)
		} else {
			text = cleanTranscription(text)
			if text != "" {
				finalText = text
				fmt.Printf("Final chunk transcribed: '%s'\n", text)
			} else {
				fmt.Println("Final chunk: no text (silence)")
			}
		}
	} else {
		fmt.Println("No significant audio remaining, skipping final chunk")
	}

	// If nothing was ever transcribed, process the whole recording
	if s.processedSamples == 0 && totalSamples > sampleRate {
		fmt.Println("WARNING: No chunks were processed during recording!")
		fmt.Println("Processing entire recording as single chunk...")

		text, err := s.transcribeThreadSafe(fullBuffer)
		if err != nil {
			fmt.Printf("Full transcription error: %v\n", err)
		} else {
			text = cleanTranscription(text)
			if text != "" {
				finalText = text
				fmt.Printf("Full recording transcribed: '%s'\n", text)
			}
		}
	}

	fmt.Println("✓ Recording stopped")

	return &TranscriptionResult{
		Text:      finalText,
		Language:  "en",
		IsPartial: false,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// cleanTranscription removes common artifacts from the transcribed text.
func cleanTranscription(text string) string {
	// The whisper model can output artifacts for silence or unknown audio.
	// We remove them for a cleaner transcription.
	text = strings.ReplaceAll(text, "[BLANK_AUDIO]", "")

	// After replacements, there might be extra whitespace.
	return strings.TrimSpace(text)
}

// transcribeThreadSafe is a thread-safe wrapper for transcription
func (s *STTService) transcribeThreadSafe(audioData []float32) (string, error) {
	// CRITICAL: Lock the model mutex to prevent concurrent access
	s.modelMutex.Lock()
	defer s.modelMutex.Unlock()

	return s.transcribe(audioData)
}

// transcribe processes audio data and returns text (MUST be called with modelMutex locked)
func (s *STTService) transcribe(audioData []float32) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	// Create context for transcription
	context, err := s.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("failed to create context: %w", err)
	}

	// Set context parameters
	context.SetLanguage("en")
	context.SetTranslate(false)
	context.SetThreads(2) // Reduced from 4 to avoid overloading

	// Process the audio
	if err := context.Process(audioData, nil, nil, nil); err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	// Extract text from all segments
	text := ""
	for {
		segment, err := context.NextSegment()
		if err != nil {
			break
		}
		text += segment.Text
	}

	return text, nil
}

// TranscribeFile transcribes an audio file
func (s *STTService) TranscribeFile(filePath string) (*TranscriptionResult, error) {
	return nil, fmt.Errorf("file transcription not yet implemented")
}

// IsRecording returns current recording status
func (s *STTService) IsRecording() bool {
	s.recordingMutex.Lock()
	defer s.recordingMutex.Unlock()
	return s.isRecording
}

// GetSupportedLanguages returns list of supported languages
func (s *STTService) GetSupportedLanguages() []string {
	return []string{
		"en", "es", "fr", "de", "it", "pt", "nl", "pl", "ru",
		"ja", "ko", "zh", "ar", "tr", "hi", "th", "vi",
	}
}

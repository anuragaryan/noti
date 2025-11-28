// Package whisper provides speech-to-text functionality using Whisper
package whisper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"noti/internal/domain"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	transcriptionSampleRate = 16000
)

// Transcriber handles speech-to-text transcription using Whisper
type Transcriber struct {
	model      whisper.Model
	modelMutex sync.Mutex
	config     *domain.STTConfig
	modelPath  string
	ctx        context.Context

	// Audio buffering for real-time transcription
	audioBuffer      []float32
	bufferMutex      sync.Mutex
	processedSamples int
	samplesPerChunk  int
	processingWg     sync.WaitGroup
	isProcessing     bool
	stopProcessing   chan struct{}
}

// NewTranscriber creates a new Whisper transcriber
func NewTranscriber(basePath string, config *domain.STTConfig) (*Transcriber, error) {
	modelFileName := fmt.Sprintf("ggml-%s.bin", config.ModelName)
	modelPath := filepath.Join(basePath, "models", modelFileName)

	// Check if model exists
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

// SetContext sets the Wails runtime context for emitting events
func (t *Transcriber) SetContext(ctx context.Context) {
	t.ctx = ctx
}

// Initialize loads the Whisper model
func (t *Transcriber) Initialize() error {
	fmt.Printf("=== Loading Whisper Model ===\n")
	fmt.Printf("Model path: %s\n", t.modelPath)

	model, err := whisper.New(t.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load whisper model: %w", err)
	}

	t.model = model
	fmt.Println("✓ Whisper model loaded successfully!")
	fmt.Println("✓ Real-time transcription enabled")
	fmt.Printf("✓ Processing chunks every %d seconds\n\n", t.config.ChunkDurationSecs)

	return nil
}

// StartProcessing begins real-time audio processing
func (t *Transcriber) StartProcessing() error {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if t.isProcessing {
		return fmt.Errorf("already processing")
	}

	// Clear previous buffer
	t.audioBuffer = []float32{}
	t.processedSamples = 0
	t.stopProcessing = make(chan struct{})
	t.isProcessing = true

	// Start real-time processing goroutine
	go t.processAudioRealtime()

	fmt.Println("✓ Transcription processing started")
	return nil
}

// StopProcessing stops real-time audio processing and returns final transcription
func (t *Transcriber) StopProcessing() (*domain.TranscriptionResult, error) {
	t.bufferMutex.Lock()

	if !t.isProcessing {
		t.bufferMutex.Unlock()
		return nil, fmt.Errorf("not currently processing")
	}

	fmt.Println("\n=== Stopping Transcription Processing ===")

	// Signal to stop processing
	close(t.stopProcessing)
	t.isProcessing = false

	// Copy the full buffer for safety
	fullBuffer := make([]float32, len(t.audioBuffer))
	copy(fullBuffer, t.audioBuffer)

	totalSamples := len(fullBuffer)
	remainingSamples := totalSamples - t.processedSamples

	fmt.Printf("Total audio: %.2f seconds (%d samples)\n", float64(totalSamples)/float64(transcriptionSampleRate), totalSamples)
	fmt.Printf("Already processed: %.2f seconds (%d samples)\n", float64(t.processedSamples)/float64(transcriptionSampleRate), t.processedSamples)
	fmt.Printf("Remaining: %.2f seconds (%d samples)\n", float64(remainingSamples)/float64(transcriptionSampleRate), remainingSamples)

	t.bufferMutex.Unlock()

	// Wait for any ongoing processing to complete
	fmt.Println("Waiting for ongoing processing to complete...")
	t.processingWg.Wait()
	fmt.Println("All ongoing processing complete")

	var finalText string

	// Process remaining audio even if less than 1 second
	if remainingSamples > transcriptionSampleRate/2 { // At least 0.5 seconds remaining
		fmt.Printf("Processing final chunk of %d samples...\n", remainingSamples)
		finalChunk := make([]float32, remainingSamples)
		copy(finalChunk, fullBuffer[t.processedSamples:])

		text, err := t.TranscribeThreadSafe(finalChunk)
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
	if t.processedSamples == 0 && totalSamples > transcriptionSampleRate {
		fmt.Println("WARNING: No chunks were processed!")
		fmt.Println("Processing entire audio as single chunk...")

		text, err := t.TranscribeThreadSafe(fullBuffer)
		if err != nil {
			fmt.Printf("Full transcription error: %v\n", err)
		} else {
			text = cleanTranscription(text)
			if text != "" {
				finalText = text
				fmt.Printf("Full audio transcribed: '%s'\n", text)
			}
		}
	}

	fmt.Println("✓ Transcription processing stopped")

	return &domain.TranscriptionResult{
		Text:      finalText,
		Language:  "en",
		IsPartial: false,
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// ProcessChunk adds an audio chunk to the buffer for processing
func (t *Transcriber) ProcessChunk(chunk domain.AudioChunk) {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if !t.isProcessing {
		return
	}

	// Append audio data to buffer
	t.audioBuffer = append(t.audioBuffer, chunk.Data...)
}

// processAudioRealtime processes audio chunks in real-time
func (t *Transcriber) processAudioRealtime() {
	ticker := time.NewTicker(time.Duration(t.config.ChunkDurationSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopProcessing:
			return
		case <-ticker.C:
			t.processNextChunk()
		}
	}
}

// processNextChunk transcribes the next audio chunk
func (t *Transcriber) processNextChunk() {
	t.bufferMutex.Lock()

	// Check if we have enough samples for a chunk
	totalSamples := len(t.audioBuffer)
	if totalSamples < t.processedSamples+t.samplesPerChunk {
		t.bufferMutex.Unlock()
		fmt.Println("[Real-time] Not enough samples yet for next chunk")
		return
	}

	// Extract chunk to process
	chunkStart := t.processedSamples
	chunkEnd := chunkStart + t.samplesPerChunk
	if chunkEnd > totalSamples {
		chunkEnd = totalSamples
	}

	chunk := make([]float32, chunkEnd-chunkStart)
	copy(chunk, t.audioBuffer[chunkStart:chunkEnd])

	t.processedSamples = chunkEnd

	t.bufferMutex.Unlock()

	// Increment wait group for this processing task
	t.processingWg.Add(1)

	// Process this chunk in background
	go func(audioChunk []float32, start, end int) {
		defer t.processingWg.Done()

		fmt.Printf("\n[Real-time] Processing chunk: %.1f-%.1f seconds (samples: %d-%d)\n",
			float64(start)/float64(transcriptionSampleRate),
			float64(end)/float64(transcriptionSampleRate),
			start, end)

		text, err := t.TranscribeThreadSafe(audioChunk)
		if err != nil {
			fmt.Printf("[Real-time] Transcription error: %v\n", err)
			return
		}

		// Clean and check if we have meaningful text
		text = cleanTranscription(text)

		if text != "" {
			fmt.Printf("[Real-time] Transcribed: '%s'\n", text)

			// Emit real-time transcription event to frontend
			if t.ctx != nil {
				result := domain.TranscriptionResult{
					Text:      text,
					Language:  "en",
					IsPartial: true,
					Timestamp: time.Now().Format(time.RFC3339),
				}

				fmt.Printf("[Real-time] Emitting event: transcription:partial with text: '%s'\n", result.Text)
				runtime.EventsEmit(t.ctx, "transcription:partial", result)
				fmt.Println("[Real-time] Event emitted successfully")
			} else {
				fmt.Println("[Real-time] WARNING: Context is nil, cannot emit event!")
			}
		} else {
			fmt.Println("[Real-time] No text transcribed (silence or noise)")
		}
	}(chunk, chunkStart, chunkEnd)
}

// TranscribeThreadSafe is a thread-safe wrapper for transcription
func (t *Transcriber) TranscribeThreadSafe(audioData []float32) (string, error) {
	// CRITICAL: Lock the model mutex to prevent concurrent access
	t.modelMutex.Lock()
	defer t.modelMutex.Unlock()

	return t.transcribe(audioData)
}

// transcribe processes audio data and returns text (MUST be called with modelMutex locked)
func (t *Transcriber) transcribe(audioData []float32) (string, error) {
	if t.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	// Create context for transcription
	context, err := t.model.NewContext()
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

// IsProcessing returns whether transcription is in progress
func (t *Transcriber) IsProcessing() bool {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	return t.isProcessing
}

// Cleanup releases resources
func (t *Transcriber) Cleanup() {
	fmt.Println("Cleaning up transcriber...")

	// Wait for all processing to complete
	t.processingWg.Wait()

	if t.model != nil {
		t.model.Close()
	}
}

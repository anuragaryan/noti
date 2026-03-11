package whisper

import (
	"fmt"
	"strings"

	gowhisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type speechEngine interface {
	Transcribe(audio []float32) (string, error)
	DetectedLanguage() string
	Close() error
}

type whisperEngine struct {
	model    gowhisper.Model
	language string
	threads  uint
	detected string
}

func newWhisperEngine(model gowhisper.Model, opts Options) *whisperEngine {
	return &whisperEngine{
		model:    model,
		language: opts.Language,
		threads:  opts.Threads,
	}
}

func (e *whisperEngine) Transcribe(audio []float32) (string, error) {
	if e.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	ctx, err := e.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("failed to create whisper context: %w", err)
	}

	if err := ctx.SetLanguage(e.language); err != nil {
		return "", fmt.Errorf("failed to set language: %w", err)
	}
	ctx.SetTranslate(false)
	ctx.SetThreads(e.threads)

	if err := ctx.Process(audio, nil, nil, nil); err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	e.detected = strings.ToLower(strings.TrimSpace(ctx.DetectedLanguage()))

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

func (e *whisperEngine) Close() error {
	if e.model == nil {
		return nil
	}
	return e.model.Close()
}

func (e *whisperEngine) DetectedLanguage() string {
	return e.detected
}

package whisper

import (
	"context"

	"noti/internal/domain"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type transcriptionEmitter interface {
	EmitPartial(result domain.TranscriptionResult)
	EmitDone(result domain.TranscriptionResult)
}

type noopEmitter struct{}

func (noopEmitter) EmitPartial(domain.TranscriptionResult) {}
func (noopEmitter) EmitDone(domain.TranscriptionResult)    {}

type wailsEmitter struct {
	ctx context.Context
}

func newWailsEmitter(ctx context.Context) transcriptionEmitter {
	if ctx == nil {
		return noopEmitter{}
	}
	return &wailsEmitter{ctx: ctx}
}

func (e *wailsEmitter) EmitPartial(result domain.TranscriptionResult) {
	runtime.EventsEmit(e.ctx, "transcription:partial", result)
}

func (e *wailsEmitter) EmitDone(result domain.TranscriptionResult) {
	runtime.EventsEmit(e.ctx, "transcription:done", result)
}

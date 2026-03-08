package events

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const DownloadEventName = "download:event"

type DownloadKind string

const (
	DownloadKindSTTModel    DownloadKind = "stt-model"
	DownloadKindLLMModel    DownloadKind = "llm-model"
	DownloadKindLlamaServer DownloadKind = "llama-server"
)

type DownloadStatus string

const (
	DownloadStatusQueued      DownloadStatus = "queued"
	DownloadStatusDownloading DownloadStatus = "downloading"
	DownloadStatusCompleted   DownloadStatus = "completed"
	DownloadStatusError       DownloadStatus = "error"
)

type DownloadEvent struct {
	ID              string         `json:"id"`
	Kind            DownloadKind   `json:"kind"`
	Label           string         `json:"label"`
	Status          DownloadStatus `json:"status"`
	BytesDownloaded int64          `json:"bytesDownloaded"`
	TotalBytes      int64          `json:"totalBytes"`
	Percent         float64        `json:"percent"`
	Error           string         `json:"error,omitempty"`
	Timestamp       time.Time      `json:"timestamp"`
}

func EmitDownloadEvent(ctx context.Context, evt DownloadEvent) {
	if ctx == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	runtime.EventsEmit(ctx, DownloadEventName, evt)
}

func CalculatePercent(downloaded, total int64) float64 {
	if total <= 0 || downloaded <= 0 {
		return 0
	}
	return (float64(downloaded) / float64(total)) * 100
}

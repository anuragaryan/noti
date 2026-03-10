// Package downloader downloads GGUF-format LLM models from Hugging Face,
package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	hfBaseURL = "https://huggingface.co"
)

// ---------------------------------------------------------------------------
// Model registry
// ---------------------------------------------------------------------------

// LLMModelEntry describes a known model: its Hugging Face repo and exact filename.
type LLMModelEntry struct {
	// ModelCode is the short name a caller may pass to Download.
	ModelCode string
	// Repo is the HuggingFace "owner/repo" string.
	Repo string
	// File is the exact GGUF filename inside the repo.
	File string
	// Description is a human-readable label shown in error messages / listings.
	Description string
}

var (
	registryMu     sync.RWMutex
	registry       []LLMModelEntry
	registryReason string
)

// SetLLMRegistry updates the runtime model catalogue used by downloader APIs.
func SetLLMRegistry(entries []LLMModelEntry) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registryReason = ""

	registry = make([]LLMModelEntry, len(entries))
	for i := range entries {
		registry[i] = LLMModelEntry{
			ModelCode:   entries[i].ModelCode,
			Repo:        entries[i].Repo,
			File:        entries[i].File,
			Description: entries[i].Description,
		}
	}
}

// SetLLMRegistryUnavailable marks LLM registry unavailable with a reason.
func SetLLMRegistryUnavailable(reason string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = []LLMModelEntry{}
	registryReason = reason
}

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// DownloadResult contains metadata about the completed download.
type DownloadResult struct {
	ModelName string // the model code that was requested
	Repo      string // HuggingFace repo
	File      string // exact filename on HF
	URL       string // full download URL
	DestPath  string // absolute path to the saved file
	SizeBytes int64  // final file size
	Skipped   bool   // true when file already existed and was not re-downloaded
}

// ErrUnknownModel is returned when the requested model code is not in Registry.
var ErrUnknownModel = errors.New("unknown model")

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Download fetches the GGUF file for modelName into opts.DestDir.
// modelName must match ModelCode in Registry exactly.
//
// Example:
//
//	result, err := llmmodel.Download(ctx, "gemma-3-1b-it-q4_k_m", &llmmodel.DownloadOptions{
//	    DestDir: "./models",
//	    ProgressFunc: func(dl, total int64) {
//	        if total > 0 { fmt.Printf("\r%.1f%%", float64(dl)/float64(total)*100) }
//	    },
//	})
func DownloadLLM(ctx context.Context, modelName string, opts *DownloadOptions) (*DownloadResult, error) {
	if opts == nil {
		opts = &DownloadOptions{}
	}
	opts.applyDefaults()

	entry, err := Lookup(modelName)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/resolve/main/%s", hfBaseURL, entry.Repo, entry.File)
	destPath := filepath.Join(opts.DestDir, entry.File)

	// Skip if already present.
	if fi, err2 := os.Stat(destPath); err2 == nil {
		return &DownloadResult{
			ModelName: modelName,
			Repo:      entry.Repo,
			File:      entry.File,
			URL:       url,
			DestPath:  destPath,
			SizeBytes: fi.Size(),
			Skipped:   true,
		}, nil
	}

	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dest dir: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		lastErr = downloadFile(ctx, opts.HTTPClient, url, destPath, opts.ProgressFunc, nil)
		if lastErr == nil {
			break
		}
		if isContextError(lastErr) {
			return nil, lastErr
		}
		if attempt < opts.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(opts.RetryDelay):
			}
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("download %q after %d attempts: %w", entry.File, opts.MaxRetries, lastErr)
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("stat downloaded file: %w", err)
	}

	return &DownloadResult{
		ModelName: modelName,
		Repo:      entry.Repo,
		File:      entry.File,
		URL:       url,
		DestPath:  destPath,
		SizeBytes: fi.Size(),
	}, nil
}

// Lookup finds a LLMModelEntry by exact model code.
// Returns ErrUnknownModel (with a helpful list) when not found.
func Lookup(modelName string) (*LLMModelEntry, error) {
	if reason := llmUnavailableReason(); reason != "" {
		return nil, fmt.Errorf("llm model catalog unavailable: %s", reason)
	}

	requested := strings.ToLower(modelName)
	entries := ListModels()
	for i := range entries {
		if strings.ToLower(entries[i].ModelCode) == requested {
			entry := entries[i]
			return &entry, nil
		}
	}

	availableModels := make([]string, 0, len(entries)+1)
	availableModels = append(availableModels, fmt.Sprintf("%q\n\nSupported models:", modelName))
	for _, e := range entries {
		availableModels = append(availableModels, fmt.Sprintf("  %-36s  %s", e.ModelCode, e.Description))
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownModel, strings.Join(availableModels, "\n"))
}

// ListModels returns a copy of the model registry for display purposes.
func ListModels() []LLMModelEntry {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]LLMModelEntry, len(registry))
	for i := range registry {
		out[i] = LLMModelEntry{
			ModelCode:   registry[i].ModelCode,
			Repo:        registry[i].Repo,
			File:        registry[i].File,
			Description: registry[i].Description,
		}
	}
	return out
}

func llmUnavailableReason() string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registryReason
}

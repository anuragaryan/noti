// Package downloader downloads GGUF-format LLM models from Hugging Face,
package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	hfBaseURL = "https://huggingface.co"
)

// ---------------------------------------------------------------------------
// Model registry
// ---------------------------------------------------------------------------

// ModelEntry describes a known model: its Hugging Face repo and exact filename.
type ModelEntry struct {
	// Aliases are the short names a caller may pass to Download.
	Aliases []string
	// Repo is the HuggingFace "owner/repo" string.
	Repo string
	// File is the exact GGUF filename inside the repo.
	File string
	// Description is a human-readable label shown in error messages / listings.
	Description string
}

// Registry is the built-in model catalogue.  Add entries here to support new
// models without changing any other code.
var Registry = []ModelEntry{
	{
		Aliases:     []string{"gemma-2-2b-it-q4_k_m", "gemma-2-2b-it"},
		Repo:        "ggml-org/gemma-2-2b-it-GGUF",
		File:        "gemma-2-2b-it-Q4_K_M.gguf",
		Description: "Google Gemma 2 2B Instruct, Q4_K_M quantisation",
	},
	{
		Aliases:     []string{"gemma-2-9b-it-q4_k_m", "gemma-2-9b-it"},
		Repo:        "ggml-org/gemma-2-9b-it-GGUF",
		File:        "gemma-2-9b-it-Q4_K_M.gguf",
		Description: "Google Gemma 2 9B Instruct, Q4_K_M quantisation",
	},
	{
		Aliases:     []string{"Qwen3.5-4B-UD-Q4_K_XL", "Qwen3.5-4B-UD"},
		Repo:        "unsloth/Qwen3.5-4B-GGUF",
		File:        "Qwen3.5-4B-UD-Q4_K_XL.gguf",
		Description: "Alibaba Qwen 3.5 4B, Q4_K_XL quantisation",
	},
	{
		Aliases:     []string{"Qwen3.5-9B-UD-Q4_K_XL", "Qwen3.5-9B-UD"},
		Repo:        "unsloth/Qwen3.5-9B-GGUF",
		File:        "Qwen3.5-9B-UD-Q4_K_XL.gguf",
		Description: "Alibaba Qwen 3.5 9B, Q4_K_XL quantisation",
	},
}

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// DownloadResult contains metadata about the completed download.
type DownloadResult struct {
	ModelName string // the alias that was requested
	Repo      string // HuggingFace repo
	File      string // exact filename on HF
	URL       string // full download URL
	DestPath  string // absolute path to the saved file
	SizeBytes int64  // final file size
	Skipped   bool   // true when file already existed and was not re-downloaded
}

// ErrUnknownModel is returned when the requested model alias is not in Registry.
var ErrUnknownModel = errors.New("unknown model")

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Download fetches the GGUF file for modelName into opts.DestDir.
// modelName must match one of the Aliases in Registry (case-insensitive).
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

	entry, err := Lookup(ctx, modelName)
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

// Lookup finds a ModelEntry by alias (case-insensitive).
// Returns ErrUnknownModel (with a helpful list) when not found.
func Lookup(ctx context.Context, modelName string) (*ModelEntry, error) {
	lower := strings.ToLower(modelName)
	for i := range Registry {
		for _, alias := range Registry[i].Aliases {
			if strings.ToLower(alias) == lower {
				return &Registry[i], nil
			}
		}
	}

	availableModels := make([]string, 0, len(Registry)+1)
	availableModels = append(availableModels, fmt.Sprintf("%q\n\nSupported models:", modelName))
	for _, e := range Registry {
		availableModels = append(availableModels, fmt.Sprintf("  %-36s  %s", e.Aliases[0], e.Description))
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownModel, strings.Join(availableModels, "\n"))
}

// ListModels returns a copy of the model registry for display purposes.
func ListModels() []ModelEntry {
	out := make([]ModelEntry, len(Registry))
	copy(out, Registry)
	return out
}

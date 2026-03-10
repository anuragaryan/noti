package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	baseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
)

var (
	sttRegistryMu     sync.RWMutex
	validModels       []string
	modelChecksums    = map[string]string{}
	sttRegistryReason string
)

// STTModelEntry describes a supported STT model and optional checksum.
type STTModelEntry struct {
	Code     string
	Checksum string
}

// SetSTTRegistry updates the known STT models and optional checksums.
func SetSTTRegistry(entries []STTModelEntry) {
	sttRegistryMu.Lock()
	defer sttRegistryMu.Unlock()
	sttRegistryReason = ""

	validModels = make([]string, 0, len(entries))
	modelChecksums = make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.Code == "" {
			continue
		}
		validModels = append(validModels, entry.Code)
		modelChecksums[entry.Code] = entry.Checksum
	}
}

// SetSTTRegistryUnavailable marks STT registry unavailable with a reason.
func SetSTTRegistryUnavailable(reason string) {
	sttRegistryMu.Lock()
	defer sttRegistryMu.Unlock()
	validModels = []string{}
	modelChecksums = map[string]string{}
	sttRegistryReason = reason
}

// DownloadModel downloads the named GGUF/GGML Whisper model to opts.DestDir.
// It skips the download if the file already exists and (when a checksum is
// registered) verifies the integrity of the file on disk.
//
// Example:
//
//	err := whisper.DownloadModel(ctx, "base.en", &whisper.DownloadOptions{
//	    DestDir: "./models",
//	    ProgressFunc: func(dl, total int64) {
//	        fmt.Printf("\r%.1f%%", float64(dl)/float64(total)*100)
//	    },
//	})
func DownloadModel(ctx context.Context, model string, opts *DownloadOptions) error {
	if opts == nil {
		opts = &DownloadOptions{}
	}
	opts.applyDefaults()

	if reason := sttUnavailableReason(); reason != "" {
		return fmt.Errorf("stt model catalog unavailable: %s", reason)
	}

	if !isValidModel(model) {
		return fmt.Errorf("unknown model %q — run ListModels() to see valid options", model)
	}

	destPath := filepath.Join(opts.DestDir, "ggml-"+model+".bin")

	// If the file already exists, optionally verify and skip download.
	if _, err := os.Stat(destPath); err == nil {
		if checksum := checksumForModel(model); checksum != "" {
			if verifyErr := verifyChecksum(destPath, checksum); verifyErr != nil {
				// File is corrupt — delete it so we re-download below.
				_ = os.Remove(destPath)
			} else {
				return nil // File present and valid.
			}
		} else {
			return nil // File present, no checksum to verify.
		}
	}

	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	url := buildURL(model)

	var lastErr error
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		lastErr = downloadFile(ctx, opts.HTTPClient, url, destPath, opts.ProgressFunc, nil)
		if lastErr == nil {
			break
		}
		if isContextError(lastErr) {
			return lastErr // Don't retry on explicit cancellation.
		}
		if attempt < opts.MaxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.RetryDelay):
			}
		}
	}
	if lastErr != nil {
		return fmt.Errorf("download model %q after %d attempts: %w", model, opts.MaxRetries, lastErr)
	}

	// Post-download checksum verification.
	if checksum := checksumForModel(model); checksum != "" {
		if err := verifyChecksum(destPath, checksum); err != nil {
			_ = os.Remove(destPath)
			return fmt.Errorf("checksum mismatch for model %q: %w", model, err)
		}
	}

	return nil
}

// ModelPath returns the expected file path for a given model and directory.
func ModelPath(model, destDir string) string {
	return filepath.Join(destDir, "ggml-"+model+".bin")
}

// ListGGMLModels returns all supported model names.
func ListGGMLModels() []string {
	sttRegistryMu.RLock()
	defer sttRegistryMu.RUnlock()

	out := make([]string, len(validModels))
	copy(out, validModels)
	return out
}

// --- internal helpers -------------------------------------------------------

func isValidModel(model string) bool {
	sttRegistryMu.RLock()
	defer sttRegistryMu.RUnlock()
	return slices.Contains(validModels, model)
}

func checksumForModel(model string) string {
	sttRegistryMu.RLock()
	defer sttRegistryMu.RUnlock()
	return modelChecksums[model]
}

func sttUnavailableReason() string {
	sttRegistryMu.RLock()
	defer sttRegistryMu.RUnlock()
	return sttRegistryReason
}

func buildURL(model string) string {
	return fmt.Sprintf("%s/ggml-%s.bin", baseURL, model)
}

func verifyChecksum(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("got %s, want %s", got, expected)
	}
	return nil
}

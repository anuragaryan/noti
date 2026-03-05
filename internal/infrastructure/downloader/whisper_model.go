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
	"time"
)

const (
	baseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
)

// Known SHA256 checksums for model files. Add more as needed.
// Leave empty string to skip verification for that model.
var modelChecksums = map[string]string{
	"small.en-q8_0":       "67a179f608ea6114bd3fdb9060e762b588a3fb3bd00c4387971be4d177958067",
	"medium.en-q8_0":      "43fa2cd084de5a04399a896a9a7a786064e221365c01700cea4666005218f11c",
	"large-v3-turbo-q5_0": "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2",
	"large-v3-turbo-q8_0": "317eb69c11673c9de1e1f0d459b253999804ec71ac4c23c17ecf5fbe24e259a1",
	"large-v3-q5_0":       "d75795ecff3f83b5faa89d1900604ad8c780abd5739fae406de19f23ecd98ad1",
}

// How much free RAM?
//├── <2GB free → small.en-q8_0 (minimal hardware)
//├── 2-3GB free → medium.en-q8_0
//├── 3-5GB free → large-v3-turbo-q5_0 (best everyday)
//└── 5-8GB free → large-v3-turbo-q8_0
//└── 8+GB free → large-v3-q5_0 (best performing)

// ValidModels is the list of all supported model names.
var ValidModels = []string{
	"large-v3-turbo-q5_0", "large-v3-turbo-q8_0", "medium.en-q8_0", "small.en-q8_0", "large-v3-q5_0",
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

	if !isValidModel(model) {
		return fmt.Errorf("unknown model %q — run ListModels() to see valid options", model)
	}

	destPath := filepath.Join(opts.DestDir, "ggml-"+model+".bin")

	// If the file already exists, optionally verify and skip download.
	if _, err := os.Stat(destPath); err == nil {
		if checksum, ok := modelChecksums[model]; ok && checksum != "" {
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
	if checksum, ok := modelChecksums[model]; ok && checksum != "" {
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
	out := make([]string, len(ValidModels))
	copy(out, ValidModels)
	return out
}

// --- internal helpers -------------------------------------------------------

func isValidModel(model string) bool {
	return slices.Contains(ValidModels, model)
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

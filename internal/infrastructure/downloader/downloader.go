// Package downloader provides utilities for downloading resources using scripts
package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMaxRetries = 3
	defaultRetryDelay = 3 * time.Second
	defaultTimeout    = 60 * time.Minute
)

// DownloadOptions configures the download behaviour.
type DownloadOptions struct {
	// DestDir is the directory where the file will be saved.
	// Defaults to the current working directory.
	DestDir string

	// MaxRetries is the number of retry attempts on transient failures.
	// Defaults to 3.
	MaxRetries int

	// RetryDelay is the wait time between retries.
	// Defaults to 3 seconds.
	RetryDelay time.Duration

	// ProgressFunc is called periodically with (bytesDownloaded, totalBytes).
	// totalBytes may be -1 if the server does not send Content-Length.
	// Set to nil to disable progress reporting.
	ProgressFunc func(downloaded, total int64)

	// HTTPClient allows injecting a custom *http.Client (e.g. with a proxy).
	// Defaults to a client with a 60-minute timeout.
	HTTPClient *http.Client
}

func (o *DownloadOptions) applyDefaults() {
	if o.MaxRetries == 0 {
		o.MaxRetries = defaultMaxRetries
	}
	if o.RetryDelay == 0 {
		o.RetryDelay = defaultRetryDelay
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
	if o.DestDir == "" {
		o.DestDir = "."
	}
}

// Downloader handles downloading resources using embedded scripts
type Downloader struct {
	script []byte
}

// NewDownloader creates a new downloader with the given script
func NewDownloader(script []byte) *Downloader {
	return &Downloader{
		script: script,
	}
}

// Download executes the download script in the specified directory with arguments
func (d *Downloader) Download(targetDir string, args ...string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Write the download script
	scriptPath := filepath.Join(targetDir, ".download-script.sh")
	if err := os.WriteFile(scriptPath, d.script, 0755); err != nil {
		return fmt.Errorf("failed to write download script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Run the download script
	cmd := exec.Command("/bin/bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = targetDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	// Always print output for debugging
	fmt.Printf("Download script output:\n%s\n", string(output))

	if err != nil {
		return fmt.Errorf("download script failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// DownloadWithCustomScript downloads using a custom script path
func (d *Downloader) DownloadWithCustomScript(scriptPath, targetDir string, args ...string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Run the download script
	cmd := exec.Command("/bin/bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = targetDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	// Always print output for debugging
	fmt.Printf("Download script output:\n%s\n", string(output))

	if err != nil {
		return fmt.Errorf("download script failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// downloadFile streams url → destPath atomically (write to temp, then rename).
func downloadFile(
	ctx context.Context,
	client *http.Client,
	url, destPath string,
	progress func(int64, int64),
	headers map[string]string,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// good
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("access denied (%s) — model may be gated; supply a token", resp.Status)
	case http.StatusNotFound:
		return fmt.Errorf("file not found (%s): %s", resp.Status, url)
	default:
		return fmt.Errorf("unexpected HTTP status %s for %s", resp.Status, url)
	}

	total := resp.ContentLength // -1 if unknown

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".download-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath) // no-op after successful rename
	}()

	var downloaded int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := tmp.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write: %w", wErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read body: %w", readErr)
		}
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpPath, destPath)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		(err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded")))
}

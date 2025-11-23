// Package downloader provides utilities for downloading resources using scripts
package downloader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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

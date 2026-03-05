// Package downloader downloads the latest pre-built llama.cpp release binaries
// from https://github.com/ggml-org/llama.cpp for the current (or a specified) platform.

package downloader

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPILatest = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Backend selects a specific GPU / compute backend for the binary.
// Use BackendCPU for the plain CPU build (the default).
type Backend string

const (
	BackendCPU    Backend = "cpu"
	BackendCUDA12 Backend = "cuda12"
	BackendCUDA13 Backend = "cuda13"
	BackendROCm   Backend = "rocm"
	BackendVulkan Backend = "vulkan"
	BackendMetal  Backend = "metal"  // macOS only – included automatically on Apple Silicon
	BackendNoAVX  Backend = "noavx"  // Windows only
	BackendAVX    Backend = "avx"    // Windows only
	BackendAVX2   Backend = "avx2"   // Windows only
	BackendAVX512 Backend = "avx512" // Windows only
)

// Platform fully identifies a target platform.  Zero value resolves to the
// host platform automatically via DetectPlatform().
type Platform struct {
	OS      string  // "linux", "darwin", "windows"
	Arch    string  // "amd64", "arm64", "s390x"
	Backend Backend // GPU/compute backend; empty → BackendCPU
}

// LlamaDownloadOptions configures the download behaviour.
type LlamaDownloadOptions struct {
	DownloadOptions

	// Platform overrides auto-detection.  Leave zero to use the host platform.
	Platform Platform

	// ExtractZip, when true, extracts the zip archive after downloading and
	// removes the zip file.  Defaults to false (zip is kept as-is).
	ExtractZip bool
}

// ReleaseInfo contains metadata about the release that was downloaded.
type ReleaseInfo struct {
	Tag       string // e.g. "b8198"
	AssetName string // zip filename
	AssetURL  string // direct download URL
	DestPath  string // path to the downloaded zip (or extract dir if ExtractZip)
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Download fetches the latest llama.cpp release binary for the resolved
// platform and returns metadata about what was downloaded.

func DownloadLlama(ctx context.Context, opts *LlamaDownloadOptions) (*ReleaseInfo, error) {
	if opts == nil {
		opts = &LlamaDownloadOptions{}
	}
	opts.applyDefaults()

	platform := opts.Platform
	if platform.OS == "" {
		platform = DetectPlatform()
	}
	if platform.Backend == "" {
		platform.Backend = BackendCPU
	}

	// Fetch the latest release metadata from GitHub API.
	release, err := fetchLatestRelease(ctx, opts.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	// Find the asset that matches the requested platform.
	assetName, assetURL, err := matchAsset(release.Assets, platform)
	if err != nil {
		available := assetNames(release.Assets)
		return nil, fmt.Errorf("%w\n\nAvailable assets for tag %s:\n  %s",
			err, release.TagName, strings.Join(available, "\n  "))
	}

	destZip := filepath.Join(opts.DestDir, assetName)

	// Skip download if the zip already exists.
	zipExists := false
	if _, statErr := os.Stat(destZip); statErr == nil {
		zipExists = true
		if !opts.ExtractZip {
			// Caller only wants the zip — return immediately.
			return &ReleaseInfo{
				Tag:       release.TagName,
				AssetName: assetName,
				AssetURL:  assetURL,
				DestPath:  destZip,
			}, nil
		}
		// ExtractZip=true: check whether the extract dir already exists too.
		extractDir := strings.TrimSuffix(destZip, ".zip")
		if _, err2 := os.Stat(extractDir); err2 == nil {
			return &ReleaseInfo{
				Tag:       release.TagName,
				AssetName: assetName,
				AssetURL:  assetURL,
				DestPath:  extractDir,
			}, nil
		}
		// Zip exists but extract dir does not — skip download, go straight to extraction.
	}

	if !zipExists {
		if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
			return nil, fmt.Errorf("create dest dir: %w", err)
		}

		// Download with retries.
		var lastErr error
		for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
			lastErr = downloadFile(ctx, opts.HTTPClient, assetURL, destZip, opts.ProgressFunc, nil)
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
			return nil, fmt.Errorf("download %s after %d attempts: %w", assetName, opts.MaxRetries, lastErr)
		}
	}

	info := &ReleaseInfo{
		Tag:       release.TagName,
		AssetName: assetName,
		AssetURL:  assetURL,
		DestPath:  destZip,
	}

	// Optionally extract the zip.
	if opts.ExtractZip {
		extractDir := strings.TrimSuffix(destZip, ".zip")
		if err := extractZip(destZip, extractDir); err != nil {
			return nil, fmt.Errorf("extract zip: %w", err)
		}
		_ = os.Remove(destZip)
		info.DestPath = extractDir
	}

	return info, nil
}

// DetectPlatform returns the Platform for the current host (CPU backend).
func DetectPlatform() Platform {
	return Platform{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Backend: BackendCPU,
	}
}

// ---------------------------------------------------------------------------
// GitHub API types
// ---------------------------------------------------------------------------

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func fetchLatestRelease(ctx context.Context, client *http.Client) (*ghRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPILatest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API returned %s: %s", resp.Status, bytes.TrimSpace(body))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode GitHub response: %w", err)
	}
	return &rel, nil
}

// matchAsset finds the best-matching asset zip for the given platform.
//
// Asset name patterns (from actual releases):
//
//	llama-<tag>-bin-macos-arm64.zip
//	llama-<tag>-bin-macos-x64.zip
//	llama-<tag>-bin-ubuntu-x64.zip
//	llama-<tag>-bin-ubuntu-x64-vulkan.zip
//	llama-<tag>-bin-ubuntu-x64-rocm-7.2.zip  (version suffix varies)
//	llama-<tag>-bin-ubuntu-s390x.zip
//	llama-<tag>-bin-win-arm64-msvc.zip
//	llama-<tag>-bin-win-cuda12-x64.zip
//	llama-<tag>-bin-win-cuda13-x64.zip
//	llama-<tag>-bin-win-vulkan-x64.zip
//	llama-<tag>-bin-win-noavx-x64.zip
//	llama-<tag>-bin-win-avx-x64.zip
//	llama-<tag>-bin-win-avx2-x64.zip
//	llama-<tag>-bin-win-avx512-x64.zip
func matchAsset(assets []ghAsset, p Platform) (name, url string, err error) {
	// Build the keyword list the asset name must contain (in order).
	var must []string

	switch p.OS {
	case "darwin":
		must = append(must, "macos")
		switch p.Arch {
		case "arm64":
			must = append(must, "arm64")
		case "amd64":
			must = append(must, "x64")
		default:
			return "", "", fmt.Errorf("unsupported macOS arch %q", p.Arch)
		}

	case "linux":
		must = append(must, "ubuntu")
		switch p.Arch {
		case "amd64":
			must = append(must, "x64")
		case "s390x":
			must = append(must, "s390x")
		case "arm64":
			must = append(must, "arm64")
		default:
			return "", "", fmt.Errorf("unsupported Linux arch %q", p.Arch)
		}
		switch p.Backend {
		case BackendVulkan:
			must = append(must, "vulkan")
		case BackendROCm:
			must = append(must, "rocm")
		case BackendCPU, "":
			// no extra keyword — the plain ubuntu-x64.zip has no suffix
		default:
			return "", "", fmt.Errorf("unsupported Linux backend %q", p.Backend)
		}

	case "windows":
		must = append(must, "win")
		switch p.Arch {
		case "arm64":
			must = append(must, "arm64")
		case "amd64":
			switch p.Backend {
			case BackendCUDA12:
				must = append(must, "cuda12")
			case BackendCUDA13:
				must = append(must, "cuda13")
			case BackendVulkan:
				must = append(must, "vulkan")
			case BackendNoAVX:
				must = append(must, "noavx")
			case BackendAVX:
				// "avx-x64" — must not match avx2 / avx512
				must = append(must, "avx-x64")
			case BackendAVX2:
				must = append(must, "avx2")
			case BackendAVX512:
				must = append(must, "avx512")
			case BackendCPU, "":
				// plain CPU build uses avx2 as the "default" on Windows
				must = append(must, "avx2")
			default:
				return "", "", fmt.Errorf("unsupported Windows backend %q", p.Backend)
			}
		default:
			return "", "", fmt.Errorf("unsupported Windows arch %q", p.Arch)
		}

	default:
		return "", "", fmt.Errorf("unsupported OS %q", p.OS)
	}

	// Find the first asset whose lowercase name contains all required keywords.
	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if !strings.HasSuffix(lower, ".zip") {
			continue
		}
		match := true
		for _, kw := range must {
			if !strings.Contains(lower, kw) {
				match = false
				break
			}
		}
		if match {
			// Extra guard: for plain Linux CPU builds, reject vulkan/rocm assets.
			if p.OS == "linux" && (p.Backend == BackendCPU || p.Backend == "") {
				if strings.Contains(lower, "vulkan") || strings.Contains(lower, "rocm") {
					continue
				}
			}
			return a.Name, a.BrowserDownloadURL, nil
		}
	}

	return "", "", fmt.Errorf("no asset found for platform %s/%s backend=%s",
		p.OS, p.Arch, p.Backend)
}

func assetNames(assets []ghAsset) []string {
	var names []string
	for _, a := range assets {
		if strings.HasSuffix(a.Name, ".zip") {
			names = append(names, a.Name)
		}
	}
	return names
}

// extractZip extracts all files from zipPath into destDir.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for _, f := range r.File {
		// Sanitise path to prevent zip-slip.
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator),
			filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

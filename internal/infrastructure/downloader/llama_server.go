// Package downloader downloads the latest pre-built llama.cpp release binaries
// from https://github.com/ggml-org/llama.cpp for the current (or a specified) platform.

package downloader

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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

	// Extract, when true, extracts the archive (zip or tar.gz) after downloading
	// and removes the archive file.  Defaults to false (archive is kept as-is).
	Extract bool
}

// ReleaseInfo contains metadata about the release that was downloaded.
type ReleaseInfo struct {
	Tag       string // e.g. "b8198"
	AssetName string // archive filename (zip or tar.gz)
	AssetURL  string // direct download URL
	DestPath  string // path to the downloaded archive (or extract dir if Extract=true)
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

	destArchive := filepath.Join(opts.DestDir, assetName)

	// Determine the extract directory by stripping the archive suffix.
	extractDir := archiveExtractDir(destArchive)

	// Skip download if the archive already exists.
	archiveExists := false
	if _, statErr := os.Stat(destArchive); statErr == nil {
		archiveExists = true
		if !opts.Extract {
			// Caller only wants the archive — return immediately.
			return &ReleaseInfo{
				Tag:       release.TagName,
				AssetName: assetName,
				AssetURL:  assetURL,
				DestPath:  destArchive,
			}, nil
		}
		// Extract=true: check whether opts.DestDir already has content beyond
		// the archive file itself (i.e. a previous extraction was flattened in).
		if entries, err := os.ReadDir(opts.DestDir); err == nil && len(entries) > 1 {
			return &ReleaseInfo{
				Tag:       release.TagName,
				AssetName: assetName,
				AssetURL:  assetURL,
				DestPath:  opts.DestDir,
			}, nil
		}
		// Archive exists but dest dir has no extracted content — skip download,
		// go straight to extraction.
	}

	if !archiveExists {
		// If Extract=true and the dest dir already has content (archive was
		// previously deleted after extraction), skip the download entirely.
		if opts.Extract {
			if entries, err := os.ReadDir(opts.DestDir); err == nil && len(entries) > 0 {
				return &ReleaseInfo{
					Tag:       release.TagName,
					AssetName: assetName,
					AssetURL:  assetURL,
					DestPath:  opts.DestDir,
				}, nil
			}
		}

		if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
			return nil, fmt.Errorf("create dest dir: %w", err)
		}

		// Download with retries.
		var lastErr error
		for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
			lastErr = downloadFile(ctx, opts.HTTPClient, assetURL, destArchive, opts.ProgressFunc, nil)
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
		DestPath:  destArchive,
	}

	// Optionally extract the archive.
	if opts.Extract {
		if err := extractArchive(destArchive, extractDir); err != nil {
			return nil, fmt.Errorf("extract archive: %w", err)
		}
		_ = os.Remove(destArchive)

		// If the extracted directory contains a single subdirectory (the
		// top-level folder bundled inside the archive), promote its children
		// directly into opts.DestDir and remove the now-empty wrapper.
		if err := flattenSingleSubdir(extractDir, opts.DestDir); err != nil {
			return nil, fmt.Errorf("flatten extracted directory: %w", err)
		}

		info.DestPath = opts.DestDir
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

// archiveExtractDir returns the destination directory for extraction by
// stripping the archive suffix (.zip or .tar.gz) from the archive path.
func archiveExtractDir(archivePath string) string {
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return strings.TrimSuffix(archivePath, ".tar.gz")
	}
	return strings.TrimSuffix(archivePath, ".zip")
}

// flattenSingleSubdir checks whether dir contains exactly one entry and that
// entry is a directory.  If so, it moves all children of that subdirectory
// into destDir and removes the (now-empty) subdirectory and dir itself.
// If dir does not contain exactly one subdirectory the function is a no-op.
func flattenSingleSubdir(dir, destDir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		// Nothing to flatten — content is already at the right level.
		return nil
	}

	subdir := filepath.Join(dir, entries[0].Name())
	children, err := os.ReadDir(subdir)
	if err != nil {
		return err
	}

	for _, child := range children {
		src := filepath.Join(subdir, child.Name())
		dst := filepath.Join(destDir, child.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move %s → %s: %w", src, dst, err)
		}
	}

	// Remove the now-empty subdirectory and the wrapper directory.
	_ = os.Remove(subdir)
	_ = os.Remove(dir)
	return nil
}

// isSupportedArchive reports whether the name ends with a supported archive suffix.
func isSupportedArchive(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz")
}

// extractArchive dispatches to the correct extraction function based on the
// archive file extension (.zip or .tar.gz).
func extractArchive(archivePath, destDir string) error {
	lower := strings.ToLower(archivePath)
	if strings.HasSuffix(lower, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	}
	return extractZip(archivePath, destDir)
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

// matchAsset finds the best-matching asset archive (zip or tar.gz) for the
// given platform.
//
// Asset name patterns (from actual releases):
//
//	llama-<tag>-bin-macos-arm64.zip
//	llama-<tag>-bin-macos-x64.zip
//	llama-<tag>-bin-ubuntu-x64.zip
//	llama-<tag>-bin-ubuntu-x64.tar.gz
//	llama-<tag>-bin-ubuntu-x64-vulkan.zip
//	llama-<tag>-bin-ubuntu-x64-vulkan.tar.gz
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
			// no extra keyword — the plain ubuntu-x64 archive has no suffix
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

	// Find matching assets. Prefer .zip over .tar.gz when both are available.
	var zipMatch, tarGzMatch *ghAsset
	for i := range assets {
		a := &assets[i]
		lower := strings.ToLower(a.Name)
		if !isSupportedArchive(lower) {
			continue
		}
		match := true
		for _, kw := range must {
			if !strings.Contains(lower, kw) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Extra guard: for plain Linux CPU builds, reject vulkan/rocm assets.
		if p.OS == "linux" && (p.Backend == BackendCPU || p.Backend == "") {
			if strings.Contains(lower, "vulkan") || strings.Contains(lower, "rocm") {
				continue
			}
		}
		if strings.HasSuffix(lower, ".zip") && zipMatch == nil {
			zipMatch = a
		} else if strings.HasSuffix(lower, ".tar.gz") && tarGzMatch == nil {
			tarGzMatch = a
		}
	}

	if zipMatch != nil {
		return zipMatch.Name, zipMatch.BrowserDownloadURL, nil
	}
	if tarGzMatch != nil {
		return tarGzMatch.Name, tarGzMatch.BrowserDownloadURL, nil
	}

	return "", "", fmt.Errorf("no asset found for platform %s/%s backend=%s",
		p.OS, p.Arch, p.Backend)
}

func assetNames(assets []ghAsset) []string {
	var names []string
	for _, a := range assets {
		if isSupportedArchive(a.Name) {
			names = append(names, a.Name)
		}
	}
	return names
}

// extractTarGz extracts all files from a .tar.gz archive into destDir.
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Sanitise path to prevent tar-slip.
		target := filepath.Join(destDir, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator),
			filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path in tar: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			out.Close()
			if copyErr != nil {
				return copyErr
			}
		case tar.TypeSymlink:
			// Sanitise symlink target to prevent escaping destDir.
			linkTarget := hdr.Linkname
			if filepath.IsAbs(linkTarget) {
				return fmt.Errorf("illegal absolute symlink target in tar: %s", linkTarget)
			}
			resolved := filepath.Join(filepath.Dir(target), linkTarget)
			if !strings.HasPrefix(filepath.Clean(resolved)+string(os.PathSeparator),
				filepath.Clean(destDir)+string(os.PathSeparator)) {
				return fmt.Errorf("illegal symlink target escapes destDir: %s -> %s", hdr.Name, linkTarget)
			}
			if err := os.Symlink(linkTarget, target); err != nil && !os.IsExist(err) {
				return err
			}
		default:
			// Skip unsupported entry types (hard links, devices, etc.).
		}
	}
	return nil
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

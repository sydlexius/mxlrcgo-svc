// Package ffmpeg resolves an ffmpeg executable for the verification and
// instrumental-detection sidecars, auto-provisioning a checksum-pinned static
// build when ffmpeg is neither configured nor on PATH. The download is verified
// against a pinned SHA256 before the extracted binary is ever used; the archive
// itself is never executed.
package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Downloader fetches a remote artifact. It is injected so tests never touch the
// network; the default implementation streams over HTTPS.
type Downloader interface {
	Download(ctx context.Context, rawURL string) (io.ReadCloser, error)
}

// Options configures Resolve.
type Options struct {
	// CacheDir holds auto-provisioned builds. Required for the cache and
	// auto-download resolution steps; when empty those steps are skipped.
	CacheDir string
	// HTTPClient backs the default downloader. Ignored when Downloader is set.
	HTTPClient *http.Client
	// Downloader is an injectable fetcher (tests). nil selects the default
	// HTTPS downloader.
	Downloader Downloader
}

// Resolve returns an absolute path to an ffmpeg executable, in priority order:
//
//  1. explicit override (a configured path) - resolved via PATH lookup, with a
//     hard error if missing; air-gapped users expect a failure here, not a
//     silent download.
//  2. a previously auto-provisioned binary cached under CacheDir.
//  3. an `ffmpeg` already on PATH.
//  4. an auto-downloaded, checksum-verified pinned build (requires CacheDir).
func Resolve(ctx context.Context, override string, opts Options) (string, error) {
	// (1) explicit override: hard error if missing.
	if o := strings.TrimSpace(override); o != "" {
		resolved, err := exec.LookPath(o)
		if err != nil {
			return "", fmt.Errorf("ffmpeg: configured ffmpeg path %q not found: %w", o, err)
		}
		return resolved, nil
	}

	// (2) cached auto-provisioned binary.
	if opts.CacheDir != "" {
		if cached := cachedBinary(opts.CacheDir); cached != "" {
			return cached, nil
		}
	}

	// (3) ffmpeg on PATH.
	if resolved, err := exec.LookPath("ffmpeg"); err == nil {
		return resolved, nil
	}

	// (4) auto-download.
	return provision(ctx, opts)
}

// binaryName is the ffmpeg executable filename for the current platform.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

// cachedPath is the absolute path the auto-provisioned binary lives at under
// cacheDir for the pinned version.
func cachedPath(cacheDir string) string {
	return filepath.Join(cacheDir, "ffmpeg-"+version, binaryName())
}

// cachedBinary returns the cached binary path if it exists and passes a cheap
// liveness check (a non-empty regular file, executable on POSIX). It returns ""
// when no usable cached binary is present.
//
// A cache hit is trusted by presence and permissions; it is NOT re-hashed
// against the pinned SHA256. This is a deliberate trade-off: re-reading the
// ~80MB binary on every serve boot is wasteful, and re-hashing buys nothing
// against the only attacker who can corrupt it - one with write access to the
// cache directory could equally swap the binary AND any stored hash. The
// checksum gate that matters runs once, at download time, before the binary is
// ever written to the cache.
func cachedBinary(cacheDir string) string {
	path := cachedPath(cacheDir)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return ""
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return path
}

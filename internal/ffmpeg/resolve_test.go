package ffmpeg

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// failDownloader fails if Download is ever called: precedence tests use it to
// prove an earlier resolution step short-circuited before the download step.
type failDownloader struct{ t *testing.T }

func (d failDownloader) Download(context.Context, string) (io.ReadCloser, error) {
	d.t.Fatalf("downloader called, but an earlier step should have resolved")
	return nil, errors.New("unreachable")
}

// makeExecStub writes an executable stub named name into dir and returns its
// path. It skips on Windows, where a bare-name shell script is not executable
// via exec.LookPath.
func makeExecStub(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("exec stub not supported on windows")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return p
}

func TestResolve_OverrideWins(t *testing.T) {
	dir := t.TempDir()
	stub := makeExecStub(t, dir, "my-ffmpeg")
	// A different ffmpeg on PATH must be ignored in favor of the override.
	pathDir := t.TempDir()
	makeExecStub(t, pathDir, "ffmpeg")
	t.Setenv("PATH", pathDir)

	got, err := Resolve(context.Background(), stub, Options{
		CacheDir:   t.TempDir(),
		Downloader: failDownloader{t},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != stub {
		t.Fatalf("path = %q, want override %q", got, stub)
	}
}

func TestResolve_OverrideMissingErrors(t *testing.T) {
	_, err := Resolve(context.Background(), "/nonexistent/ffmpeg-xyz", Options{
		Downloader: failDownloader{t},
	})
	if err == nil {
		t.Fatal("expected error for missing override, got nil")
	}
}

func TestResolve_CacheWinsOverPathAndDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH stub not supported on windows")
	}
	cacheDir := t.TempDir()
	// Pre-populate the cache with a live binary.
	versionDir := filepath.Join(cacheDir, "ffmpeg-"+version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cachedPath(cacheDir), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write cached: %v", err)
	}
	// Also place an ffmpeg on PATH; cache must win.
	pathDir := t.TempDir()
	makeExecStub(t, pathDir, "ffmpeg")
	t.Setenv("PATH", pathDir)

	got, err := Resolve(context.Background(), "", Options{
		CacheDir:   cacheDir,
		Downloader: failDownloader{t},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != cachedPath(cacheDir) {
		t.Fatalf("path = %q, want cached %q", got, cachedPath(cacheDir))
	}
}

func TestResolve_PathWinsOverDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH stub not supported on windows")
	}
	pathDir := t.TempDir()
	stub := makeExecStub(t, pathDir, "ffmpeg")
	t.Setenv("PATH", pathDir)

	// Empty cache dir: no cached binary, so PATH is the resolver - download
	// must not be reached.
	got, err := Resolve(context.Background(), "", Options{
		CacheDir:   t.TempDir(),
		Downloader: failDownloader{t},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != stub {
		t.Fatalf("path = %q, want PATH stub %q", got, stub)
	}
}

func TestCachedBinary_RejectsNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits differ on windows")
	}
	cacheDir := t.TempDir()
	versionDir := filepath.Join(cacheDir, "ffmpeg-"+version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Non-executable file must not count as a live cached binary.
	if err := os.WriteFile(cachedPath(cacheDir), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := cachedBinary(cacheDir); got != "" {
		t.Fatalf("cachedBinary = %q, want empty for non-executable file", got)
	}
	// Empty file must also be rejected.
	if err := os.WriteFile(cachedPath(cacheDir), nil, 0o755); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if got := cachedBinary(cacheDir); got != "" {
		t.Fatalf("cachedBinary = %q, want empty for zero-byte file", got)
	}
}

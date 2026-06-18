package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ulikunitz/xz"
)

// fixtureBinary is the dummy content placed where ffmpeg lives in the archive.
var fixtureBinary = []byte("#!/bin/sh\necho ffmpeg-fixture\n")

// archiveBinPath is the in-archive path of the fixture binary, platform-correct
// (ffmpeg.exe on Windows) so exact-match extraction works everywhere.
func archiveBinPath() string { return "ffmpeg-pkg/bin/" + binaryName() }

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func buildTarXz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	tw := tar.NewWriter(xw)
	// A directory entry and a decoy file, to prove extraction picks the binary.
	writeTar(t, tw, "ffmpeg-pkg/README.txt", []byte("docs"))
	writeTar(t, tw, archiveBinPath(), fixtureBinary)
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}
	return buf.Bytes()
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write: %v", err)
	}
}

func buildZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string][]byte{
		"ffmpeg-pkg/README.txt": []byte("docs"),
		archiveBinPath():        fixtureBinary,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// plainDownloader fetches over plain HTTP for the httptest server (the default
// downloader refuses non-https). It records how many fetches happened.
type plainDownloader struct{ hits int32 }

func (d *plainDownloader) Download(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	atomic.AddInt32(&d.hits, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// useFixtureArtifact swaps resolveArtifact for one returning art (restored on
// cleanup) and hides ffmpeg from PATH so resolution must reach the download step.
func useFixtureArtifact(t *testing.T, art artifact) {
	t.Helper()
	prev := resolveArtifact
	resolveArtifact = func(_, _ string) (artifact, error) { return art, nil }
	t.Cleanup(func() { resolveArtifact = prev })
	t.Setenv("PATH", t.TempDir()) // empty dir: no ffmpeg on PATH
}

func TestProvision_SuccessExtractsCachesAndReuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    archiveKind
		payload []byte
	}{
		{"tar.xz", archiveTarXz, buildTarXz(t)},
		{"zip", archiveZip, buildZip(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(tc.payload)
			}))
			defer srv.Close()

			useFixtureArtifact(t, artifact{
				URL:              srv.URL + "/ffmpeg.archive",
				SHA256:           sha256Hex(tc.payload),
				BinPathInArchive: archiveBinPath(),
				kind:             tc.kind,
			})
			dl := &plainDownloader{}
			cacheDir := t.TempDir()
			opts := Options{CacheDir: cacheDir, Downloader: dl}

			got, err := Resolve(context.Background(), "", opts)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != cachedPath(cacheDir) {
				t.Fatalf("path = %q, want %q", got, cachedPath(cacheDir))
			}
			content, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("read extracted binary: %v", err)
			}
			if !bytes.Equal(content, fixtureBinary) {
				t.Fatalf("extracted content = %q, want fixture", content)
			}
			if hits := atomic.LoadInt32(&dl.hits); hits != 1 {
				t.Fatalf("download hits = %d, want 1", hits)
			}

			// Second resolve must hit the cache, not re-download.
			got2, err := Resolve(context.Background(), "", opts)
			if err != nil {
				t.Fatalf("second Resolve: %v", err)
			}
			if got2 != got {
				t.Fatalf("second path = %q, want %q", got2, got)
			}
			if hits := atomic.LoadInt32(&dl.hits); hits != 1 {
				t.Fatalf("after cache hit, download hits = %d, want 1 (no re-fetch)", hits)
			}
		})
	}
}

func TestProvision_ChecksumMismatchRejected(t *testing.T) {
	payload := buildTarXz(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	useFixtureArtifact(t, artifact{
		URL:              srv.URL + "/ffmpeg.archive",
		SHA256:           strings.Repeat("0", 64), // deliberately wrong
		BinPathInArchive: archiveBinPath(),
		kind:             archiveTarXz,
	})
	cacheDir := t.TempDir()
	opts := Options{CacheDir: cacheDir, Downloader: &plainDownloader{}}

	_, err := Resolve(context.Background(), "", opts)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(cachedPath(cacheDir)); !os.IsNotExist(statErr) {
		t.Fatalf("binary must not be cached on mismatch; stat err = %v", statErr)
	}
}

func TestProvision_NoCacheDir(t *testing.T) {
	useFixtureArtifact(t, builds[platformKey{os: "linux", arch: "amd64"}])
	_, err := Resolve(context.Background(), "", Options{})
	if err == nil || !strings.Contains(err.Error(), "cache directory") {
		t.Fatalf("error = %v, want cache directory error", err)
	}
}

func TestArtifactFor(t *testing.T) {
	if _, err := artifactFor("linux", "amd64"); err != nil {
		t.Fatalf("linux/amd64: unexpected error %v", err)
	}
	dErr := mustErr(t, "darwin", "arm64")
	if !strings.Contains(dErr.Error(), "macOS") {
		t.Fatalf("darwin error = %v, want macOS guidance", dErr)
	}
	uErr := mustErr(t, "plan9", "mips")
	if !strings.Contains(uErr.Error(), "no pinned build") {
		t.Fatalf("unknown error = %v, want no-pinned-build", uErr)
	}
}

func mustErr(t *testing.T, goos, goarch string) error {
	t.Helper()
	_, err := artifactFor(goos, goarch)
	if err == nil {
		t.Fatalf("%s/%s: expected error", goos, goarch)
	}
	return err
}

func TestMatchesBinary(t *testing.T) {
	want := "ffmpeg-pkg/bin/" + binaryName()
	for _, entry := range []string{want, "./" + want, "other-pkg/bin/" + binaryName()} {
		if !matchesBinary(entry, want) {
			t.Errorf("matchesBinary(%q) = false, want true", entry)
		}
	}
	if matchesBinary("ffmpeg-pkg/README.txt", want) {
		t.Error("matchesBinary(README) = true, want false")
	}
}

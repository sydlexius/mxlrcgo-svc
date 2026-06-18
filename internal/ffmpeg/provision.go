package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// maxArtifactBytes bounds how much we will read from an archive entry or the
// download stream, guarding against a decompression bomb. BtbN ffmpeg builds
// are ~80MB; 512MB is a generous ceiling.
const maxArtifactBytes = 512 << 20

// resolveArtifact selects the pinned build for the running platform. It is a
// package var so tests can substitute a fixture artifact without touching the
// real pinned builds map.
var resolveArtifact = artifactFor

// provision downloads the pinned build for the current platform, verifies its
// SHA256, extracts the ffmpeg binary into CacheDir, and returns its path. The
// archive is never executed; only the checksum-verified binary is extracted.
func provision(ctx context.Context, opts Options) (string, error) {
	art, err := resolveArtifact(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if opts.CacheDir == "" {
		return "", fmt.Errorf("ffmpeg: cannot auto-provision without a cache directory; set media.ffmpeg_path or install ffmpeg")
	}

	dl := opts.Downloader
	if dl == nil {
		dl = &httpDownloader{client: opts.HTTPClient}
	}

	versionDir := filepath.Join(opts.CacheDir, "ffmpeg-"+version)
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		return "", fmt.Errorf("ffmpeg: create cache dir: %w", err)
	}

	// Stream the download into a temp file in the cache dir (same filesystem as
	// the final binary, so the later rename is atomic) while hashing it.
	archivePath, err := downloadVerified(ctx, dl, art, versionDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(archivePath) }()

	finalPath := cachedPath(opts.CacheDir)
	if err := extractBinary(art, archivePath, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

// downloadVerified streams the artifact to a temp file in dir, computes its
// SHA256, and rejects a mismatch. It returns the temp archive path on success.
func downloadVerified(ctx context.Context, dl Downloader, art artifact, dir string) (string, error) {
	body, err := dl.Download(ctx, art.URL)
	if err != nil {
		return "", fmt.Errorf("ffmpeg: download %s: %w", art.URL, err)
	}
	defer func() { _ = body.Close() }()

	tmp, err := os.CreateTemp(dir, "download-*.archive")
	if err != nil {
		return "", fmt.Errorf("ffmpeg: create temp archive: %w", err)
	}
	tmpPath := tmp.Name()

	hasher := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(body, maxArtifactBytes))
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg: download body: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg: write temp archive: %w", closeErr)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, art.SHA256) {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg: checksum mismatch for %s: got %s, want %s", art.URL, got, art.SHA256)
	}
	return tmpPath, nil
}

// extractBinary pulls the ffmpeg binary out of the verified archive, writes it
// to a temp file, chmods it executable, and atomically renames it to dest.
func extractBinary(art artifact, archivePath, dest string) error {
	tmpBin, err := os.CreateTemp(filepath.Dir(dest), "ffmpeg-*.tmp")
	if err != nil {
		return fmt.Errorf("ffmpeg: create temp binary: %w", err)
	}
	tmpBinPath := tmpBin.Name()
	cleanup := func() {
		_ = tmpBin.Close()
		_ = os.Remove(tmpBinPath)
	}

	var extractErr error
	switch art.kind {
	case archiveTarXz:
		extractErr = extractTarXz(archivePath, art.BinPathInArchive, tmpBin)
	case archiveZip:
		extractErr = extractZip(archivePath, art.BinPathInArchive, tmpBin)
	default:
		extractErr = fmt.Errorf("ffmpeg: unknown archive kind %d", art.kind)
	}
	if extractErr != nil {
		cleanup()
		return extractErr
	}
	if err := tmpBin.Chmod(0o755); err != nil {
		cleanup()
		return fmt.Errorf("ffmpeg: chmod binary: %w", err)
	}
	if err := tmpBin.Close(); err != nil {
		_ = os.Remove(tmpBinPath)
		return fmt.Errorf("ffmpeg: close binary: %w", err)
	}
	if err := os.Rename(tmpBinPath, dest); err != nil {
		_ = os.Remove(tmpBinPath)
		return fmt.Errorf("ffmpeg: install binary: %w", err)
	}
	return nil
}

// extractTarXz finds wantPath inside a .tar.xz archive and copies it to w.
func extractTarXz(archivePath, wantPath string, w io.Writer) error {
	f, err := os.Open(archivePath) //nolint:gosec // reason: archivePath is our own sha256-verified temp file created under the cache dir, not user input
	if err != nil {
		return fmt.Errorf("ffmpeg: open archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("ffmpeg: open xz stream: %w", err)
	}
	tr := tar.NewReader(xzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ffmpeg: read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !matchesBinary(hdr.Name, wantPath) {
			continue
		}
		if _, err := io.Copy(w, io.LimitReader(tr, maxArtifactBytes)); err != nil {
			return fmt.Errorf("ffmpeg: extract binary: %w", err)
		}
		return nil
	}
	return fmt.Errorf("ffmpeg: ffmpeg binary %q not found in archive", wantPath)
}

// extractZip finds wantPath inside a .zip archive and copies it to w.
func extractZip(archivePath, wantPath string, w io.Writer) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("ffmpeg: open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || !matchesBinary(zf.Name, wantPath) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("ffmpeg: open zip entry: %w", err)
		}
		_, copyErr := io.Copy(w, io.LimitReader(rc, maxArtifactBytes))
		_ = rc.Close()
		if copyErr != nil {
			return fmt.Errorf("ffmpeg: extract binary: %w", copyErr)
		}
		return nil
	}
	return fmt.Errorf("ffmpeg: ffmpeg binary %q not found in archive", wantPath)
}

// matchesBinary reports whether an archive entry is the ffmpeg binary we want.
// It accepts the exact pinned path or, defensively, any entry ending in the
// canonical bin/ffmpeg[.exe] suffix (tolerant of a top-level directory rename
// in a future pinned build).
func matchesBinary(entry, wantPath string) bool {
	entry = path.Clean(strings.TrimPrefix(entry, "./"))
	if entry == path.Clean(wantPath) {
		return true
	}
	return strings.HasSuffix(entry, "/bin/"+binaryName())
}

// httpsOnlyRedirect rejects any redirect hop that is not https, preventing a
// silent downgrade to cleartext mid-download.
func httpsOnlyRedirect(req *http.Request, _ []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing non-https redirect to %s", req.URL.Redacted())
	}
	return nil
}

// httpDownloader is the default HTTPS downloader.
type httpDownloader struct {
	client *http.Client
}

func (d *httpDownloader) Download(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-https download: %s", rawURL)
	}
	client := d.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	// The scheme guard above only covers the initial URL. Go's http.Client
	// silently follows redirects, so without a CheckRedirect hook an https->http
	// redirect would downgrade the (~80MB) fetch to cleartext. The checksum still
	// gates use, but reject the downgrade as defense in depth and to honor the
	// "over HTTPS" contract. Shallow-copy so we never mutate a caller's client.
	if client.CheckRedirect == nil {
		c := *client
		c.CheckRedirect = httpsOnlyRedirect
		client = &c
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return resp.Body, nil
}

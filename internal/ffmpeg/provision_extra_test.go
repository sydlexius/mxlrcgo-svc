package ffmpeg

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

// errDownloader always fails, exercising provision's download-error path.
type errDownloader struct{}

func (errDownloader) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("boom")
}

func serveBytes(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProvision_DownloadError(t *testing.T) {
	useFixtureArtifact(t, builds[platformKey{os: "linux", arch: "amd64"}])
	_, err := Resolve(context.Background(), "", Options{CacheDir: t.TempDir(), Downloader: errDownloader{}})
	if err == nil || !strings.Contains(err.Error(), "download") {
		t.Fatalf("error = %v, want download error", err)
	}
}

func TestProvision_BinaryNotInArchive(t *testing.T) {
	// A tar.xz containing only a README; the wanted binary path is absent and the
	// suffix fallback cannot match either.
	var buf bytes.Buffer
	xw, _ := xz.NewWriter(&buf)
	tw := tar.NewWriter(xw)
	writeTar(t, tw, "pkg/README.txt", []byte("docs"))
	_ = tw.Close()
	_ = xw.Close()
	payload := buf.Bytes()

	srv := serveBytes(t, payload)
	useFixtureArtifact(t, artifact{
		URL:              srv.URL,
		SHA256:           sha256Hex(payload),
		BinPathInArchive: "pkg/does-not-exist",
		kind:             archiveTarXz,
	})
	_, err := Resolve(context.Background(), "", Options{CacheDir: t.TempDir(), Downloader: &plainDownloader{}})
	if err == nil || !strings.Contains(err.Error(), "not found in archive") {
		t.Fatalf("error = %v, want not-found-in-archive", err)
	}
}

func TestProvision_CorruptArchives(t *testing.T) {
	for _, kind := range []archiveKind{archiveTarXz, archiveZip} {
		payload := []byte("this is not a valid archive")
		srv := serveBytes(t, payload)
		useFixtureArtifact(t, artifact{
			URL:              srv.URL,
			SHA256:           sha256Hex(payload),
			BinPathInArchive: archiveBinPath(),
			kind:             kind,
		})
		_, err := Resolve(context.Background(), "", Options{CacheDir: t.TempDir(), Downloader: &plainDownloader{}})
		if err == nil {
			t.Fatalf("kind %d: expected extraction error, got nil", kind)
		}
	}
}

func TestProvision_UnknownArchiveKind(t *testing.T) {
	payload := buildTarXz(t)
	srv := serveBytes(t, payload)
	useFixtureArtifact(t, artifact{
		URL:              srv.URL,
		SHA256:           sha256Hex(payload),
		BinPathInArchive: archiveBinPath(),
		kind:             archiveKind(99),
	})
	_, err := Resolve(context.Background(), "", Options{CacheDir: t.TempDir(), Downloader: &plainDownloader{}})
	if err == nil || !strings.Contains(err.Error(), "unknown archive kind") {
		t.Fatalf("error = %v, want unknown-archive-kind", err)
	}
}

func TestHTTPDownloader(t *testing.T) {
	ctx := context.Background()
	d := &httpDownloader{}

	if _, err := d.Download(ctx, "http://example.com/x"); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("http url: err = %v, want non-https refusal", err)
	}
	if _, err := d.Download(ctx, "https://exa mple.com/x"); err == nil {
		t.Fatal("invalid url: expected error, got nil")
	}

	// Success over TLS, using the test server's client (trusts its cert).
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer tls.Close()
	ok := &httpDownloader{client: tls.Client()}
	rc, err := ok.Download(ctx, tls.URL)
	if err != nil {
		t.Fatalf("tls download: %v", err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(body) != "payload" {
		t.Fatalf("body = %q, want payload", body)
	}

	// Non-200 status is an error.
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()
	d404 := &httpDownloader{client: bad.Client()}
	if _, err := d404.Download(ctx, bad.URL); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("404: err = %v, want status error", err)
	}
}

func TestHTTPDownloader_RejectsHTTPSDowngradeRedirect(t *testing.T) {
	ctx := context.Background()
	// The cleartext target the redirect would downgrade to.
	httpTarget := serveBytes(t, []byte("downgraded"))

	// An https server that 302-redirects to the http target.
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, httpTarget.URL, http.StatusFound)
	}))
	defer tls.Close()

	// Use the TLS server's client (trusts its cert) but with no CheckRedirect, so
	// our https-only policy is the thing under test.
	d := &httpDownloader{client: tls.Client()}
	if _, err := d.Download(ctx, tls.URL); err == nil || !strings.Contains(err.Error(), "non-https redirect") {
		t.Fatalf("err = %v, want non-https redirect refusal", err)
	}
}

// TestProvision_IgnoresTraversalEntries locks the invariant that the extracted
// destination is fixed (cachedPath), never derived from the archive entry name,
// so a path-traversal or absolute entry cannot write outside the cache.
func TestProvision_IgnoresTraversalEntries(t *testing.T) {
	var buf bytes.Buffer
	xw, _ := xz.NewWriter(&buf)
	tw := tar.NewWriter(xw)
	writeTar(t, tw, "../../../evil", []byte("pwned"))
	writeTar(t, tw, "/abs/evil", []byte("pwned"))
	writeTar(t, tw, archiveBinPath(), fixtureBinary)
	_ = tw.Close()
	_ = xw.Close()
	payload := buf.Bytes()

	srv := serveBytes(t, payload)
	cacheDir := t.TempDir()
	useFixtureArtifact(t, artifact{
		URL:              srv.URL,
		SHA256:           sha256Hex(payload),
		BinPathInArchive: archiveBinPath(),
		kind:             archiveTarXz,
	})

	got, err := Resolve(context.Background(), "", Options{CacheDir: cacheDir, Downloader: &plainDownloader{}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != cachedPath(cacheDir) {
		t.Fatalf("path = %q, want %q", got, cachedPath(cacheDir))
	}
	content, _ := os.ReadFile(got)
	if !bytes.Equal(content, fixtureBinary) {
		t.Fatalf("extracted content = %q, want fixture binary", content)
	}

	// Only the binary may exist under the cache dir; no traversal entry escaped.
	var files []string
	if err := filepath.WalkDir(cacheDir, func(p string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !de.IsDir() {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(files) != 1 || files[0] != got {
		t.Fatalf("cache dir files = %v, want only %q", files, got)
	}
	// And nothing named "evil" leaked to the cache dir's parent.
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(cacheDir), "evil")); !os.IsNotExist(statErr) {
		t.Fatalf("traversal entry escaped: stat err = %v", statErr)
	}
}

func TestProvision_CacheDirCreateFails(t *testing.T) {
	// Point CacheDir at a path whose parent is a file, so MkdirAll fails.
	notADir := t.TempDir() + "/file"
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	useFixtureArtifact(t, builds[platformKey{os: "linux", arch: "amd64"}])
	_, err := Resolve(context.Background(), "", Options{CacheDir: notADir, Downloader: &plainDownloader{}})
	if err == nil || !strings.Contains(err.Error(), "cache dir") {
		t.Fatalf("error = %v, want cache dir create error", err)
	}
}

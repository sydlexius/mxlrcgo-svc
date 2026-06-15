package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStaticHandlerServesCSS confirms the embedded Tailwind output is served
// under /static/ with a revalidation cache header. CSS is regenerated per
// release under the same filename (no content hash), so "no-cache" is correct:
// clients revalidate on each request rather than serving stale CSS after a
// release. Fonts use a separate immutable policy (see TestStaticHandlerServesFont).
func TestStaticHandlerServesCSS(t *testing.T) {
	h := StaticHandler()

	req := httptest.NewRequest(http.MethodGet, "/static/css/output.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET output.css status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache (CSS is not content-fingerprinted)", cc)
	}
}

// TestStaticHandlerServesFont confirms a self-hosted font is embedded and served
// (offline single-binary requirement) with an immutable long-lived cache header.
// Fonts are content-stable binaries that do not change between releases, so the
// immutable policy is safe and avoids unnecessary revalidation round-trips.
func TestStaticHandlerServesFont(t *testing.T) {
	h := StaticHandler()

	req := httptest.NewRequest(http.MethodGet, "/static/fonts/InterVariable.woff2", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET InterVariable.woff2 status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable (fonts are content-stable)", cc)
	}
}

// TestStaticHandlerMissingAsset returns 404 for an asset not in the embed FS
// and must not set any cache header (fail closed: do not cache misses, and in
// particular never advertise "immutable" for a path that does not exist).
func TestStaticHandlerMissingAsset(t *testing.T) {
	h := StaticHandler()

	req := httptest.NewRequest(http.MethodGet, "/static/css/does-not-exist.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing asset status = %d, want 404", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("Cache-Control = %q on 404, want empty (misses must not be cached)", cc)
	}
}

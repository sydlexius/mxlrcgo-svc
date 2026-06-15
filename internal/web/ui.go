package web

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/sydlexius/mxlrcgo-svc/internal/config"
	"github.com/sydlexius/mxlrcgo-svc/web/templates"
)

// UI renders the serve-mode web pages. It holds the effective configuration (for
// the Config view) and the build version (for the sidebar). The config is read
// only for rendering and is never mutated.
type UI struct {
	cfg     config.Config
	version string
}

// NewUI builds the web UI renderer from the effective config and build version.
func NewUI(cfg config.Config, version string) *UI {
	return &UI{cfg: cfg, version: version}
}

// Register wires the web UI routes onto mux: the static asset handler, a root
// redirect to /config, and the Reports and Config pages. Routes are GET-only;
// the JSON API and its method patterns are registered separately by the server.
func (u *UI) Register(mux *http.ServeMux) {
	mux.Handle("GET "+staticPrefix, StaticHandler())
	mux.HandleFunc("GET /{$}", u.handleRoot)
	mux.HandleFunc("GET /reports", u.handleReports)
	mux.HandleFunc("GET /config", u.handleConfig)
}

// handleRoot redirects the bare root to the Config view, the default landing
// page for v1 (Reports is still a placeholder).
func (u *UI) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/config", http.StatusFound)
}

// handleReports renders the Reports placeholder page.
func (u *UI) handleReports(w http.ResponseWriter, r *http.Request) {
	render(w, r, templates.ReportsPage(u.version))
}

// handleConfig renders the effective configuration with secrets redacted.
// FormatConfigText is the single redaction source of truth (shared with the
// logging layer), so api.token and server.webhook_api_keys are masked before
// the text reaches the template. Source-hint maps are nil: the view shows the
// merged effective values, not per-field provenance.
func (u *UI) handleConfig(w http.ResponseWriter, r *http.Request) {
	toml := config.FormatConfigText(u.cfg, nil, nil)
	// Even with secrets redacted, the effective config exposes operational
	// detail (paths, intervals, provider lanes); keep it out of browser and
	// intermediary caches.
	w.Header().Set("Cache-Control", "no-store")
	render(w, r, templates.ConfigPage(u.version, toml))
}

// render writes a templ component as the HTML response. It renders into a buffer
// first so that a render failure can return a clean 500 rather than a 200 with a
// half-written body. The status line is not committed until the buffer is ready.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("web UI render failed", "path", r.URL.Path, "error", err) //nolint:gosec // G706 false positive: r.URL.Path logged as a structured slog key-value, not a format string / execution sink
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

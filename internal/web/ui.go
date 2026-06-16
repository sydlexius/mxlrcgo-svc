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
// only for rendering and is never mutated. When an Auth is attached (issue #204,
// lane 3) the UI routes are gated by RequireSession and the /login + /logout
// endpoints are registered; without it the routes are public (the #210 default).
type UI struct {
	cfg        config.Config
	version    string
	auth       *Auth
	onboarding *Onboarding
}

// UIOption customizes a UI.
type UIOption func(*UI)

// WithAuth attaches the session-auth subsystem: it gates the UI routes behind
// RequireSession and registers the login/logout endpoints. Omitting it leaves
// the routes public (the #210 behavior).
func WithAuth(a *Auth) UIOption {
	return func(u *UI) { u.auth = a }
}

// WithOnboarding attaches the first-run onboarding flow (issue #204, lane 4): it
// registers the /setup endpoints and redirects the UI routes to /setup until an
// admin exists. It is meaningful only alongside WithAuth (onboarding feeds the
// login session); without auth it is ignored.
func WithOnboarding(o *Onboarding) UIOption {
	return func(u *UI) { u.onboarding = o }
}

// AttachOnboarding wires the onboarding flow onto an already-constructed UI. It
// is the post-construction equivalent of WithOnboarding, used by the server
// layer where the UI is built first (WithWebUIAuth) and onboarding attached
// after.
func (u *UI) AttachOnboarding(o *Onboarding) { u.onboarding = o }

// NewUI builds the web UI renderer from the effective config and build version.
func NewUI(cfg config.Config, version string, opts ...UIOption) *UI {
	u := &UI{cfg: cfg, version: version}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// Register wires the web UI routes onto mux: the static asset handler, a root
// redirect to /config, and the Reports and Config pages. Routes are GET-only;
// the JSON API and its method patterns are registered separately by the server.
// When an Auth is attached the page routes are wrapped in RequireSession and the
// /login (GET/POST) and /logout (POST) endpoints are registered; the static
// assets and the login endpoints themselves stay public.
func (u *UI) Register(mux *http.ServeMux) {
	mux.Handle("GET "+staticPrefix, StaticHandler())
	if u.auth != nil {
		mux.HandleFunc("GET /login", u.auth.handleLoginForm)
		mux.HandleFunc("POST /login", u.auth.handleLogin)
		mux.HandleFunc("POST /logout", u.auth.handleLogout)
		if u.onboarding != nil {
			mux.HandleFunc("GET /setup", u.onboarding.handleSetupForm)
			mux.HandleFunc("POST /setup", u.onboarding.handleSetup)
		}
		// The page guard is RequireSession, wrapped (when onboarding is present)
		// by FirstRunGate so an un-onboarded daemon redirects to /setup before the
		// session check runs.
		guard := func(h http.Handler) http.Handler {
			sess := u.auth.RequireSession(h)
			if u.onboarding != nil {
				return u.onboarding.FirstRunGate(sess)
			}
			return sess
		}
		mux.Handle("GET /{$}", guard(http.HandlerFunc(u.handleRoot)))
		mux.Handle("GET /reports", guard(http.HandlerFunc(u.handleReports)))
		mux.Handle("GET /config", guard(http.HandlerFunc(u.handleConfig)))
		return
	}
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

// render writes a templ component as the HTML response with an implicit 200.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	renderWithStatus(w, r, http.StatusOK, c)
}

// renderWithStatus writes a templ component as the HTML response with the given
// status code. It renders into a buffer first so that a render failure can
// return a clean 500 rather than a partial body under a committed 200; the
// status line is not written until the buffer is ready.
func renderWithStatus(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("web UI render failed", "path", r.URL.Path, "error", err) //nolint:gosec // G706 false positive: r.URL.Path logged as a structured slog key-value, not a format string / execution sink
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

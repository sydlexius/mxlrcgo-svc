package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sydlexius/mxlrcgo-svc/internal/config"
)

// secretCfg returns a config carrying both secret-bearing fields populated, so
// redaction tests have something to mask.
func secretCfg() config.Config {
	cfg := config.Config{}
	cfg.API.Token = "tok_SUPERSECRET_TOKEN_VALUE"
	cfg.Server.WebhookAPIKeys = []string{"mxlrc_secretkey_one", "mxlrc_secretkey_two"}
	return cfg
}

// newUIServer registers the UI routes on a fresh mux for httptest.
func newUIServer(cfg config.Config, version string) *http.ServeMux {
	mux := http.NewServeMux()
	NewUI(cfg, version).Register(mux)
	return mux
}

// TestConfigViewRedactsSecrets is the required in-package proof that the Config
// view never emits a secret-bearing value. It populates the token and webhook
// keys, renders the live /config response, and asserts the raw secrets are
// absent while the redaction marker is present.
func TestConfigViewRedactsSecrets(t *testing.T) {
	mux := newUIServer(secretCfg(), "v9.9.9")

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /config status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	for _, secret := range []string{
		"tok_SUPERSECRET_TOKEN_VALUE",
		"mxlrc_secretkey_one",
		"mxlrc_secretkey_two",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("Config view leaked secret %q", secret)
		}
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Error("Config view missing [REDACTED] marker; expected redacted token/webhook keys")
	}
}

// TestConfigViewEmptyTokenNotSet confirms an unset token renders as "(not set)"
// rather than "[REDACTED]", so an operator can tell unset from configured.
func TestConfigViewEmptyTokenNotSet(t *testing.T) {
	mux := newUIServer(config.Config{}, "dev")

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /config status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "token = (not set)") {
		t.Errorf("empty token should render as (not set); body did not contain it")
	}
}

// TestActiveNavHighlight verifies each page marks exactly its own nav link with
// aria-current="page", which drives the active-item accent styling.
func TestActiveNavHighlight(t *testing.T) {
	mux := newUIServer(config.Config{}, "dev")

	cases := []struct {
		path       string
		activeHref string
	}{
		{"/config", "/config"},
		{"/reports", "/reports"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", tc.path, rec.Code)
			}
			body := rec.Body.String()
			wantActive := `href="` + tc.activeHref + `" class="mx-nav-link" aria-current="page"`
			if !strings.Contains(body, wantActive) {
				t.Errorf("page %s did not mark %s active; missing %q", tc.path, tc.activeHref, wantActive)
			}
			// The other link must not be active.
			other := "/reports"
			if tc.activeHref == "/reports" {
				other = "/config"
			}
			if strings.Contains(body, `href="`+other+`" class="mx-nav-link" aria-current="page"`) {
				t.Errorf("page %s wrongly marked %s active too", tc.path, other)
			}
		})
	}
}

// TestShellRendersLogoutControl verifies the shared sidebar shell renders a
// sign-out control that POSTs to /logout on every authed page (issue #204, lane
// 4 UAT). /logout is POST-only, so a GET anchor would be unreachable; the
// control must be a form submit.
func TestShellRendersLogoutControl(t *testing.T) {
	mux := newUIServer(config.Config{}, "dev")

	for _, path := range []string{"/config", "/reports"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
			}
			body := rec.Body.String()
			for _, want := range []string{`method="post"`, `action="/logout"`, "Sign out", `type="submit"`} {
				if !strings.Contains(body, want) {
					t.Errorf("%s: shell missing logout control fragment %q", path, want)
				}
			}
		})
	}
}

// TestRootRedirectsToConfig checks the bare root sends operators to the Config
// view (the v1 landing page).
func TestRootRedirectsToConfig(t *testing.T) {
	mux := newUIServer(config.Config{}, "dev")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET / status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/config" {
		t.Errorf("GET / Location = %q, want /config", loc)
	}
}

// TestReportsPlaceholder confirms the Reports page renders its placeholder and
// carries the version in the sidebar.
func TestReportsPlaceholder(t *testing.T) {
	mux := newUIServer(config.Config{}, "v1.2.3")

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reports status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Reports workspace coming soon") {
		t.Error("Reports page missing placeholder copy")
	}
	if !strings.Contains(body, "v1.2.3") {
		t.Error("sidebar did not render the version string")
	}
}

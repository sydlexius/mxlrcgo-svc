# Implementation: serve-mode web UI foundation (issue #210)

Status: implemented 2026-06-14 on `feat/210-web-ui-foundation`
Scope: the first of three lanes implementing the #186 design of record
(`docs/design/2026-06-10-186-reports-workspace.md`). This note records the
build decisions and the deliberate deviations from CodeRabbit's coding plan on
the issue, so reviewers can reconcile the implementation against both.

## What shipped

- **Toolchain (dev-time only, no runtime node, no CGO):** `templ` pinned via
  the `go.mod` tool directive; Tailwind via the standalone CLI. Make targets
  `templ`, `tailwind`, `ui`, `ui-check`. A CI job (`ui-check`) regenerates and
  `git diff --exit-code`s so committed assets can never go stale.
- **Design tokens:** `web/static/css/design-tokens.css`, ported from stillwater
  M55 and pared to v1 constraints: dark-only, exactly two font families (Inter,
  JetBrains Mono), self-hosted. Tokens are `--mx-`-prefixed.
- **Shell:** fixed left sidebar (`web/templates/layout.templ` + `sidebar.templ`)
  with a brand wordmark, a mono version line, and two nav items, Reports and
  Config, with an `aria-current` active highlight in the accent token.
- **Config view:** `web/templates/config.templ` renders the effective TOML with
  every secret redacted. Reports is a placeholder (`reports.templ`) pending #211.
- **Serving:** `internal/web` wraps the `go:embed`'d assets (`web/static`) and
  registers `GET /{$}` -> redirect `/config`, `GET /config`, `GET /reports`,
  `GET /static/`. Mounted on the existing serve listener via the new
  `server.WithWebUI(cfg, version)` option; `runServe` passes the effective
  config.

## Architecture

```text
web/templates/*.templ      -> generated *_templ.go (package templates, committed)
web/static/css/*.css        -> Tailwind output.css (committed) + design tokens
web/static/fonts/*.woff2     -> self-hosted Inter + JetBrains Mono (+ OFL texts)
web/static/embed.go          -> go:embed all:css all:fonts  (package static)
internal/web/static.go       -> StaticHandler() over the embed FS, immutable cache
internal/web/ui.go           -> UI{cfg, version}: page renderers + route table
internal/server/server.go    -> WithWebUI option mounts UI on the API mux
```

The embed directive lives in `web/static/` (not `internal/web/`) because
`go:embed` cannot reach up out of its own directory tree; this mirrors
stillwater's `web/static/embed.go`.

## Deviations from CodeRabbit's coding plan (and why)

1. **Reused `config.FormatConfigText` instead of a new `internal/web.RedactConfig`.**
   The repo already renders redacted TOML via a centralized `IsSensitiveConfigKey`
   allowlist shared with the logging layer. A second redactor would risk the two
   allowlists drifting apart; one source of truth is safer.
2. **Tailwind v4 (CSS-first) instead of a v3 `tailwind.config.js`.** The installed
   toolchain and stillwater are both v4 (`@import "tailwindcss"`), which has no JS
   config file; a v3 config would be silently ignored.
3. **Embed in `web/static/embed.go`, not `internal/web/embed.go`** (go:embed
   reach-up limitation; see Architecture).
4. **Config view renders `bannerCfg`, the effective config, not raw `cfg`.**
   Rendered-evidence UAT showed raw `cfg` misreports a CLI `--token` as
   "(not set)"; `bannerCfg` is the resolved snapshot the startup banner uses.
5. **Equivalent structural render tests rather than brittle golden files**, plus
   a live-handler redaction test (the strongest proof of the security property).

## Testing and evidence

- `internal/web` unit tests at 95% (redaction, active-nav, root redirect, static
  serving + cache, missing-asset 404); `internal/server` tests cover the
  `WithWebUI` mount and the no-UI default. 31 packages pass, lint clean.
- Rendered evidence (binding UI/UX rule) against the live binary at branch HEAD:
  computed styles match the pinned M55 dark tokens; both fonts load (exactly two
  families); `/config` shows `token = [REDACTED]` / `webhook_api_keys =
  [REDACTED]` with zero raw-secret leakage.
- Generated `*_templ.go` is excluded from patch coverage in `codecov.yml`
  (authored from `.templ` sources, behavior covered by tests).

## Enablement flag and auth gate

The web UI is **OFF by default** behind `server.web_ui_enabled = true` in the
TOML config. The zero-value (`false`) means omitting the key keeps the listener
serving only the JSON API, exactly as before #210.

**Do not enable this flag until #204 (auth/onboarding) ships.** Enabling it
before auth lands exposes an unauthenticated browser UI on the serve listener -
any client that can reach the port can read the effective configuration (including
the presence/absence of API tokens and webhook keys, though values are redacted).
The flag exists to let the UI be shipped to the binary now and turned on safely
once the auth layer is in place.

## Out of scope (deferred)

- Action Queue / Recent Activity components -> #211 Reports workspace (reference
  pinned in `web/templates/REFERENCE-211.md`).
- Adjustable background opacity / glass treatment / preference machinery ->
  excluded by the #186 design of record; noted as a possible future enhancement.
- Authenticated web access, onboarding UI, TLS -> #204.

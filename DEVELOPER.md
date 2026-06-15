# Developer notes

## Web UI assets (serve mode)

The serve-mode web UI (issue #210) is built from templ templates and Tailwind
CSS at **dev time**. The generated output is committed and embedded via
`go:embed`, so the shipped binary has **no node runtime dependency and no CGO**.

### Toolchain

- **templ** is pinned via the `go.mod` tool directive, so `go tool templ`
  needs no separate install.
- **Tailwind** uses the standalone CLI (a single, node-free binary). Install it
  from the [Tailwind releases](https://github.com/tailwindlabs/tailwindcss/releases)
  (or `brew install tailwindcss`). CI pins **v4.2.0**; match that locally to keep
  generated CSS byte-identical. Override the binary path with
  `make ui TAILWIND=/path/to/tailwindcss`.

### Regenerating after editing `web/`

Any change under `web/templates/*.templ` or `web/static/css/*.css` requires
regenerating the committed output:

```sh
make ui          # runs `templ generate` + Tailwind, writes *_templ.go + output.css
```

Then commit the regenerated files alongside your source change. CI runs
`make ui-check` (regenerate, then `git diff --exit-code`) and fails if the
committed assets are stale.

### Source layout

| Path                         | Purpose                                          |
| ---------------------------- | ------------------------------------------------ |
| `web/templates/*.templ`      | templ source for the shell, sidebar, and pages   |
| `web/templates/*_templ.go`   | generated Go (committed, CI-verified)            |
| `web/static/css/input.css`   | Tailwind entry + component classes               |
| `web/static/css/design-tokens.css` | M55 design tokens (dark-only, 2 fonts)     |
| `web/static/css/output.css`  | compiled CSS (committed, CI-verified)            |
| `web/static/fonts/`          | self-hosted Inter + JetBrains Mono (woff2 + OFL) |
| `web/static/embed.go`        | `go:embed` of css + fonts                        |
| `internal/web/`              | static handler + page renderers (`UI`)           |

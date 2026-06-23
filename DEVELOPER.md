# Developer notes

## Web UI assets (serve mode)

The serve-mode web UI (issue #210) is built from templ templates and Tailwind
CSS. The generated output (`web/templates/*_templ.go`, `web/static/css/output.css`)
is **generated on build, not committed** (issue #364): it is gitignored and
produced by `make ui` in every build path (the pre-push gate, Dockerfile,
GoReleaser, and CI). The compiled CSS is embedded via `go:embed`, so the shipped
binary has **no node runtime dependency and no CGO**.

> **After a fresh clone, run `make ui` before `go build`.** `web/static/embed.go`
> embeds `output.css` at compile time, so the generated assets must exist on disk
> first. `make build` / the gate / CI all run generation for you; a bare
> `go build ./...` on a clean checkout will fail until you have run `make ui`.

### Toolchain

- **templ** is pinned via the `go.mod` tool directive, so `go tool templ`
  needs no separate install.
- **Tailwind** uses the standalone CLI (a single, node-free binary). Install it
  from the [Tailwind releases](https://github.com/tailwindlabs/tailwindcss/releases)
  (or `brew install tailwindcss`). CI pins **v4.2.0**; match that locally to keep
  generated CSS byte-identical. Override the binary path with
  `make ui TAILWIND=/path/to/tailwindcss`.

### Regenerating after editing `web/`

The generated assets are gitignored, so there is nothing to commit after editing
`web/templates/*.templ` or `web/static/css/*.css`. Just regenerate before you
build or test locally:

```sh
make ui          # runs `templ generate` + Tailwind, writes *_templ.go + output.css
```

CI, the Dockerfile, and GoReleaser all run `make generate` (an alias of `make ui`)
before their build steps, so the assets are always produced fresh from source.
Because nothing generated is committed, a Dependabot bump of templ or Tailwind can
no longer leave a stale committed artifact behind.

### Source layout

| Path                         | Purpose                                          |
| ---------------------------- | ------------------------------------------------ |
| `web/templates/*.templ`      | templ source for the shell, sidebar, and pages   |
| `web/templates/*_templ.go`   | generated Go (generated on build, gitignored)    |
| `web/static/css/input.css`   | Tailwind entry + component classes               |
| `web/static/css/design-tokens.css` | M55 design tokens (dark-only, 2 fonts)     |
| `web/static/css/output.css`  | compiled CSS (generated on build, gitignored)    |
| `web/static/fonts/`          | self-hosted Inter + JetBrains Mono (woff2 + OFL) |
| `web/static/embed.go`        | `go:embed` of css + fonts                        |
| `internal/web/`              | static handler + page renderers (`UI`)           |

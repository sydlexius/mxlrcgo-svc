# Technology Stack

**Project:** mxlrcgo-svc (restructured MxLRC-Go)
**Researched:** 2026-04-10

## Recommended Stack

### Core Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.25+ (go.mod) | Language | Bumped to 1.25.0 (required by `golang.org/x/text v0.36.0`). Go 1.25+ is the project minimum. | HIGH |
| `github.com/alexflint/go-arg` | v1.6.1 | CLI argument parsing | Already in use (v1.4.3). Upgrade to v1.6.1 gains native `env` tag support with correct precedence (CLI flag > env var > default). This eliminates the need for a separate env var library for token config. No API breakage from v1.4.3 to v1.6.1. | HIGH |

### Config (.env file loading)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `github.com/joho/godotenv` | v1.5.1 | Load `.env` files | go-arg handles CLI flags and env vars natively, but `.env` file loading requires a separate library. godotenv is the de facto standard (10.3k stars, 51k importers on pkg.go.dev). In the implementation, `arg.MustParse()` runs first, then `godotenv.Load()` populates `os.Environ` from `.env` (does NOT override existing env vars). Token precedence is then resolved manually via explicit `os.Getenv()` checks. Library is declared feature-complete by maintainer; v1.5.1 is the latest stable release. | HIGH |

### Existing Dependencies (retain, upgrade)

| Technology | Current | Upgrade To | Purpose | Why Upgrade | Confidence |
|------------|---------|------------|---------|-------------|------------|
| `github.com/valyala/fastjson` | v1.6.3 | v1.6.10 | JSON parsing for Musixmatch API | Bug fixes since v1.6.3. Published Feb 2026. No API changes. | HIGH |
| `golang.org/x/text` | v0.3.8 | v0.36.0 | Unicode normalization (NFKC) for `slugify` | Major version gap. v0.3.8 is from 2022; v0.36.0 published Apr 2026. Security and Unicode version updates. | HIGH |
| `github.com/dhowden/tag` | v0.0.0-20220618 | v0.0.0-20240417 (latest) | Audio metadata reading | Latest pseudo-version from Apr 2024. Untagged module but actively maintained. Bug fixes for DSF, edge cases. | MEDIUM |

### Build/Dev Tooling (no changes needed)

| Technology | Version | Purpose | Notes |
|------------|---------|---------|-------|
| GoReleaser | existing | Cross-platform release builds | Update config for new binary name `mxlrcgo-svc` and new `cmd/` entry point path |
| golangci-lint | v2.11+ | Linter aggregator | No changes needed, already comprehensive |
| Make | existing | Build orchestration | Update targets for new `cmd/mxlrcgo-svc/` path |

## What NOT to Use

### CLI Framework: Do NOT switch to cobra/viper

| Alternative | Why Not |
|-------------|---------|
| `spf13/cobra` + `spf13/viper` | Massively over-engineered for a single-command CLI. Cobra is designed for multi-subcommand tools (kubectl, docker). This project has zero subcommands. go-arg's struct-tag approach is simpler, has less boilerplate, and already handles the exact precedence needed. Adding cobra would triple the dependency tree for no benefit. |
| `urfave/cli` | Same problem as cobra -- subcommand-oriented framework for a single-command tool. |
| `spf13/pflag` | Lower-level than go-arg with no env var support. Would require manual env var handling. |

### Config: Do NOT use viper or koanf

| Alternative | Why Not |
|-------------|---------|
| `spf13/viper` | Pulls in 15+ transitive dependencies. Designed for complex config hierarchies (YAML, TOML, JSON, remote config). This project needs exactly one config value (token) from three sources. godotenv + go-arg's env tags cover this completely. |
| `knadh/koanf` | Lighter than viper but still overkill. Adds a config framework when go-arg + godotenv already provide the exact behavior needed with zero framework. |
| Roll-your-own `.env` parser | Fragile. The `.env` format has edge cases (quotes, comments, multiline, variable substitution). godotenv handles all of these correctly and is battle-tested. |

### JSON: Do NOT switch to encoding/json or sonic

| Alternative | Why Not |
|-------------|---------|
| `encoding/json` | Would require defining Go structs for the deeply nested Musixmatch API response. The existing code navigates the JSON tree dynamically with fastjson, which is the right pattern for a third-party API where response structure may change. Also 10-15x slower. |
| `bytedance/sonic` | Requires CGO for SIMD optimizations. Project constraint is CGO_ENABLED=0. Without CGO, sonic falls back to encoding/json speed. |

## Config Token Precedence Implementation

The project requires: CLI flag > env var (`MUSIXMATCH_TOKEN`) > `.env` file.

**Implemented approach in `cmd/mxlrcgo-svc/main.go`:**

`arg.MustParse()` runs first (it exits on bad args, so `.env` is only needed for the token).
After parsing, `godotenv.Load()` populates `os.Environ` from `.env` (does NOT override existing env vars).
Token precedence is then resolved manually:

```go
// Parse CLI flags first (exits on usage errors before .env is needed)
var args Args
arg.MustParse(&args)

// Load .env into os.Environ after parsing (does NOT override existing env vars)
_ = godotenv.Load()

// Resolve token with explicit precedence: CLI flag > MUSIXMATCH_TOKEN env var > .env value
token := args.Token
if token == "" {
    token = os.Getenv("MUSIXMATCH_TOKEN")
}
if token == "" {
    slog.Error("no API token provided: use --token flag, MUSIXMATCH_TOKEN env var, or .env file")
    os.Exit(1)
}
```

The `Token` field in `Args` does NOT use an `env:` tag — precedence is enforced explicitly.
`godotenv.Load()` after `arg.MustParse()` works because: the token check happens after both calls,
and `godotenv.Load()` only populates env vars not already set (so a real `MUSIXMATCH_TOKEN` in
the shell environment is not overwritten by the `.env` file).

Precedence achieved:
1. `--token=xxx` on CLI — `args.Token` is set; first check wins
2. `MUSIXMATCH_TOKEN=xxx` in shell environment — `os.Getenv` returns it; second check wins
3. `MUSIXMATCH_TOKEN=xxx` in `.env` file — godotenv loaded it; `os.Getenv` returns it on second check
4. No value set — exits with clear error message

## Project Layout for Restructure

No library needed -- this is a Go convention, not a dependency:

```
cmd/mxlrcgo-svc/main.go          # Entry point, thin: loads .env, parses args, runs app
internal/app/app.go               # App struct owns state (replaces global vars)
internal/models/models.go         # Types: Track, Song, Lyrics, etc. (from structs.go)
internal/musixmatch/client.go     # Musixmatch API client + Fetcher interface
internal/lyrics/writer.go         # LRC file writing (from lyrics.go)
internal/scanner/scanner.go       # Directory scanning, input parsing (from utils.go)
```

The `internal/` directory is enforced by the Go compiler -- packages under `internal/` cannot be imported by external modules. This is a language feature, not a convention.

## Dependency Upgrade Plan

```bash
# After module rename to sydlexius/mxlrcgo-svc:

# Upgrade existing dependencies
go get github.com/alexflint/go-arg@v1.6.1
go get github.com/valyala/fastjson@v1.6.10
go get golang.org/x/text@latest
go get github.com/dhowden/tag@latest

# Add new dependency
go get github.com/joho/godotenv@v1.5.1

# Tidy
go mod tidy
```

## Version Verification Sources

| Package | Source | Verified |
|---------|--------|----------|
| go-arg v1.6.1 | pkg.go.dev/github.com/alexflint/go-arg (Published: Nov 25, 2025) | HIGH |
| godotenv v1.5.1 | pkg.go.dev/github.com/joho/godotenv (Published: Feb 5, 2023) | HIGH |
| fastjson v1.6.10 | pkg.go.dev/github.com/valyala/fastjson (Published: Feb 21, 2026) | HIGH |
| golang.org/x/text v0.36.0 | pkg.go.dev/golang.org/x/text (Published: Apr 9, 2026) | HIGH |
| dhowden/tag latest | pkg.go.dev/github.com/dhowden/tag (Published: Apr 17, 2024) | HIGH |
| Go 1.26.2 | go.dev/dl (current stable) | HIGH |
| go-arg env tag support | Context7 /alexflint/go-arg README + pkg.go.dev examples | HIGH |
| godotenv feature-complete | github.com/joho/godotenv README ("declared feature complete") | HIGH |

---

*Stack research: 2026-04-10*

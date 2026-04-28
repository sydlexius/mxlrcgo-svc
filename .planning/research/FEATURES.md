# Feature Landscape

**Domain:** Go CLI project restructuring (flat main package to idiomatic Go layout)
**Researched:** 2026-04-10

## Table Stakes

Features users (developers maintaining this codebase) expect. Missing = restructuring is incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `cmd/mxlrcgo-svc/main.go` entry point | Official Go docs: commands go in `cmd/` subdirectories. Required for `go install` to produce correctly-named binary. Separates entry point from logic. | Low | Thin main: parse args, wire dependencies, call `app.Run()`. No business logic in main. |
| `internal/` package hierarchy | Go compiler enforces import boundaries. Prevents accidental external coupling. Official Go docs recommend `internal/` as default for private packages. | Medium | Four packages: `models`, `musixmatch`, `lyrics`, `scanner` (per PROJECT.md target layout). |
| Eliminate global mutable state | Global `inputs`/`failed` vars in main.go are the documented known issue. Blocks testability, prevents concurrent use, creates hidden coupling. | Medium | Replace with an `App` struct that owns both queues. Pass as method receiver or function parameter. |
| Exported types from internal packages | Types currently unexported in `main` package (e.g., `Track`, `Song`, `InputsQueue`). Moving to `internal/` packages means they must be exported (`track` becomes `models.Track`). | Low | Rename all types and methods to start with uppercase. Update all references. Mechanical refactor. |
| `Fetcher` interface for Musixmatch client | Enables test mocking without hitting real API. Standard Go pattern -- accept interfaces, return structs. PROJECT.md lists this as an active requirement. | Low | `type Fetcher interface { FindLyrics(Track) (Song, error) }`. Musixmatch client implements it. |
| Token externalization (CLI flag > env var > .env) | Hardcoded token in main.go is a security problem (committed to git). `go-arg` already supports `env` struct tags with correct precedence: CLI flag > env var > default. | Low | Add `arg:"env:MUSIXMATCH_TOKEN"` tag to the `Token` field in `Args`. For `.env` support, use `joho/godotenv` or manual load before `arg.MustParse`. |
| Proper error returns (not bool) | `writeLRC()` returns `bool` instead of `error`. Anti-pattern -- loses error context. Standard Go: return `error` from functions that can fail. | Low | Change `writeLRC` signature to return `error`. Callers already handle the failure case. |
| Updated Makefile / CI / GoReleaser | Build tooling references `mxlrc-go` binary name and flat package. Must update for new `cmd/` path and binary name `mxlrcgo-svc`. | Low | Mechanical: update `go build ./cmd/mxlrcgo-svc`, `.goreleaser.yml` main path, CI workflow paths. |
| Module path rename | `go.mod` says `github.com/fashni/mxlrc-go`. Fork identity requires `sydlexius/mxlrcgo-svc`. All import paths change. | Low | `go mod edit -module github.com/sydlexius/mxlrcgo-svc`, then fix all import statements. |
| Behavior preservation | All existing CLI flags and input modes (CLI pairs, text file, directory) must work identically. This is a restructuring, not a rewrite. | Medium | Verify with existing `utils_test.go` tests. Manual smoke testing of all three input modes after restructuring. |

## Differentiators

Features that improve quality beyond "restructuring complete." Not required for M0 but valuable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| `App` struct with `Run() error` method | Beyond just eliminating globals: centralizes orchestration, makes the processing loop testable in isolation, establishes the pattern for future features. | Medium | `App` holds config, fetcher, input queue, failed queue, output writer. `main()` constructs it, calls `Run()`. |
| Replace `isInArray` with Go generics | Current `isInArray` uses `reflect` (slow, unsafe). Go 1.22+ supports generics. `slices.Contains` from stdlib does this. | Low | Single function replacement. Removes `reflect` import from utils.go. |
| Compile regex once (package-level `var`) | `slugify()` compiles regex on every call. Should be package-level `var` or `sync.Once`. | Low | Move `regexp.MustCompile` to package-level vars in the lyrics/writer package. |
| Structured logging (slog) | Currently uses `log` stdlib. Go 1.21+ has `log/slog` in stdlib. Structured logging with levels, no external dependency. | Medium | Not required for M0 but improves observability. Can be deferred. |
| Reusable HTTP client | `musixmatch.go` creates a new `http.Client` per request. Should create once and reuse (connection pooling, timeout consistency). | Low | Store `*http.Client` as field on `Musixmatch` struct, initialized in constructor. |
| Consistent error wrapping | Some errors use `fmt.Errorf("...: %w", err)`, others use `errors.New()` or bare returns. Consistent wrapping with `%w` enables `errors.Is`/`errors.As` usage. | Low | Standardize during the move to new packages. |
| Context propagation | `findLyrics` uses `context.Background()`. Passing context from caller enables cancellation, timeout control, and graceful shutdown integration. | Low | Add `ctx context.Context` as first parameter to `FindLyrics`. Wire from `App.Run()` using signal-based context. |
| Constructor functions (`New...`) | Go convention: packages expose `NewClient()`, `NewWriter()` etc. Makes initialization explicit, validates required fields, sets defaults. | Low | Each internal package gets a constructor: `musixmatch.NewClient(token, httpClient)`, `lyrics.NewWriter()`, etc. |

## Anti-Features

Features to explicitly NOT build during this restructuring.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Switch to cobra/viper | `go-arg` already handles CLI parsing with env var support. Cobra is for complex multi-subcommand CLIs. This tool has one command. Adding cobra triples dependency weight for zero benefit. | Keep `go-arg`. Add `env` struct tags for token. |
| `pkg/` directory | Official Go docs: "`pkg/` is useful for larger projects where the root directory becomes cluttered." This is a small single-binary CLI. `internal/` is sufficient and compiler-enforced. | Use `internal/` only. No public API packages. |
| Config file support (YAML/TOML) | The tool has ~7 CLI flags. A config file is over-engineering. Token externalization via env var is sufficient. `.env` file support is the maximum needed. | CLI flags + env vars + `.env` file via `go-arg` env tags + optional godotenv. |
| `internal/app/` nesting | The project-layout reference shows `internal/app/myapp/` but this is for projects with multiple apps. MxLRC-Go has one binary. Extra nesting adds navigation cost for zero benefit. | Flat `internal/` packages: `internal/models/`, `internal/musixmatch/`, etc. |
| Dependency injection framework | Wire, dig, fx are for large service graphs. This CLI has 4 packages. Manual construction in `main()` is clearer and more debuggable. | Manual wiring in `cmd/mxlrcgo-svc/main.go`. |
| New test coverage | PROJECT.md explicitly defers this: "Move existing tests only, no new coverage." Test coverage is a separate milestone concern. Adding tests now expands scope. | Move `utils_test.go` to appropriate package. Fix import paths. Do not write new tests. |
| Concurrency / goroutine pool | The sequential processing loop with cooldown is intentional (rate limiting). Adding concurrency changes behavior and complicates the restructuring. | Keep sequential processing. Concurrency is a future feature. |
| Plugin architecture | The tool does one thing (fetch lyrics). An extension system is over-engineering. | Keep monolithic binary. |
| Separate `go.mod` per package (workspace) | Go workspaces are for multi-module repos. This is a single module with internal packages. | Single `go.mod` at root. |

## Feature Dependencies

```
Module path rename ──> All other features (import paths change first)
    |
    v
internal/ package hierarchy ──> Exported types (types must be exported when moved to packages)
    |
    v
Eliminate global state ──> App struct (globals replaced by struct fields)
    |                         |
    v                         v
Fetcher interface ──> Token externalization (interface needs token config)
    |
    v
Updated Makefile/CI/GoReleaser (must build from new paths)
    |
    v
Behavior preservation (verified last, after all moves complete)
```

Key dependency chain:
1. Module rename must happen first (all import paths depend on it)
2. Package creation and type export are tightly coupled (move + export in same step)
3. Global state elimination and App struct are tightly coupled
4. Build tooling updates are independent of code changes but must happen before CI passes
5. Behavior verification is the final gate

## MVP Recommendation

**All table stakes are required.** This is a restructuring -- partial completion leaves the codebase in a worse state than before (half-moved files, broken imports).

Prioritize in dependency order:
1. Module path rename (unblocks everything)
2. Create `internal/` packages + export types (bulk of the work)
3. Eliminate globals + create App struct (main architectural improvement)
4. Fetcher interface (enables future testability)
5. Token externalization via `go-arg` env tags (security fix)
6. Error return cleanup (`writeLRC` bool to error)
7. Update build tooling (Makefile, CI, GoReleaser)
8. Behavior verification (smoke test all three input modes)

**Defer all differentiators to post-M0** except:
- Constructor functions (`New...`) -- do these during package creation, costs nothing extra
- Compile regex once -- trivial to fix during the move, prevents future performance issues
- Replace `isInArray` with `slices.Contains` -- one-line fix, removes reflect dependency

These three differentiators have near-zero marginal cost when done alongside the restructuring, and paying technical debt later costs more.

## Sources

- Go official module layout guide: https://go.dev/doc/modules/layout (HIGH confidence)
- golang-standards/project-layout reference: Context7, benchmark score 91.6 (MEDIUM confidence -- community reference, not official)
- go-arg env var support: Context7 /alexflint/go-arg, HIGH reputation (HIGH confidence)
- Cobra CLI framework: Context7 /spf13/cobra (HIGH confidence -- consulted to confirm it's NOT needed for this project)
- Existing codebase: main.go, structs.go, musixmatch.go, lyrics.go, utils.go (direct inspection)
- PROJECT.md and ARCHITECTURE.md: project requirements and current state (direct inspection)

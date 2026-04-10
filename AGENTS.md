<!-- GSD:project-start source:PROJECT.md -->
## Project

**MxLRC-Go (sydlexius/mxlrcsvc-go)**

A Go CLI tool that fetches synced lyrics from the Musixmatch API and saves them as `.lrc` files. This is a fork of fashni/mxlrc-go being restructured into a maintainable Go project layout under a new module path, with global state eliminated and the hardcoded API token externalized.

**Core Value:** The tool fetches synced lyrics reliably and writes correct `.lrc` files. Everything else (project structure, config handling, CI) exists to support that.

### Constraints

- **Binary name**: `mxlrcsvc-go` (matches new module name)
- **No CGO**: Must remain CGO_ENABLED=0 for cross-compilation
- **Go 1.22+**: Minimum Go version from existing go.mod
- **Behavior preservation**: All existing CLI flags and behaviors must work identically after restructuring
- **Token precedence**: CLI flag > environment variable (`MUSIXMATCH_TOKEN`) > `.env` file
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.22 (minimum, per `go.mod`) - All application code
- Bash - Pre-commit hooks (`.githooks/pre-commit`), Makefile targets
- YAML - CI/CD workflows (`.github/workflows/`), configuration files
## Runtime
- Go (compiled binary, no runtime dependency)
- CGO disabled for release builds (`CGO_ENABLED=0` in `.goreleaser.yml`)
- Go Modules (`go.mod` / `go.sum`)
- Lockfile: `go.sum` present
## Frameworks
- None - Pure Go standard library for HTTP, I/O, and CLI orchestration
- `github.com/alexflint/go-arg` v1.4.3 - CLI argument parsing via struct tags (`main.go`, `structs.go`)
- Go standard `testing` package - No third-party test framework
- Make (`Makefile`) - Build orchestration (build, test, lint, fmt, clean)
- GoReleaser (`.goreleaser.yml`) - Cross-platform release builds
- golangci-lint v2.11.4 (`.golangci.yml`) - Linter aggregator with 12 enabled linters
## Key Dependencies
- `github.com/alexflint/go-arg` v1.4.3 - CLI argument parsing. Defines the entire user interface via struct tags on `Args` in `structs.go`
- `github.com/dhowden/tag` v0.0.0-20220618230019 - Audio file metadata reading (ID3, MP4, FLAC, OGG, DSF). Used in `utils.go` for directory-scan mode
- `github.com/valyala/fastjson` v1.6.3 - High-performance JSON parsing for Musixmatch API responses. Used in `musixmatch.go` to navigate deeply nested JSON
- `golang.org/x/text` v0.3.8 - Unicode normalization (NFKC) for filename sanitization in `slugify()` (`utils.go`)
- `github.com/alexflint/go-scalar` v1.1.0 - Transitive dependency of go-arg
## Configuration
- No `.env` files - Configuration is entirely via CLI flags
- Musixmatch API token: passed via `-t/--token` flag or falls back to a hardcoded default token in `main.go`
- No environment variables required for operation
- `Song` (positional, required) - Song info as `artist,title` pairs, a `.txt` file path, or a directory path
- `-o/--outdir` (default: `lyrics`) - Output directory for `.lrc` files
- `-c/--cooldown` (default: `15`) - Cooldown between API requests in seconds
- `-d/--depth` (default: `100`) - Max recursion depth for directory scanning
- `-u/--update` - Overwrite existing `.lrc` files in directory mode
- `--bfs` - Use BFS instead of DFS for directory traversal
- `-t/--token` - Musixmatch API token
- `go.mod` - Module definition and Go version constraint
- `.goreleaser.yml` - Cross-compilation targets (linux/darwin/windows, amd64/arm64, excluding windows/arm64)
- `.golangci.yml` - Linter configuration (v2 format)
- `.editorconfig` - Editor formatting (tabs for Go, 2-space for YAML/JSON/MD)
- `.gitattributes` - Line ending normalization (LF everywhere)
- `.typos.toml` - Spell-checker config (excludes `go.sum`)
- `.pre-commit-config.yaml` - Pre-commit framework hooks: trailing-whitespace, end-of-file-fixer, check-yaml, check-added-large-files (500KB), check-merge-conflict, typos, gitleaks, golangci-lint, gofmt, conventional-pre-commit
- `.githooks/pre-commit` - Manual pre-commit hook: typos, gofmt, go build, golangci-lint, govulncheck
## Platform Requirements
- Go 1.22+
- golangci-lint v2.11+ (for linting)
- typos-cli (for spell checking)
- govulncheck (for vulnerability scanning)
- goimports (optional, for import formatting)
- pre-commit (optional, for `.pre-commit-config.yaml` hooks)
- Standalone static binary (CGO_ENABLED=0)
- No runtime dependencies
- Supported platforms: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
## CI/CD Pipeline
- `ci.yml` - Lint + Test + Build matrix (linux/darwin/windows x amd64/arm64). Uses `dorny/paths-filter` to skip on non-code changes. Build requires lint+test to pass first.
- `release.yml` - GoReleaser on `v*.*.*` tags. Produces cross-platform archives with conventional-commit changelogs.
- `codeql.yml` - GitHub CodeQL security analysis for Go. Runs on push/PR to main and weekly (Monday 04:17 UTC).
- `dependabot-auto-approve.yml` - Auto-approves Dependabot PRs for patch/minor updates.
- `dependabot-merge.yml` - Auto-merges approved Dependabot PRs after CI passes (squash merge, delete branch).
- Weekly updates (Monday) for `gomod` and `github-actions` ecosystems
- Conventional commit prefixes (`chore(deps)` for Go, `ci` for Actions)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- Flat, single-word lowercase names: `main.go`, `lyrics.go`, `musixmatch.go`, `structs.go`, `utils.go`
- Test files use Go's standard `_test.go` suffix: `utils_test.go`
- No subdirectories -- all `.go` files live in the project root
- camelCase for unexported (all functions are unexported since single `main` package): `writeLRC()`, `slugify()`, `parseInput()`, `getSongDir()`
- Method receivers are short abbreviations: `mx` for `Musixmatch`, `q` for `InputsQueue`
- Mixed snake_case appears in some parameter names (legacy from Python port): `song_list`, `save_path`, `text_fn`, `lrc_file` -- avoid in new code, use camelCase
- Short, abbreviated names preferred: `fn` (filename), `fp` (filepath), `mx` (Musixmatch), `cnt` (count), `cur` (current), `res` (response)
- Loop variables are single-letter: `i`, `f`, `m`, `v`
- Error variables are always `err` (shadowed freely within nested scopes)
- PascalCase for all exported types: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`
- Struct fields use PascalCase with JSON tags: `TrackName string \`json:"track_name,omitempty"\``
- All types defined in `structs.go`
- SCREAMING_CASE for package-level URL constant: `const URL = "https://..."`
## Code Style
- `gofmt` is the canonical formatter (enforced by pre-commit hook and CI)
- `goimports` for import grouping (run via `make fmt`)
- Tab indentation for `.go` files, 2-space indentation for config files (`.editorconfig`)
- `golangci-lint` v2 with `.golangci.yml` config
- Enabled linters: `errcheck`, `govet`, `staticcheck`, `unused`, `bodyclose`, `gosec`, `noctx`, `unconvert`, `unparam`, `wastedassign`, `misspell`, `revive`
- Test files excluded from: `gosec`, `errcheck`, `noctx`
- US English locale enforced via `misspell`
- `revive` configured with `disableStutteringCheck` and `unexported-return` warnings
- Always include a justification comment after `//nolint:linter`:
- Used sparingly -- only for `gosec` false positives on file operations with user-provided paths
- LF enforced everywhere via `.gitattributes` (`* text=auto eol=lf`)
## Import Organization
- None used. All imports are direct package paths.
## Error Handling
- Explicitly discard close error with `_ =` (satisfies `errcheck`)
## Logging
- `log.Printf("verb noun: %s", value)` for informational progress messages
- `log.Println(err)` for non-fatal error reporting
- `log.Fatal(err)` for unrecoverable startup/file errors
- `log.Fatalf("message: %v", err)` when adding context to fatal errors
- `fmt.Printf()` used for user-facing output (timer countdown, result counts)
- `log.*` for operational messages (searching, saving, skipping)
- `fmt.*` for user-facing interactive output (progress counters, prompts)
## Comments
- `nolint` directives always get a `// reason` suffix
- Inline comments for non-obvious constants: `// 2 MiB`, `// forbidden chars in filename`
- No function-level doc comments on unexported functions (acceptable since single `main` package)
- One instance exists: `// log.Println(baseURL.String())` in `musixmatch.go:46` -- avoid adding more
## Function Design
- Functions are generally short (10-40 lines)
- Longest function: `getSongDir()` at ~57 lines, `findLyrics()` at ~117 lines
- Pass pointer to `InputsQueue` when mutating: `func parseInput(args Args, in *InputsQueue) string`
- Value receiver on `Musixmatch` (no mutation needed): `func (mx Musixmatch) findLyrics(track Track)`
- Pointer receiver on `InputsQueue` (mutates state): `func (q *InputsQueue) push(i Inputs)`
- Named return for `success bool` pattern in `writeLRC()` (enables deferred close error tracking)
- `(value, error)` tuple for fallible operations: `findLyrics(track Track) (Song, error)`
- Simple `bool` for write operations: `writeSyncedLRC()`, `writeUnsyncedLRC()`
- Pointer-or-nil for validation: `assertInput(song string) *Track`
## Module Design
- No exported functions or variables (single `main` package, CLI binary)
- All types are exported (PascalCase) for JSON deserialization via struct tags
- Two package-level mutable vars: `var inputs InputsQueue` and `var failed InputsQueue` in `main.go`
- Not applicable (flat structure, single package)
- `structs.go` -- all type definitions and `InputsQueue` methods
- `utils.go` -- input parsing, directory scanning, string utilities
- `lyrics.go` -- LRC file writing (synced, unsynced, instrumental)
- `musixmatch.go` -- API client and response parsing
- `main.go` -- CLI entry point, orchestration loop, signal handling
## Commit Conventions
- Prefixes: `feat:`, `fix:`, `docs:`, `style:`, `refactor:`, `perf:`, `test:`, `build:`, `ci:`, `chore:`, `revert:`
- No emoji in commits, code, comments, or documentation
## Spell Checking
- `go.sum` is excluded from spell checking
- Runs as both pre-commit hook check and pre-commit framework hook
## Secret Scanning
- Scans for accidentally committed secrets/credentials
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- All source files live in a single `main` package with no subdirectories
- Procedural architecture organized by responsibility (API, lyrics, utils, types)
- No dependency injection -- structs are instantiated directly in `main()`
- Global mutable state via package-level `InputsQueue` variables
- Sequential processing loop with cooldown timer between API calls
## Layers
- Purpose: Parse CLI arguments, orchestrate the main processing loop, handle signals
- Location: `main.go`
- Contains: `main()`, `timer()`, `failedHandler()`, `closeHandler()`
- Depends on: `go-arg` for argument parsing, `InputsQueue` (structs.go), `parseInput()` (utils.go), `Musixmatch.findLyrics()` (musixmatch.go), `writeLRC()` (lyrics.go)
- Used by: Nothing (top-level entry point)
- Purpose: Communicate with the Musixmatch desktop API, parse responses, return structured song data
- Location: `musixmatch.go`
- Contains: `Musixmatch` struct, `findLyrics(Track) (Song, error)` method
- Depends on: `net/http`, `fastjson`, `encoding/json`, types from `structs.go`
- Used by: `main.go` processing loop
- Purpose: Format song data into LRC format and write to disk
- Location: `lyrics.go`
- Contains: `writeLRC()`, `writeSyncedLRC()`, `writeUnsyncedLRC()`, `writeInstrumentalLRC()`
- Depends on: `Song` type from `structs.go`, `slugify()` from `utils.go`
- Used by: `main.go` processing loop
- Purpose: Input parsing, directory scanning, filename sanitization, generic helpers
- Location: `utils.go`
- Contains: `parseInput()`, `getSongMulti()`, `getSongText()`, `getSongDir()`, `slugify()`, `isInArray()`, `assertInput()`, `supportedFType()`
- Depends on: `dhowden/tag` for audio metadata, `golang.org/x/text/unicode/norm` for NFKC normalization, types from `structs.go`
- Used by: `main.go` (via `parseInput()`), `lyrics.go` (via `slugify()`)
- Purpose: Define all data structures used across the application
- Location: `structs.go`
- Contains: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`
- Depends on: Nothing (pure data definitions)
- Used by: Every other layer
## Data Flow
- Two package-level global variables: `inputs InputsQueue` and `failed InputsQueue` (`main.go:15-16`)
- `InputsQueue` is a simple FIFO queue backed by a slice (`structs.go:55-79`)
- No concurrency primitives -- single-threaded sequential processing
- State is not persisted between runs (except the `_failed.txt` retry file)
## Key Abstractions
- Purpose: FIFO queue holding items to process (or that failed)
- Examples: `structs.go:55-79`
- Pattern: Simple slice-backed queue with `next()`, `pop()`, `push()`, `len()`, `empty()` methods
- Note: `next()` peeks without removing; `pop()` removes and returns the front item
- Purpose: API client for the Musixmatch desktop endpoint
- Examples: `musixmatch.go:20-22`
- Pattern: Struct with a single `Token` field; stateless except for the token. New `http.Client` created per request.
- Purpose: Represent the full result from a lyrics lookup
- Examples: `structs.go:1-37`
- Pattern: Nested value types. `Song` composes `Track`, `Lyrics`, and `Synced`. JSON tags map to Musixmatch API response fields.
- Purpose: Represent a single work item in the processing queue
- Examples: `structs.go:39-43`
- Pattern: Bundles a `Track` (what to search for) with `Outdir` and `Filename` (where to write output)
## Entry Points
- Location: `main.go:18` (`func main()`)
- Triggers: Direct CLI invocation (`mxlrc-go [args]`)
- Responsibilities: Parses args, populates input queue, creates `Musixmatch` client, runs processing loop, handles failures and graceful shutdown
- Location: `Makefile:7-8` and `.goreleaser.yml:3-4`
- Triggers: `make build` or GoReleaser on tag push
- Responsibilities: Compiles `main` package into `mxlrc-go` binary
## Error Handling
- `findLyrics()` returns `(Song, error)` -- caller logs the error and pushes the item to the `failed` queue (`main.go:56-58`)
- `writeLRC()` returns `bool` success flag instead of `error` -- internally logs errors before returning `false` (`lyrics.go:12`)
- File I/O errors during input parsing (`getSongText`, `getSongDir`) call `log.Fatal()` -- immediate termination (`utils.go:48`, `utils.go:67`)
- HTTP status codes mapped to specific error messages: 401 = "too many requests", 404 = "no results found" (`musixmatch.go:66-74`)
- Response body size is capped at 2 MiB to prevent memory exhaustion (`musixmatch.go:77-84`)
- `defer` pattern used for closing `http.Response.Body` and output files (`musixmatch.go:63`, `lyrics.go:36-41`)
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, or `.github/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->

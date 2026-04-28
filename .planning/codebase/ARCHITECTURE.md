# Architecture

**Analysis Date:** 2026-04-10 (updated post-M0 restructure)

## Pattern Overview

**Overall:** Multi-package CLI application (`cmd/` entry point + `internal/` domain packages)

**Key Characteristics:**
- Entry point lives in `cmd/mxlrcgo-svc/main.go`; all domain logic lives in `internal/`
- Procedural architecture organized by responsibility across discrete packages
- Dependency injection via interfaces (`musixmatch.Fetcher`, `lyrics.Writer`)
- `App` struct owns all processing state — no package-level globals
- Sequential processing loop with cooldown timer between API calls

## Layers

**CLI / Entry Point Layer:**
- Purpose: Parse CLI arguments, load token, wire dependencies, start App
- Location: `cmd/mxlrcgo-svc/main.go`
- Contains: `main()`, signal context setup, token resolution
- Depends on: `go-arg`, `godotenv`, `internal/app`, `internal/musixmatch`, `internal/lyrics`, `internal/scanner`
- Used by: Nothing (top-level entry point)

**App Orchestration Layer:**
- Purpose: Own processing state, run the fetch loop, handle failures
- Location: `internal/app/app.go`, `internal/app/queue.go`
- Contains: `App` struct, `App.Run(ctx)`, `App.timer()`, `App.handleFailed()`, `InputsQueue`
- Depends on: `musixmatch.Fetcher`, `lyrics.Writer`, `internal/models`
- Used by: `cmd/mxlrcgo-svc/main.go`

**API Client Layer:**
- Purpose: Communicate with the Musixmatch desktop API, parse responses, return structured song data
- Location: `internal/musixmatch/client.go`, `internal/musixmatch/fetcher.go`
- Contains: `Client` struct, `FindLyrics(ctx, Track) (Song, error)` method, `Fetcher` interface
- Depends on: `net/http`, `fastjson`, `encoding/json`, `internal/models`
- Used by: `internal/app` via `Fetcher` interface

**Output / Lyrics Writer Layer:**
- Purpose: Format song data into LRC format and write to disk
- Location: `internal/lyrics/writer.go`, `internal/lyrics/slugify.go`
- Contains: `LRCWriter`, `WriteLRC()`, `writeSyncedLRC()`, `writeUnsyncedLRC()`, `writeInstrumentalLRC()`, `Slugify()`
- Depends on: `internal/models`, `golang.org/x/text/unicode/norm`
- Used by: `internal/app` via `Writer` interface

**Scanner / Input Layer:**
- Purpose: Input parsing, directory scanning, filename sanitization
- Location: `internal/scanner/scanner.go`
- Contains: `Scanner`, `ParseInput()`, `GetSongMulti()`, `GetSongText()`, `GetSongDir()`, `AssertInput()`
- Depends on: `dhowden/tag`, `internal/app` (for `InputsQueue`), `internal/models`
- Used by: `cmd/mxlrcgo-svc/main.go`

**Models Layer:**
- Purpose: Define all data structures used across the application
- Location: `internal/models/models.go`
- Contains: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Inputs`
- Depends on: Nothing (pure data definitions)
- Used by: Every other layer

## Data Flow

**Main Processing Pipeline:**

1. `main()` loads `.env` via `godotenv`, parses CLI args, resolves token (flag > env var > .env), sets up signal context
2. `scanner.ParseInput()` detects input mode (CLI/text-file/directory) and populates an `InputsQueue`
3. For directory mode, `GetSongDir()` recursively scans audio files and reads metadata via `dhowden/tag`
4. `App.Run(ctx)` iterates the inputs queue: for each `Inputs` item, calls `fetcher.FindLyrics(ctx, cur.Track)`
5. `FindLyrics()` builds HTTP request (with run context for cancellation), parses nested JSON via `fastjson`, unmarshals into `Song` struct
6. `writer.WriteLRC()` checks content eligibility first, then dispatches to `writeSyncedLRC`, `writeUnsyncedLRC`, or `writeInstrumentalLRC`
7. LRC file is written to disk with metadata tags and formatted lyrics lines
8. Failed items are collected in `App.failed`; on completion or SIGTERM, `handleFailed()` writes `_failed.txt` for retry

**Input Detection Flow (ParseInput):**

1. If exactly one positional arg AND it's an existing file path: treat as text file
2. If exactly one positional arg AND it's an existing directory: treat as directory scan
3. Otherwise: treat all positional args as `artist,title` pairs

**State Management:**
- `App` struct owns `inputs *InputsQueue` and `failed *InputsQueue` — no package-level globals
- `InputsQueue` is a slice-backed FIFO queue with safe `Next()`/`Pop()` returning `(value, error)` (`internal/app/queue.go`)
- No concurrency primitives — single-threaded sequential processing
- State is not persisted between runs (except the `_failed.txt` retry file)

## Key Abstractions

**InputsQueue:**
- Purpose: FIFO queue holding items to process (or that failed)
- Location: `internal/app/queue.go`
- Pattern: Slice-backed queue with `Next()`, `Pop()` (both return `(models.Inputs, error)`), `Push()`, `Len()`, `Empty()`

**musixmatch.Fetcher (interface):**
- Purpose: Abstracts lyrics lookup; enables testing without real HTTP calls
- Location: `internal/musixmatch/fetcher.go`
- Pattern: `FindLyrics(ctx context.Context, track models.Track) (models.Song, error)`

**lyrics.Writer (interface):**
- Purpose: Abstracts LRC file output; enables testing without real disk writes
- Location: `internal/lyrics/writer.go`
- Pattern: `WriteLRC(song models.Song, filename string, outdir string) error`

**Song / Track / Lyrics / Synced:**
- Purpose: Represent the full result from a lyrics lookup
- Location: `internal/models/models.go`
- Pattern: Nested value types. `Song` composes `Track`, `Lyrics`, and `Synced`. JSON tags map to Musixmatch API response fields.

**Inputs:**
- Purpose: Represent a single work item in the processing queue
- Location: `internal/models/models.go`
- Pattern: Bundles a `Track` (what to search for) with `Outdir` and `Filename` (where to write output)

## Entry Points

**CLI Entry Point:**
- Location: `cmd/mxlrcgo-svc/main.go` (`func main()`)
- Triggers: Direct CLI invocation (`mxlrcgo-svc [args]`)
- Responsibilities: Token resolution, wires fetcher/writer/scanner/app, starts `App.Run(ctx)`

**Build Entry Point:**
- Location: `Makefile` and `.goreleaser.yml`
- Triggers: `make build` or GoReleaser on tag push
- Responsibilities: Compiles `cmd/mxlrcgo-svc` into `mxlrcgo-svc` binary

## Error Handling

**Strategy:** Return `error` from functions, log and continue on non-fatal errors, `os.Exit(1)` on unrecoverable startup errors

**Patterns:**
- `FindLyrics()` returns `(Song, error)` — caller logs and pushes to `failed` queue
- `WriteLRC()` returns `error` — caller logs and pushes to `failed` queue
- `Next()`/`Pop()` return `(models.Inputs, error)` — safe against empty-queue panic
- HTTP status codes mapped to specific error messages: 401 = "too many requests", 404 = "no results found"
- Response body size is capped at 2 MiB to prevent memory exhaustion
- `defer` pattern used for closing `http.Response.Body` and output files

## Cross-Cutting Concerns

**Logging:** `log/slog` throughout. Structured key-value pairs. All log output goes to stderr.

**Validation:** Input validation in `AssertInput()` — checks that input string splits into exactly 2 comma-separated parts. File extension validation in `GetSongDir()`. No validation on API token format.

**Authentication:** Token resolved with precedence: `--token` CLI flag > `MUSIXMATCH_TOKEN` env var > `.env` file (loaded via `godotenv`). No hardcoded fallback — missing token is a fatal startup error.

**Graceful Shutdown:** Signal context (`context.WithCancel`) propagated through `App.Run(ctx)` and into `FindLyrics(ctx, ...)` so in-flight HTTP requests respect Ctrl+C / SIGTERM.

**Rate Limiting:** Simple cooldown timer between API calls, configurable via `--cooldown` flag (default 15 seconds). Implemented as a ticker-based countdown in `App.timer(ctx)` that respects context cancellation.

---

*Architecture analysis: 2026-04-10 | Updated post-M0: 2026-04-11*

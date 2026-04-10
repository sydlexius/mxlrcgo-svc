# Architecture

**Analysis Date:** 2026-04-10

## Pattern Overview

**Overall:** Single-package CLI application (flat `main` package, no internal packages)

**Key Characteristics:**
- All source files live in a single `main` package with no subdirectories
- Procedural architecture organized by responsibility (API, lyrics, utils, types)
- No dependency injection -- structs are instantiated directly in `main()`
- Global mutable state via package-level `InputsQueue` variables
- Sequential processing loop with cooldown timer between API calls

## Layers

**CLI / Entry Point Layer:**
- Purpose: Parse CLI arguments, orchestrate the main processing loop, handle signals
- Location: `main.go`
- Contains: `main()`, `timer()`, `failedHandler()`, `closeHandler()`
- Depends on: `go-arg` for argument parsing, `InputsQueue` (structs.go), `parseInput()` (utils.go), `Musixmatch.findLyrics()` (musixmatch.go), `writeLRC()` (lyrics.go)
- Used by: Nothing (top-level entry point)

**API Client Layer:**
- Purpose: Communicate with the Musixmatch desktop API, parse responses, return structured song data
- Location: `musixmatch.go`
- Contains: `Musixmatch` struct, `findLyrics(Track) (Song, error)` method
- Depends on: `net/http`, `fastjson`, `encoding/json`, types from `structs.go`
- Used by: `main.go` processing loop

**Output / Lyrics Writer Layer:**
- Purpose: Format song data into LRC format and write to disk
- Location: `lyrics.go`
- Contains: `writeLRC()`, `writeSyncedLRC()`, `writeUnsyncedLRC()`, `writeInstrumentalLRC()`
- Depends on: `Song` type from `structs.go`, `slugify()` from `utils.go`
- Used by: `main.go` processing loop

**Utilities Layer:**
- Purpose: Input parsing, directory scanning, filename sanitization, generic helpers
- Location: `utils.go`
- Contains: `parseInput()`, `getSongMulti()`, `getSongText()`, `getSongDir()`, `slugify()`, `isInArray()`, `assertInput()`, `supportedFType()`
- Depends on: `dhowden/tag` for audio metadata, `golang.org/x/text/unicode/norm` for NFKC normalization, types from `structs.go`
- Used by: `main.go` (via `parseInput()`), `lyrics.go` (via `slugify()`)

**Types Layer:**
- Purpose: Define all data structures used across the application
- Location: `structs.go`
- Contains: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`
- Depends on: Nothing (pure data definitions)
- Used by: Every other layer

## Data Flow

**Main Processing Pipeline:**

1. `main()` parses CLI args via `go-arg` into `Args` struct (`main.go:20`)
2. `parseInput()` detects input mode (CLI/text-file/directory) and populates the global `inputs` queue (`utils.go:122-139`)
3. For directory mode, `getSongDir()` recursively scans audio files and reads metadata via `dhowden/tag` (`utils.go:63-120`)
4. Main loop iterates `inputs` queue: for each `Inputs` item, calls `mx.findLyrics(cur.Track)` (`main.go:43-60`)
5. `findLyrics()` builds HTTP request to `apic-desktop.musixmatch.com`, parses nested JSON response via `fastjson`, unmarshals track/lyrics/subtitle data into `Song` struct (`musixmatch.go:24-141`)
6. `writeLRC()` dispatches to the appropriate writer (`writeSyncedLRC`, `writeUnsyncedLRC`, or `writeInstrumentalLRC`) based on available content (`lyrics.go:12-77`)
7. LRC file is written to disk with metadata tags (artist, title, album, length) and formatted lyrics lines
8. Failed items are collected in the global `failed` queue; on completion or SIGTERM, `failedHandler()` writes `_failed.txt` for retry (`main.go:77-115`)

**Input Detection Flow (parseInput):**

1. If exactly one positional arg AND it's an existing file path: treat as text file (`utils.go:124-128`)
2. If exactly one positional arg AND it's an existing directory: treat as directory scan (`utils.go:129-131`)
3. Otherwise: treat all positional args as `artist,title` pairs (`utils.go:137`)

**State Management:**
- Two package-level global variables: `inputs InputsQueue` and `failed InputsQueue` (`main.go:15-16`)
- `InputsQueue` is a simple FIFO queue backed by a slice (`structs.go:55-79`)
- No concurrency primitives -- single-threaded sequential processing
- State is not persisted between runs (except the `_failed.txt` retry file)

## Key Abstractions

**InputsQueue:**
- Purpose: FIFO queue holding items to process (or that failed)
- Examples: `structs.go:55-79`
- Pattern: Simple slice-backed queue with `next()`, `pop()`, `push()`, `len()`, `empty()` methods
- Note: `next()` peeks without removing; `pop()` removes and returns the front item

**Musixmatch:**
- Purpose: API client for the Musixmatch desktop endpoint
- Examples: `musixmatch.go:20-22`
- Pattern: Struct with a single `Token` field; stateless except for the token. New `http.Client` created per request.

**Song / Track / Lyrics / Synced:**
- Purpose: Represent the full result from a lyrics lookup
- Examples: `structs.go:1-37`
- Pattern: Nested value types. `Song` composes `Track`, `Lyrics`, and `Synced`. JSON tags map to Musixmatch API response fields.

**Inputs:**
- Purpose: Represent a single work item in the processing queue
- Examples: `structs.go:39-43`
- Pattern: Bundles a `Track` (what to search for) with `Outdir` and `Filename` (where to write output)

## Entry Points

**CLI Entry Point:**
- Location: `main.go:18` (`func main()`)
- Triggers: Direct CLI invocation (`mxlrc-go [args]`)
- Responsibilities: Parses args, populates input queue, creates `Musixmatch` client, runs processing loop, handles failures and graceful shutdown

**Build Entry Point:**
- Location: `Makefile:7-8` and `.goreleaser.yml:3-4`
- Triggers: `make build` or GoReleaser on tag push
- Responsibilities: Compiles `main` package into `mxlrc-go` binary

## Error Handling

**Strategy:** Return `error` from functions, log and continue on non-fatal errors, `log.Fatal` on unrecoverable errors

**Patterns:**
- `findLyrics()` returns `(Song, error)` -- caller logs the error and pushes the item to the `failed` queue (`main.go:56-58`)
- `writeLRC()` returns `bool` success flag instead of `error` -- internally logs errors before returning `false` (`lyrics.go:12`)
- File I/O errors during input parsing (`getSongText`, `getSongDir`) call `log.Fatal()` -- immediate termination (`utils.go:48`, `utils.go:67`)
- HTTP status codes mapped to specific error messages: 401 = "too many requests", 404 = "no results found" (`musixmatch.go:66-74`)
- Response body size is capped at 2 MiB to prevent memory exhaustion (`musixmatch.go:77-84`)
- `defer` pattern used for closing `http.Response.Body` and output files (`musixmatch.go:63`, `lyrics.go:36-41`)

## Cross-Cutting Concerns

**Logging:** Standard library `log` package throughout. No structured logging. All log output goes to stderr. Messages use `log.Printf` for informational, `log.Println` for simple messages, `log.Fatal` for unrecoverable errors.

**Validation:** Input validation happens in `assertInput()` (`utils.go:22-32`) -- checks that input string splits into exactly 2 comma-separated parts. File extension validation in `getSongDir()` via `isInArray(supportedFType(), ...)` (`utils.go:94`). No validation on API token format.

**Authentication:** Single Musixmatch API token passed via `--token` flag or hardcoded default (`main.go:36-38`). Token is sent as `usertoken` query parameter to the desktop API endpoint.

**Graceful Shutdown:** `closeHandler()` listens for SIGTERM/SIGINT and calls `failedHandler()` to save unprocessed items before exiting (`main.go:117-126`).

**Rate Limiting:** Simple cooldown timer between API calls, configurable via `--cooldown` flag (default 15 seconds). Implemented as a blocking countdown in `timer()` (`main.go:66-75`).

---

*Architecture analysis: 2026-04-10*

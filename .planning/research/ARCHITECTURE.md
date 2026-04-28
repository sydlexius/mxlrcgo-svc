# Architecture Patterns

**Domain:** Go CLI restructuring (flat main package to cmd/internal layout)
**Researched:** 2026-04-10
**Confidence:** HIGH (official Go documentation + established community conventions)

## Source: Official Go Module Layout

The Go team documents the recommended project layouts at go.dev/doc/modules/layout (verified via Context7, HIGH confidence). The relevant pattern for this project is **"Server project"** / **"Command with supporting packages"**:

> "Because a server is typically a self-contained binary, it is recommended to keep the Go packages implementing the server's logic within the internal directory to prevent them from being exported. Additionally, grouping all Go commands in a cmd directory helps manage the project."

> "For larger packages or commands, it is recommended to place supporting functionality into an internal directory. This practice prevents other modules from depending on code that is not intended for external use."

This applies directly to mxlrcgo-svc: a single binary with no exported API, where all packages exist to serve that binary.

## Recommended Architecture

### Target Layout

```
cmd/mxlrcgo-svc/
  main.go               Entry point: arg parsing, App construction, run

internal/
  app/
    app.go               App struct (owns state), Run() method, signal handling
  models/
    models.go            Track, Song, Lyrics, Synced, Lines, Time, Args, Inputs
    queue.go             InputsQueue (FIFO queue type)
  musixmatch/
    client.go            Musixmatch struct, Fetcher interface, findLyrics()
  lyrics/
    writer.go            writeLRC, writeSyncedLRC, writeUnsyncedLRC, writeInstrumentalLRC
    slugify.go           slugify() helper (used only by lyrics writer)
  scanner/
    scanner.go           parseInput, getSongDir, getSongText, getSongMulti
    scanner_test.go      Relocated tests from utils_test.go (slugify tests move to lyrics/)
```

### Why This Layout (Not Alternatives)

**Why `internal/app/` instead of inlining orchestration in `cmd/`:**
The current `main.go` has ~70 lines of orchestration logic (the processing loop, timer, failedHandler, closeHandler) plus global state. Putting this into `cmd/mxlrcgo-svc/main.go` would repeat the flat-file mistake. The `app` package owns the `App` struct that replaces global state, runs the processing loop, and handles signals. `main.go` becomes a thin 15-20 line file that parses args, constructs an `App`, and calls `app.Run()`.

**Why `internal/lyrics/slugify.go` instead of a shared `internal/util/`:**
`slugify()` is only called from `writeLRC()`. Keeping it in `lyrics/` avoids a "junk drawer" utils package. If another package later needs it, promote it then. Go convention: avoid generic `util` packages; name packages for what they provide.

**Why `internal/models/queue.go` separate from `models.go`:**
The `InputsQueue` type has behavior (methods) while the other types are pure data structs. Separating them within the same package improves readability without adding import complexity. Both are `package models`.

**Why NOT `pkg/` directory:**
The `pkg/` convention is for libraries that export packages for external consumption. This is a CLI tool; nothing should be importable outside the module. `internal/` enforces this at the compiler level.

### Component Boundaries

| Component | Package | Responsibility | Exports | Depends On |
|-----------|---------|---------------|---------|------------|
| Entry Point | `cmd/mxlrcgo-svc` | Parse CLI args, construct App, call Run | `main()` only | `internal/app`, `internal/models` |
| Application | `internal/app` | Own state (inputs/failed queues), orchestrate processing loop, signal handling, timer, failed-item handler | `App` struct, `Run()`, `NewApp()` | `internal/models`, `internal/musixmatch`, `internal/lyrics`, `internal/scanner` |
| Models | `internal/models` | Define all data types, input queue operations | `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue` | Nothing (leaf package) |
| API Client | `internal/musixmatch` | HTTP communication with Musixmatch API, JSON parsing | `Client` struct, `Fetcher` interface, `NewClient()` | `internal/models` |
| Lyrics Writer | `internal/lyrics` | Format song data as LRC, write to disk, filename sanitization | `WriteLRC()`, `Slugify()` | `internal/models` |
| Scanner | `internal/scanner` | Detect input mode, scan directories, parse text files, build input queue | `ParseInput()`, `ScanDir()` | `internal/models` |

### Key Design Decision: The Fetcher Interface

```go
// internal/musixmatch/client.go

// Fetcher abstracts the lyrics lookup so tests can mock the API.
type Fetcher interface {
    FindLyrics(track models.Track) (models.Song, error)
}

// Client implements Fetcher against the real Musixmatch desktop API.
type Client struct {
    Token      string
    HTTPClient *http.Client
}

func NewClient(token string) *Client {
    return &Client{
        Token:      token,
        HTTPClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (c *Client) FindLyrics(track models.Track) (models.Song, error) {
    // ... existing findLyrics logic
}
```

The `Fetcher` interface lives in `internal/musixmatch` alongside its primary implementation. The `App` struct accepts a `Fetcher` rather than a concrete `*Client`, enabling test doubles without import cycles.

### Key Design Decision: The App Struct

```go
// internal/app/app.go

// App owns the processing state that was previously global.
type App struct {
    Inputs  models.InputsQueue
    Failed  models.InputsQueue
    Fetcher musixmatch.Fetcher
    Args    models.Args
    Mode    string
}

func NewApp(args models.Args, fetcher musixmatch.Fetcher) *App {
    return &App{
        Fetcher: fetcher,
        Args:    args,
    }
}

func (a *App) Run() error {
    a.Mode = scanner.ParseInput(a.Args, &a.Inputs)
    // ... processing loop (moved from main.go)
    // ... signal handling (closeHandler)
    // ... failed handling (failedHandler)
}
```

This eliminates the two global `var inputs InputsQueue` and `var failed InputsQueue` from current `main.go:15-16`. State is instance-scoped, making the app testable and potentially runnable multiple times in a single process.

## Data Flow

### Current Flow (flat main package)

```
CLI args
  |
  v
main() --- parses args via go-arg
  |
  v
parseInput() --- detects mode, populates global `inputs` queue
  |
  v
Processing loop (in main) --- iterates `inputs.next()`/`inputs.pop()`
  |
  +---> Musixmatch.findLyrics(track) --- HTTP to API, returns Song
  |       |
  |       v
  +---> writeLRC(song, filename, outdir) --- formats LRC, writes file
  |
  v
failedHandler() --- writes _failed.txt from global `failed` queue
```

### Target Flow (cmd/internal layout)

```
cmd/mxlrcgo-svc/main.go
  |
  | 1. Parse CLI args (go-arg -> models.Args)
  | 2. Resolve token (flag > env > .env)
  | 3. Construct musixmatch.Client
  | 4. Construct app.App (with Client as Fetcher)
  | 5. Call app.Run()
  |
  v
internal/app.Run()
  |
  | 1. scanner.ParseInput(args, &a.Inputs) -> mode
  | 2. Setup signal handler (references a.Inputs, a.Failed)
  | 3. Processing loop:
  |     |
  |     +---> a.Fetcher.FindLyrics(track)  [interface call]
  |     |       |
  |     |       v
  |     |     internal/musixmatch.Client.FindLyrics()
  |     |       - builds HTTP request
  |     |       - parses fastjson response
  |     |       - returns models.Song
  |     |
  |     +---> lyrics.WriteLRC(song, filename, outdir)
  |             - slugify filename
  |             - format LRC tags
  |             - dispatch to synced/unsynced/instrumental writer
  |             - write to disk
  |
  | 4. a.handleFailed(mode, count)
  |     - write _failed.txt from a.Failed queue
  v
  return error (or nil)
```

### Import Graph (no cycles)

```
cmd/mxlrcgo-svc
  imports: internal/app, internal/models, internal/musixmatch

internal/app
  imports: internal/models, internal/musixmatch (for Fetcher interface),
           internal/lyrics, internal/scanner

internal/musixmatch
  imports: internal/models

internal/lyrics
  imports: internal/models

internal/scanner
  imports: internal/models

internal/models
  imports: nothing (leaf)
```

**Cycle-free by construction:** `models` is the leaf. Every other internal package depends on `models` and nothing else in `internal/`. Only `app` depends on multiple internal packages. The `cmd` entry point depends on `app`, `models`, and `musixmatch` (to construct the client).

## Patterns to Follow

### Pattern 1: Thin Main, Fat App

**What:** `cmd/mxlrcgo-svc/main.go` should be 15-25 lines. Parse args, resolve configuration, construct dependencies, call `app.Run()`, handle the returned error.

**When:** Always for CLI tools with any orchestration logic.

**Example:**

```go
// cmd/mxlrcgo-svc/main.go
package main

import (
    "log"
    "os"

    "github.com/alexflint/go-arg"
    "github.com/sydlexius/mxlrcgo-svc/internal/app"
    "github.com/sydlexius/mxlrcgo-svc/internal/models"
    "github.com/sydlexius/mxlrcgo-svc/internal/musixmatch"
)

func main() {
    var args models.Args
    arg.MustParse(&args)

    token := resolveToken(args.Token)
    client := musixmatch.NewClient(token)
    application := app.NewApp(args, client)

    if err := application.Run(); err != nil {
        log.Fatal(err)
    }
}

func resolveToken(flagToken string) string {
    if flagToken != "" {
        return flagToken
    }
    if envToken := os.Getenv("MUSIXMATCH_TOKEN"); envToken != "" {
        return envToken
    }
    // .env file fallback (godotenv or manual parsing)
    return ""
}
```

### Pattern 2: Accept Interfaces, Return Structs

**What:** `app.NewApp` accepts `musixmatch.Fetcher` (interface), not `*musixmatch.Client` (concrete type). Callers construct the concrete type; the app only depends on the capability contract.

**When:** Any component that has an external dependency you want to mock in tests.

**Why:** This is the standard Go testing pattern. The interface is small (single method), following Go's preference for narrow interfaces.

### Pattern 3: Models as Leaf Package

**What:** `internal/models` has zero imports from other internal packages. All data types live here. Every other package imports `models` but `models` imports nothing.

**When:** Always. This prevents import cycles by construction. If you find `models` needing to import another internal package, that logic belongs elsewhere.

### Pattern 4: Exported Names Within Internal

**What:** Types and functions in `internal/` packages use exported names (`WriteLRC`, `Track`, `FindLyrics`) even though they're not importable outside the module.

**When:** Always for `internal/` packages that are used by other packages in the same module.

**Why:** Go's `internal` directory already prevents external access. Using exported names within `internal` is how internal packages communicate. Unexported names are only visible within the same package.

## Anti-Patterns to Avoid

### Anti-Pattern 1: The "util" Package

**What:** Creating `internal/util/` or `internal/helpers/` as a catch-all.

**Why bad:** Becomes a dependency magnet. Everything imports `util`, nothing is cohesive. Creates hidden coupling.

**Instead:** Place helpers next to their caller. `slugify` goes in `lyrics/` because that's where it's used. `isInArray` should be replaced with a Go generic function local to where it's needed (Go 1.22+ supports this), or inlined since it's simple.

### Anti-Pattern 2: Circular Imports via "Convenience"

**What:** Having `scanner` import `musixmatch` to do API calls during directory scan, or `lyrics` import `scanner` for path resolution.

**Why bad:** Creates import cycles that Go rejects at compile time. More importantly, signals a boundary violation.

**Instead:** Keep the dependency graph as a DAG. `app` is the only package that connects `scanner`, `musixmatch`, and `lyrics`. They never talk to each other directly.

### Anti-Pattern 3: Interface Pollution

**What:** Defining interfaces for every internal package boundary (e.g., `Writer` interface for `lyrics`, `Scanner` interface for `scanner`).

**Why bad:** Premature abstraction. These internal packages have a single consumer (`app`). Interfaces add indirection without benefit unless you need to mock them.

**Instead:** Only define `Fetcher` because it crosses a network boundary (the Musixmatch API). File I/O in `lyrics` and `scanner` can be tested with temp dirs, no mock needed.

### Anti-Pattern 4: Config Struct Explosion

**What:** Creating a separate `internal/config` package to hold configuration, then passing it through every function.

**Why bad:** For this size of project, it adds a package for something that `models.Args` already handles. The token resolution logic is 10 lines in `main.go`.

**Instead:** Keep `Args` in `models`. Do token resolution in `main.go` before constructing the client. Pass resolved values directly.

## Build Order (Phase Dependency Graph)

The restructuring must follow dependency order. You cannot move a file into a new package if its types are not yet available in `models`.

```
Phase 1: internal/models
    Create models package with all types from structs.go.
    No dependencies. This unblocks everything else.

Phase 2: internal/musixmatch + internal/lyrics + internal/scanner  (parallelizable)
    Each depends only on models (created in Phase 1).
    Can be done in any order or simultaneously.
    - musixmatch: move from musixmatch.go, add Fetcher interface, export FindLyrics
    - lyrics: move from lyrics.go, move slugify from utils.go, export WriteLRC
    - scanner: move parseInput/getSong* from utils.go, export ParseInput

Phase 3: internal/app
    Depends on all Phase 2 packages + models.
    Move orchestration logic from main.go: processing loop, timer,
    failedHandler, closeHandler. Create App struct owning inputs/failed state.

Phase 4: cmd/mxlrcgo-svc/main.go
    Depends on app + models + musixmatch (to construct client).
    Thin entry point. Token resolution. Wire everything together.

Phase 5: Build system updates
    Makefile, goreleaser, CI workflows point to cmd/mxlrcgo-svc/.
    go.mod module path changes to sydlexius/mxlrcgo-svc.
    Depends on all code being in its final location.
```

**Critical constraint:** The module path rename (`go.mod`) should happen in Phase 5, after all internal packages are relocated. Renaming the module path changes every import statement. Doing it first means every subsequent file move requires updating imports twice.

**Alternative:** Rename module path first (Phase 0), then do all restructuring with the new import paths. Trade-off: single import-path update per file, but the build is broken until all files are moved. For a codebase this small, either order works. Recommend renaming last for cleaner incremental commits.

## Scalability Considerations

| Concern | Current (5 files) | After Restructure (~12 files) | Future Growth |
|---------|-------------------|-------------------------------|---------------|
| Build time | Trivial | Trivial | Still trivial; Go compiles packages in parallel |
| Test isolation | All tests share `main` package | Each package testable independently | Add test files per package freely |
| New features | Modify existing files | Add files to appropriate `internal/` package | Clear where new code goes |
| API client changes | Edit `musixmatch.go` | Edit `internal/musixmatch/client.go` | Can add multiple client implementations behind `Fetcher` |
| New output formats | Modify `lyrics.go` | Add new writer in `internal/lyrics/` | Writers share the package but are separate files |
| Concurrency (future) | Blocked by global state | `App` struct is instance-scoped | Can add worker pools in `app.Run()` without global conflicts |

## File Migration Map

Exact mapping from current files to target locations:

| Current File | Target Package | Target File(s) | What Moves |
|-------------|---------------|-----------------|------------|
| `structs.go` | `internal/models` | `models.go`, `queue.go` | All types + InputsQueue methods |
| `musixmatch.go` | `internal/musixmatch` | `client.go` | Musixmatch struct -> Client struct, findLyrics -> FindLyrics, add Fetcher interface |
| `lyrics.go` | `internal/lyrics` | `writer.go` | writeLRC -> WriteLRC, all write* functions |
| `utils.go` (slugify) | `internal/lyrics` | `slugify.go` | slugify -> Slugify (only caller is WriteLRC) |
| `utils.go` (scanner) | `internal/scanner` | `scanner.go` | parseInput, getSongDir, getSongText, getSongMulti, assertInput |
| `utils.go` (helpers) | inline / delete | -- | `isInArray` replaced with generic or `slices.Contains`; `supportedFType` moves to scanner |
| `main.go` (loop/state) | `internal/app` | `app.go` | Processing loop, timer, failedHandler, closeHandler, global vars -> App struct fields |
| `main.go` (entry) | `cmd/mxlrcgo-svc` | `main.go` | Arg parsing, token resolution, wiring |
| `utils_test.go` | `internal/lyrics` | `slugify_test.go` | TestSlugify (tests the slugify function that moves to lyrics/) |

## Sources

- Go official module layout guide: https://go.dev/doc/modules/layout (verified via Context7, HIGH confidence)
- Go internal package visibility rules: go.dev/doc/modules/layout, Go specification (verified via Context7, HIGH confidence)
- Current codebase analysis: all .go files read directly (HIGH confidence)
- PROJECT.md target layout specification (HIGH confidence)

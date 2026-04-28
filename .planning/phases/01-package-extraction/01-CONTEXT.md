# Phase 1: Package Extraction - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Rename the Go module from `github.com/fashni/mxlrc-go` to `github.com/sydlexius/mxlrcgo-svc`, create five internal packages (`models`, `musixmatch`, `lyrics`, `scanner`, `app`), export types, introduce interfaces, convert error handling from `log.Fatal`/`bool` returns to proper error returns, and clean up legacy naming. The entry point (`cmd/`) and token externalization are Phase 3. Global state elimination is Phase 2.

</domain>

<decisions>
## Implementation Decisions

### Package boundaries
- **D-01:** Five internal packages as defined in roadmap: `internal/models`, `internal/musixmatch`, `internal/lyrics`, `internal/scanner`, `internal/app`
- **D-02:** `InputsQueue` moves to `internal/app` (it is processing state, not a domain model). `models` holds only data types: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Inputs`
- **D-03:** `Args` struct stays in the main entry point (`main.go` for now, `cmd/mxlrcgo-svc/main.go` in Phase 3). Internal packages receive individual values, not the whole Args struct

### Interface design
- **D-04:** Two interfaces created in Phase 1: `Fetcher` in `internal/musixmatch` and `Writer` in `internal/lyrics`
- **D-05:** Interfaces live in the packages that implement them, not in a shared `models` package

### Error handling migration
- **D-06:** All `log.Fatal` calls removed from internal packages -- functions return errors instead
- **D-07:** `writeLRC` changes from returning `bool` to returning `error` (per API-04)
- **D-08:** Internal packages are allowed to emit informational log messages (progress like "searching for X...", "no synced lyrics found"), but never crash the program
- **D-09:** Switch to `log/slog` everywhere -- both internal packages and the main entry point. This pulls LOG-01 forward from v2 requirements into Phase 1

### Naming & export conventions
- **D-10:** Fix snake_case variable names during the move: `song_list` -> `songList`, `save_path` -> `savePath`, `text_fn` -> `textFn`, `lrc_file` -> `lrcFile`, etc.
- **D-11:** API base URL stays as a private (unexported) constant within the `internal/musixmatch` package. Not configurable via constructor
- **D-12:** All types and constructor functions exported (PascalCase) from internal packages as required by LAYOUT-03/LAYOUT-04

### Agent's Discretion
- Constructor function signatures (parameter types and order)
- Exact file layout within each internal package (single file vs multiple files per package)
- Whether to split `utils.go` helpers across packages or keep shared helpers in a common location
- `slugify` regex compilation approach (package-level `var` with `regexp.MustCompile`)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs -- requirements fully captured in decisions above and in the following planning docs:

### Project requirements
- `.planning/REQUIREMENTS.md` -- Full requirements list; Phase 1 covers MOD-01, MOD-02, LAYOUT-02 through LAYOUT-06, API-01, API-04, API-05
- `.planning/ROADMAP.md` -- Phase 1 success criteria (5 items that must be TRUE)

### Codebase analysis
- `.planning/codebase/STRUCTURE.md` -- Current flat file layout being restructured
- `.planning/codebase/CONVENTIONS.md` -- Go naming and style conventions to follow
- `.planning/codebase/CONCERNS.md` -- Tech debt items being addressed (global state, isInArray, regex compilation, snake_case)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `structs.go` -- All type definitions to be moved to `internal/models`. Types are already PascalCase exported.
- `musixmatch.go` -- API client logic to be moved to `internal/musixmatch`. Already structured as a method on `Musixmatch` struct.
- `lyrics.go` -- LRC writing functions to be moved to `internal/lyrics`. Functions: `writeLRC`, `writeSyncedLRC`, `writeUnsyncedLRC`, `writeInstrumentalLRC`.
- `utils.go` -- Mixed responsibilities to be split: `slugify` -> `lyrics` or shared util, `parseInput`/`getSongMulti`/`getSongText`/`getSongDir` -> `scanner`, `isInArray` -> replaced with `slices.Contains`, `assertInput` -> `scanner` or `app`
- `utils_test.go` -- Existing `slugify` tests, need to move to whichever package gets `slugify`

### Established Patterns
- Value receiver on `Musixmatch` (no mutation needed) -- maintain this pattern for the new `Client` type
- Pointer receiver on `InputsQueue` (mutates state) -- maintain for queue in `app`
- JSON struct tags on all types -- must preserve for API deserialization
- `fastjson` for API response parsing + `encoding/json` for struct unmarshaling -- keep this dual-parser approach

### Integration Points
- `main.go` currently imports nothing (flat `package main`). After extraction, `main.go` imports all five internal packages
- `go.mod` module path changes from `github.com/fashni/mxlrc-go` to `github.com/sydlexius/mxlrcgo-svc`
- No existing self-imports to update (confirmed: no cross-file package imports exist today)

</code_context>

<specifics>
## Specific Ideas

- User wants `slog` adopted now rather than deferring to v2 -- this is a deliberate scope expansion from LOG-01
- User is not deeply experienced with Go, so implementation should follow idiomatic patterns without clever abstractions

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 01-package-extraction*
*Context gathered: 2026-04-10*

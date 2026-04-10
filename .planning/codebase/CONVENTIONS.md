# Coding Conventions

**Analysis Date:** 2026-04-10

## Naming Patterns

**Files:**
- Flat, single-word lowercase names: `main.go`, `lyrics.go`, `musixmatch.go`, `structs.go`, `utils.go`
- Test files use Go's standard `_test.go` suffix: `utils_test.go`
- No subdirectories -- all `.go` files live in the project root

**Functions:**
- camelCase for unexported (all functions are unexported since single `main` package): `writeLRC()`, `slugify()`, `parseInput()`, `getSongDir()`
- Method receivers are short abbreviations: `mx` for `Musixmatch`, `q` for `InputsQueue`
- Mixed snake_case appears in some parameter names (legacy from Python port): `song_list`, `save_path`, `text_fn`, `lrc_file` -- avoid in new code, use camelCase

**Variables:**
- Short, abbreviated names preferred: `fn` (filename), `fp` (filepath), `mx` (Musixmatch), `cnt` (count), `cur` (current), `res` (response)
- Loop variables are single-letter: `i`, `f`, `m`, `v`
- Error variables are always `err` (shadowed freely within nested scopes)

**Types:**
- PascalCase for all exported types: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`
- Struct fields use PascalCase with JSON tags: `TrackName string \`json:"track_name,omitempty"\``
- All types defined in `structs.go`

**Constants:**
- SCREAMING_CASE for package-level URL constant: `const URL = "https://..."`

## Code Style

**Formatting:**
- `gofmt` is the canonical formatter (enforced by pre-commit hook and CI)
- `goimports` for import grouping (run via `make fmt`)
- Tab indentation for `.go` files, 2-space indentation for config files (`.editorconfig`)

**Linting:**
- `golangci-lint` v2 with `.golangci.yml` config
- Enabled linters: `errcheck`, `govet`, `staticcheck`, `unused`, `bodyclose`, `gosec`, `noctx`, `unconvert`, `unparam`, `wastedassign`, `misspell`, `revive`
- Test files excluded from: `gosec`, `errcheck`, `noctx`
- US English locale enforced via `misspell`
- `revive` configured with `disableStutteringCheck` and `unexported-return` warnings

**nolint Directives:**
- Always include a justification comment after `//nolint:linter`:
  ```go
  f, err := os.Open(text_fn) //nolint:gosec // path comes from user CLI argument
  ```
- Used sparingly -- only for `gosec` false positives on file operations with user-provided paths

**Line Endings:**
- LF enforced everywhere via `.gitattributes` (`* text=auto eol=lf`)

## Import Organization

**Order (enforced by goimports):**
1. Standard library packages
2. Third-party packages (blank line separator)

**Example from `musixmatch.go`:**
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/valyala/fastjson"
)
```

**Path Aliases:**
- None used. All imports are direct package paths.

## Error Handling

**Patterns -- two distinct strategies based on context:**

1. **Return errors to caller** (in library-like functions like `findLyrics`):
   ```go
   func (mx Musixmatch) findLyrics(track Track) (Song, error) {
       // Wrap with context using fmt.Errorf + %w
       return song, fmt.Errorf("failed to parse API URL: %w", err)
       // Simple sentinel errors
       return song, errors.New("no results found")
   }
   ```

2. **log.Fatal for unrecoverable errors** (in CLI-layer/startup code):
   ```go
   if err := os.MkdirAll(args.Outdir, 0750); err != nil {
       log.Fatal(err)
   }
   ```

3. **log.Println + return bool for file I/O** (in `lyrics.go`):
   ```go
   func writeLRC(song Song, filename string, outdir string) (success bool) {
       f, err := os.Create(fp)
       if err != nil {
           log.Println(err)
           return false
       }
   }
   ```

**HTTP response body closing:**
```go
defer func() { _ = res.Body.Close() }()
```
- Explicitly discard close error with `_ =` (satisfies `errcheck`)

**File close with error tracking via named return:**
```go
defer func() {
    if cerr := f.Close(); cerr != nil && success {
        log.Printf("error closing %s: %v", fp, cerr)
        success = false
    }
}()
```

**Response size limiting:**
```go
const maxResponseSize = 2 << 20 // 2 MiB
body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseSize+1))
if len(body) > maxResponseSize {
    return song, fmt.Errorf("musixmatch API response too large (%d bytes)", len(body))
}
```

## Logging

**Framework:** Standard library `log` package (no structured logging)

**Patterns:**
- `log.Printf("verb noun: %s", value)` for informational progress messages
- `log.Println(err)` for non-fatal error reporting
- `log.Fatal(err)` for unrecoverable startup/file errors
- `log.Fatalf("message: %v", err)` when adding context to fatal errors
- `fmt.Printf()` used for user-facing output (timer countdown, result counts)

**When to use which:**
- `log.*` for operational messages (searching, saving, skipping)
- `fmt.*` for user-facing interactive output (progress counters, prompts)

## Comments

**When to Comment:**
- `nolint` directives always get a `// reason` suffix
- Inline comments for non-obvious constants: `// 2 MiB`, `// forbidden chars in filename`
- No function-level doc comments on unexported functions (acceptable since single `main` package)

**JSDoc/TSDoc:** Not applicable (Go project)

**Commented-out code:**
- One instance exists: `// log.Println(baseURL.String())` in `musixmatch.go:46` -- avoid adding more

## Function Design

**Size:**
- Functions are generally short (10-40 lines)
- Longest function: `getSongDir()` at ~57 lines, `findLyrics()` at ~117 lines

**Parameters:**
- Pass pointer to `InputsQueue` when mutating: `func parseInput(args Args, in *InputsQueue) string`
- Value receiver on `Musixmatch` (no mutation needed): `func (mx Musixmatch) findLyrics(track Track)`
- Pointer receiver on `InputsQueue` (mutates state): `func (q *InputsQueue) push(i Inputs)`

**Return Values:**
- Named return for `success bool` pattern in `writeLRC()` (enables deferred close error tracking)
- `(value, error)` tuple for fallible operations: `findLyrics(track Track) (Song, error)`
- Simple `bool` for write operations: `writeSyncedLRC()`, `writeUnsyncedLRC()`
- Pointer-or-nil for validation: `assertInput(song string) *Track`

## Module Design

**Exports:**
- No exported functions or variables (single `main` package, CLI binary)
- All types are exported (PascalCase) for JSON deserialization via struct tags
- Two package-level mutable vars: `var inputs InputsQueue` and `var failed InputsQueue` in `main.go`

**Barrel Files:**
- Not applicable (flat structure, single package)

**File Organization:**
- `structs.go` -- all type definitions and `InputsQueue` methods
- `utils.go` -- input parsing, directory scanning, string utilities
- `lyrics.go` -- LRC file writing (synced, unsynced, instrumental)
- `musixmatch.go` -- API client and response parsing
- `main.go` -- CLI entry point, orchestration loop, signal handling

## Commit Conventions

**Format:** Conventional Commits enforced by pre-commit hook
- Prefixes: `feat:`, `fix:`, `docs:`, `style:`, `refactor:`, `perf:`, `test:`, `build:`, `ci:`, `chore:`, `revert:`
- No emoji in commits, code, comments, or documentation

## Spell Checking

**Tool:** `typos` CLI with `.typos.toml` config
- `go.sum` is excluded from spell checking
- Runs as both pre-commit hook check and pre-commit framework hook

## Secret Scanning

**Tool:** `gitleaks` via pre-commit framework (`.pre-commit-config.yaml`)
- Scans for accidentally committed secrets/credentials

---

*Convention analysis: 2026-04-10*

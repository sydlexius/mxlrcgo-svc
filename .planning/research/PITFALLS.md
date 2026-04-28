# Domain Pitfalls

**Domain:** Go CLI project restructuring (flat main package to cmd/internal layout)
**Project:** MxLRC-Go (sydlexius/mxlrcgo-svc)
**Researched:** 2026-04-10

## Critical Pitfalls

Mistakes that cause broken builds, runtime panics, or require rework.

### Pitfall 1: Circular imports between new internal packages

**What goes wrong:** When splitting a flat `package main` into `internal/models`, `internal/musixmatch`, `internal/lyrics`, and `internal/scanner`, you discover cross-dependencies that worked fine in one package but create import cycles when separated. In this codebase: `musixmatch.go` produces `Song` (needs models), `lyrics.go` consumes `Song` (needs models) and calls `slugify` (currently in utils.go), `utils.go` uses `Track`, `Inputs`, `InputsQueue` (needs models) and calls `isInArray` with `supportedFType`. Everything references `structs.go` types freely today because it is all one package.

**Why it happens:** In a single package, Go allows any file to reference any symbol in any other file. Splitting creates directed dependency edges. If package A imports package B, B cannot import A. Developers plan the split around file boundaries rather than dependency direction, then discover cycles at compile time.

**Consequences:** Compile failure. Either rework the package boundaries or introduce an unwanted dependency direction (e.g., models depending on musixmatch).

**Prevention:**
1. Map the actual call graph before moving any code. For this codebase:
   - `models` (structs.go): `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Inputs`, `Args`, `InputsQueue` -- zero imports of other project code.
   - `musixmatch` (musixmatch.go): imports `Track`, `Song` from models -- depends on models only.
   - `lyrics` (lyrics.go): imports `Song` from models, calls `slugify` -- depends on models. `slugify` must live somewhere lyrics can import (either in lyrics itself, or a shared `util` package, NOT in scanner).
   - `scanner` (utils.go dir/text/multi parsing): imports `Track`, `Inputs`, `InputsQueue` from models, calls `isInArray`/`supportedFType` -- depends on models.
2. `slugify` is currently in `utils.go` alongside scanner functions. If it moves to `internal/scanner`, then `internal/lyrics` would need to import `internal/scanner` just for `slugify`, while `scanner` has no reason to depend on `lyrics`. This is not a cycle, but it is a bad coupling. **Move `slugify` into `internal/lyrics`** (its only consumer) or a small `internal/fileutil` package.
3. `isInArray` and `supportedFType` are only used in `getSongDir`. They should stay with the scanner package or be inlined (replaced by `slices.Contains`).

**Detection:** Run `go build ./...` after each file move. If you get `import cycle not allowed`, you have this problem. Better: draw the dependency graph on paper first and verify it is a DAG.

**Phase:** Module rename + file relocation phase. Must be resolved before any code compiles.

---

### Pitfall 2: Module rename breaks all internal imports simultaneously

**What goes wrong:** Changing `go.mod` from `module github.com/fashni/mxlrc-go` to `module github.com/sydlexius/mxlrcgo-svc` breaks every import statement in the project that references the old module path. Since this project is currently flat `package main` with no internal imports, the module rename itself is safe TODAY. But if you restructure first (creating `internal/` packages with `import "github.com/fashni/mxlrc-go/internal/models"`), then rename, you must update every import path. If you rename first, then restructure, the new imports use the correct path from the start.

**Why it happens:** Go import paths are tied to the module path in `go.mod`. Module rename and package restructuring are two independent operations, but doing them in the wrong order multiplies the work and risk of stale import paths.

**Consequences:** All files fail to compile. Partially updated imports cause confusing errors like `cannot find package "github.com/fashni/mxlrc-go/internal/models" in any of:`. IDE tooling (gopls) breaks until all imports are consistent.

**Prevention:**
1. **Rename the module FIRST, before creating any internal packages.** Since the project is currently `package main` with no internal imports (only external deps), renaming `go.mod` is a one-line change with zero import updates needed.
2. Then create the `cmd/` and `internal/` structure using the NEW module path in all imports from the start.
3. Verify: after renaming `go.mod`, run `go build ./...` to confirm existing code still compiles (it will, because flat `package main` has no self-imports).

**Detection:** `go build ./...` fails with "cannot find module" or "cannot find package" errors referencing the old module path.

**Phase:** Must be the very first code change. Rename module, commit, then restructure.

---

### Pitfall 3: Exported vs unexported identifiers after package split

**What goes wrong:** All types and functions in the current codebase are accessible within `package main` regardless of capitalization. When split into separate packages, anything that needs to be used across package boundaries must be exported (capitalized). In this codebase:
- `InputsQueue` methods: `next()`, `pop()`, `push()`, `len()`, `empty()` are all lowercase. If `InputsQueue` moves to `internal/models` and `cmd/mxlrcgo-svc/main.go` needs to call them, they must become `Next()`, `Pop()`, `Push()`, `Len()`, `Empty()`.
- `findLyrics()` on `Musixmatch` must become `FindLyrics()`.
- `writeLRC()`, `writeSyncedLRC()`, `writeUnsyncedLRC()`, `writeInstrumentalLRC()` must be exported if called from outside `internal/lyrics`.
- `parseInput()`, `getSongMulti()`, `getSongText()`, `getSongDir()`, `assertInput()` need export if called from outside scanner.
- `slugify()` needs export only if called cross-package (if kept in lyrics, it can stay unexported).
- `timer()`, `failedHandler()`, `closeHandler()` stay in main or the App struct -- may not need export.

**Why it happens:** In `package main`, capitalization is irrelevant for internal usage. Developers move files into new packages, change the `package` declaration, and forget to capitalize the identifiers that are now cross-package calls.

**Consequences:** Compile errors: `q.next undefined (type InputsQueue has no field or method next)`. Many of them at once, across many files, creating a frustrating wall of errors.

**Prevention:**
1. Before moving each file, list every symbol it defines and every symbol it references from other files.
2. For each symbol that will be in a DIFFERENT package from its caller, capitalize it.
3. Do this per-package, compile, fix, then move the next package. Do NOT move all files at once.
4. Consider: if a method does not need to be called from outside its package, keep it unexported. E.g., `writeSyncedLRC` could remain unexported inside `internal/lyrics` if only `WriteLRC` calls it.

**Detection:** `go build ./...` produces "undefined" or "cannot refer to unexported" errors immediately.

**Phase:** File relocation phase. Address per-file as each file moves to its new package.

---

### Pitfall 4: Global state removal breaks the signal handler race contract

**What goes wrong:** The current signal handler (`closeHandler`) directly accesses the global `inputs` and `failed` variables from a separate goroutine. When refactoring to an `App` struct that owns these queues, the signal handler must also be updated. But the underlying race condition (documented in CONCERNS.md) still exists: the goroutine and main loop both access the queues without synchronization. Simply moving global vars into a struct does NOT fix the race -- it just moves the data race from globals to struct fields.

**Why it happens:** Developers see "eliminate global state" as the goal, move vars into a struct, and consider it done. But the actual problem is concurrent access without synchronization. A struct field is just as racey as a global if two goroutines access it without a mutex or channel.

**Consequences:** Go race detector (`go test -race`) still flags data races. Under concurrent access during SIGTERM, queue corruption can cause panics (index out of range on empty queue).

**Prevention:**
1. When creating the `App` struct, use `context.Context` with cancellation for signal handling instead of direct goroutine access to shared state.
2. Pattern: `closeHandler` sets a `context.CancelFunc`, main loop checks `ctx.Done()` channel on each iteration, drains remaining items to failed list under its own (single-goroutine) control.
3. This eliminates the need for a mutex entirely -- only one goroutine touches the queues, the signal just triggers a context cancellation.
4. Run `go test -race` after refactoring to verify no data races remain.

**Detection:** `go test -race ./...` flags races. Also: review any goroutine that accesses App struct fields and ask "who else touches this concurrently?"

**Phase:** Global state elimination phase. Must be designed into the `App` struct from the start, not patched after.

---

### Pitfall 5: GoReleaser, CI, and Makefile not updated for new `cmd/` entrypoint

**What goes wrong:** After moving `main.go` to `cmd/mxlrcgo-svc/main.go`, every build path reference breaks:
- `.goreleaser.yml` has `main: .` -- must become `main: ./cmd/mxlrcgo-svc`
- `.goreleaser.yml` has `binary: mxlrc-go` -- must become `binary: mxlrcgo-svc`
- `Makefile` has `go build -o $(BINARY) .` -- must become `go build -o $(BINARY) ./cmd/mxlrcgo-svc`
- CI workflow (`ci.yml`) has `go build -ldflags="-s -w" -o mxlrc-go .` -- must update both the binary name and the build path
- `go install` path changes: users now run `go install github.com/sydlexius/mxlrcgo-svc/cmd/mxlrcgo-svc@latest`

**Why it happens:** Developers focus on the Go code restructuring and forget that the build pipeline is a separate system with its own hardcoded paths. These failures only surface when CI runs or someone tries `make build`.

**Consequences:** CI fails on every push until fixed. GoReleaser fails on the next tag, producing no release binaries. Users cannot install via `go install`.

**Prevention:**
1. Make a checklist of every file that references the binary name or build path:
   - `Makefile` (BINARY var, build target)
   - `.goreleaser.yml` (builds.main, builds.binary, release.github.name)
   - `.github/workflows/ci.yml` (build-matrix step `go build` command)
   - `.github/workflows/release.yml` (indirectly via goreleaser)
   - `README.md` (install instructions, usage examples)
2. Update all of them in the SAME commit as the file relocation.
3. Run `make build` locally before pushing to verify the Makefile works.
4. Run `goreleaser check` (or `goreleaser build --snapshot --clean`) locally to verify the release config.

**Detection:** `make build` fails. `goreleaser check` fails. CI build job fails.

**Phase:** File relocation phase, same commit as moving main.go to cmd/.

---

## Moderate Pitfalls

### Pitfall 6: Test file package declaration mismatch

**What goes wrong:** `utils_test.go` declares `package main` and tests `slugify()` (an unexported function). When `slugify` moves to `internal/lyrics`, the test must move too and declare `package lyrics`. If the test stays in the wrong package or uses `package lyrics_test` (external test package), it cannot access unexported `slugify`.

**Prevention:**
1. Move test files alongside their source files into the same `internal/` package.
2. `utils_test.go` tests `slugify`, which moves to `internal/lyrics`. Create `internal/lyrics/writer_test.go` (or similar) with `package lyrics` and move the `TestSlugify` function.
3. Update test function names if they reference old function names.
4. Run `go test ./...` after each file move to catch breakage early.

**Phase:** File relocation phase. Move tests with their source code.

---

### Pitfall 7: `go-arg` `Args` struct placement creates an import direction problem

**What goes wrong:** The `Args` struct defines CLI flags via `go-arg` struct tags. It lives in `structs.go` today. If it moves to `internal/models`, then `internal/models` depends on `go-arg` (a CLI parsing library) just for struct tags. This is backwards: models should be dependency-free, and CLI concerns should stay in `cmd/`.

If `Args` stays in `cmd/mxlrcgo-svc/main.go`, that is correct for separation of concerns. But then `parseInput` (which receives `Args`) must be called from `cmd/` and cannot live in `internal/scanner` unless scanner accepts individual parameters instead of the full `Args` struct.

**Prevention:**
1. Keep `Args` in `cmd/mxlrcgo-svc/` (it is a CLI concern).
2. Have `cmd/main.go` parse args, then call scanner functions with individual parameters: `scanner.ParseInput(songs []string, outdir string, update bool, depth int, bfs bool, queue *models.InputsQueue)` rather than passing the `Args` struct.
3. This keeps `internal/scanner` independent of the CLI framework.
4. Do NOT put `Args` in `internal/models` -- it does not belong there.

**Phase:** File relocation phase. Design the function signatures before moving code.

---

### Pitfall 8: Token externalization precedence logic scattered across layers

**What goes wrong:** The project requirement is: CLI flag > env var > .env file. If this logic is implemented partially in the `Args` struct (via `go-arg`'s `env` tag), partially in `main.go` (reading .env), and partially in the `Musixmatch` client, the precedence becomes unclear and hard to test.

Additionally, `go-arg` supports `env` tags natively (`arg:"env:MUSIXMATCH_TOKEN"`), which handles CLI flag > env var precedence automatically. But .env file support requires a separate library or manual loading. If the .env file is loaded (setting environment variables) AFTER `arg.MustParse`, the env var value is ignored because parsing already happened.

**Prevention:**
1. Load .env file FIRST (if present), using `os.Setenv` or a library like `joho/godotenv`.
2. THEN call `arg.MustParse`, which reads the env var (set by .env or shell) and the CLI flag, with CLI flag winning.
3. This gives the correct precedence: CLI flag > env var (whether from shell or .env) automatically.
4. Do NOT implement custom precedence logic -- let `go-arg`'s built-in `env` tag handle it.
5. The `Args` struct becomes: `Token string \`arg:"-t,--token,env:MUSIXMATCH_TOKEN" help:"musixmatch token"\``
6. Remove the hardcoded fallback token. If no token is provided by any method, exit with a clear error message.

**Phase:** Token externalization phase. Must be designed holistically, not incrementally.

---

### Pitfall 9: Forgetting to update `.golangci.yml` exclusion paths

**What goes wrong:** The linter config (`.golangci.yml`) may have path-based exclusions (e.g., excluding test files from gosec, or excluding specific files from certain linters). When files move from root to `internal/*/`, the exclusion patterns no longer match. Linters that were previously suppressed suddenly fire on the restructured code, or new code in `internal/` is not covered by intended rules.

**Prevention:**
1. Review `.golangci.yml` for any `path:` or `path-except:` patterns.
2. Update patterns to match new file locations (e.g., `internal/.*_test\.go` instead of `.*_test\.go` if path-specific).
3. Run `make lint` after restructuring and review any new warnings -- they may be previously suppressed issues now surfacing due to path changes.

**Phase:** File relocation phase. Update alongside file moves.

---

### Pitfall 10: `log.Fatal` calls in library code prevent clean shutdown

**What goes wrong:** Several functions use `log.Fatal()` for error handling: `getSongText` (utils.go:48), `getSongDir` (utils.go:67), `parseInput` (utils.go:134), `failedHandler` (main.go:97,109), and `isInArray` (utils.go:154). When these functions move into `internal/` packages, `log.Fatal` calls `os.Exit(1)`, which skips deferred functions, prevents cleanup, and makes the functions untestable (calling them in a test kills the test process).

**Prevention:**
1. When moving functions to `internal/` packages, convert `log.Fatal(err)` to `return ..., err` or `return ..., fmt.Errorf("context: %w", err)`.
2. Let the caller in `cmd/main.go` decide whether to fatal or handle the error.
3. Library code (anything in `internal/`) should NEVER call `os.Exit` or `log.Fatal`.
4. This is the single biggest behavior-preservation risk: functions that currently abort now return errors, and the caller must handle them.

**Phase:** File relocation phase. Convert error handling as each function moves.

---

## Minor Pitfalls

### Pitfall 11: `go.sum` stale entries after module rename

**What goes wrong:** After renaming the module in `go.mod`, `go.sum` may contain entries for the old module path. Running `go mod tidy` cleans this up, but forgetting to do so can cause confusing warnings or checksum mismatches.

**Prevention:** Run `go mod tidy` immediately after changing the module path in `go.mod`. Commit the updated `go.sum` with the module rename.

**Phase:** Module rename phase.

---

### Pitfall 12: Goreleaser repo name mismatch

**What goes wrong:** `.goreleaser.yml` has `release.github.name: mxlrc-go`. If the GitHub repository is renamed to `mxlrcgo-svc`, this must be updated. If the repo is NOT renamed (staying `mxlrc-go` while the module is `mxlrcgo-svc`), the goreleaser config is correct as-is but the binary name and module name diverge from the repo name -- a source of user confusion.

**Prevention:** Decide upfront whether the GitHub repo will be renamed. If yes, update `release.github.name` in `.goreleaser.yml`. If no, document the discrepancy.

**Phase:** Module rename phase.

---

### Pitfall 13: IDE/gopls confusion during mid-restructure state

**What goes wrong:** During the restructuring, the project passes through intermediate states where some files are in the new location and some are still in the old location. `gopls` (the Go language server) can get confused, showing phantom errors, not recognizing moved packages, or caching stale analysis. This is especially disruptive if using VS Code with the Go extension.

**Prevention:**
1. Restructure in atomic commits -- move one complete "unit" (source + test + all references) at a time.
2. After each move, run `go build ./...` from the terminal (not relying on IDE).
3. If gopls is confused, restart it (VS Code: `Go: Restart Language Server`).
4. Do NOT trust IDE error highlights during restructuring -- verify with `go build` from CLI.

**Phase:** File relocation phase. Ongoing during the restructuring work.

---

### Pitfall 14: Dependency versions not updated during restructuring

**What goes wrong:** The existing `go.mod` has outdated dependencies (e.g., `golang.org/x/text v0.3.8` when current is v0.14+). Restructuring is an opportunity to update, but updating dependencies AND restructuring in the same commit makes it hard to isolate breakage. If a test fails, is it the restructuring or the dependency update?

**Prevention:**
1. Do NOT update dependencies during restructuring. Keep the same pinned versions.
2. After restructuring is complete and all tests pass, update dependencies in a separate commit/PR.
3. Exception: if a dependency update is required for the restructuring to work (unlikely here), do it as a separate preceding commit with its own test run.

**Phase:** Post-restructuring. Separate concern entirely.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Module rename | Stale go.sum, goreleaser repo name mismatch | `go mod tidy` immediately; decide on repo rename upfront |
| File relocation | Circular imports, unexported identifiers, test file misplacement | Map dependency graph first; capitalize cross-package symbols; move tests with source |
| File relocation | Build pipeline breakage (Makefile, CI, goreleaser) | Checklist of all files referencing binary name/build path; update atomically |
| File relocation | `log.Fatal` in library code | Convert to error returns as each function moves to `internal/` |
| Global state elimination | Signal handler still races | Use `context.Context` cancellation instead of goroutine access to shared state |
| Token externalization | Precedence logic fragmentation | Load .env before `arg.MustParse`; use go-arg's native `env` tag; no custom logic |
| CLI/Args separation | Args struct in wrong package | Keep `Args` in cmd/; pass individual params to internal packages |
| Linting | Exclusion paths no longer match | Review `.golangci.yml` path patterns after file moves; run `make lint` |

## Sources

- go.dev/doc/modules/layout (official Go module layout guidance) -- HIGH confidence
- Context7: /golang/go -- Go module system, import path resolution, internal directory enforcement -- HIGH confidence
- Context7: /alexflint/go-arg -- env tag support for environment variable precedence -- HIGH confidence
- CONCERNS.md codebase analysis -- direct source code inspection -- HIGH confidence
- PROJECT.md target layout and requirements -- direct project documentation -- HIGH confidence
- Go specification: internal directory import restriction -- HIGH confidence

---

*Concerns audit: 2026-04-10*

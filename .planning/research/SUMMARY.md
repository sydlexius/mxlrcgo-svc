# Project Research Summary

**Project:** mxlrcgo-svc (MxLRC-Go restructuring)
**Domain:** Go CLI project restructuring (flat main package to idiomatic cmd/internal layout)
**Researched:** 2026-04-10
**Confidence:** HIGH

## Executive Summary

This project is a structural refactor of a Go CLI tool that fetches synced lyrics from the Musixmatch API. The codebase is currently a flat `package main` with five files, global mutable state, and a hardcoded API token. The goal is to restructure it into the idiomatic Go `cmd/` + `internal/` layout, eliminate global state, externalize the API token, and introduce a testability boundary (the `Fetcher` interface). No new features are being added -- this is a pure restructuring with a security fix (token externalization).

The recommended approach is well-established: the official Go module layout guide prescribes exactly this pattern for "command with supporting packages." The restructuring creates five internal packages (`app`, `models`, `musixmatch`, `lyrics`, `scanner`) with a cycle-free dependency graph rooted at a leaf `models` package. The stack changes are minimal -- upgrade existing dependencies (go-arg, fastjson, x/text, dhowden/tag) and add one new dependency (godotenv for .env file loading). No framework changes, no new paradigms. The hardest part is executing the migration in the right order without breaking the build at intermediate steps.

The key risks are (1) circular imports from moving code into separate packages without mapping the dependency graph first, (2) the module rename breaking all imports if done at the wrong point in the sequence, and (3) the global-state-to-struct migration silently preserving a race condition in the signal handler. All three are preventable with the phased approach laid out below. The codebase is small enough (~5 files, ~800 lines) that the entire restructuring is tractable in a single focused effort.

## Key Findings

### Recommended Stack

The stack is conservative and well-justified. No framework swaps, no speculative additions. Every change has a concrete reason tied to the restructuring goals.

**Core technologies:**
- **Go 1.24+ (go.mod minimum):** Two releases back from current stable (1.26.2) for CI compatibility. No features from 1.25/1.26 are needed.
- **go-arg v1.6.1:** Upgrade from v1.4.3. Native `env` struct tag support gives CLI flag > env var > default precedence for free. Eliminates need for custom token resolution logic.
- **godotenv v1.5.1:** New dependency. Loads `.env` file into `os.Environ` before `arg.MustParse`, giving .env values the lowest precedence automatically. Declared feature-complete by maintainer.
- **fastjson v1.6.10, x/text v0.36.0, dhowden/tag latest:** Dependency bumps for bug fixes and security. No API changes.

**What NOT to use:** cobra/viper (over-engineered for single-command CLI), sonic (needs CGO), koanf (overkill), `pkg/` directory (no public API).

### Expected Features

**Must have (table stakes) -- all required for M0:**
- `cmd/mxlrcgo-svc/main.go` entry point (Go convention, correct `go install` binary name)
- `internal/` package hierarchy (compiler-enforced boundaries)
- Global state elimination (the documented known issue)
- Exported types across packages (mechanical but necessary)
- `Fetcher` interface for Musixmatch client (testability boundary)
- Token externalization: CLI flag > env var > .env file (security fix)
- Proper error returns (`error` not `bool`)
- Updated Makefile/CI/GoReleaser for new paths
- Module path rename to `sydlexius/mxlrcgo-svc`
- Behavior preservation across all three input modes

**Should have (low-cost differentiators to include in M0):**
- Constructor functions (`New...`) -- do during package creation, zero marginal cost
- Compile regex once (package-level `var` instead of per-call) -- trivial fix during file move
- Replace `isInArray` with `slices.Contains` -- one-line fix, removes reflect dependency

**Defer to post-M0:**
- Structured logging (slog) -- valuable but expands scope
- Context propagation -- design it into App.Run() but full propagation is post-M0
- Reusable HTTP client -- low complexity but separate concern
- Consistent error wrapping -- standardize during moves but full audit is post-M0
- New test coverage -- PROJECT.md explicitly defers this

### Architecture Approach

The target is a strict DAG of five internal packages with `models` as the leaf node. `app` is the sole orchestrator that connects `scanner`, `musixmatch`, and `lyrics` -- they never import each other. The `cmd/mxlrcgo-svc/main.go` is a 15-25 line thin entry point that parses args, constructs dependencies, and calls `app.Run()`. The `Fetcher` interface lives in `internal/musixmatch` alongside its implementation, accepted by `App` as a constructor parameter. `Args` stays in `cmd/` (CLI concern, not a model) and scanner functions accept individual parameters rather than the full Args struct.

**Major components:**
1. **`cmd/mxlrcgo-svc`** -- Thin entry: parse args, load .env, construct App, call Run()
2. **`internal/app`** -- Owns state (input/failed queues), orchestrates processing loop, signal handling
3. **`internal/models`** -- All data types (Track, Song, Lyrics, InputsQueue). Zero internal imports. Leaf package.
4. **`internal/musixmatch`** -- API client + Fetcher interface. HTTP communication, JSON parsing. Depends only on models.
5. **`internal/lyrics`** -- LRC formatting, file writing, slugify. Depends only on models.
6. **`internal/scanner`** -- Input mode detection, directory scanning, text file parsing. Depends only on models.

### Critical Pitfalls

1. **Circular imports between internal packages** -- Map the dependency graph as a DAG before moving any code. Key trap: `slugify` must move to `lyrics/` (its only caller), NOT stay with scanner functions. Replace `isInArray` with `slices.Contains` inline.

2. **Module rename breaks all imports if sequenced wrong** -- Rename the module FIRST (Phase 1), before creating internal packages. Currently flat `package main` with no self-imports, so the rename is a zero-breakage one-line change. All new packages then use the correct path from the start.

3. **Signal handler race condition survives the struct migration** -- Moving globals into an App struct does NOT fix the concurrent access. Use `context.Context` with cancellation: signal handler calls cancel, main loop checks `ctx.Done()`. Only one goroutine touches the queues. Run `go test -race` to verify.

4. **Build pipeline not updated for new cmd/ path** -- Makefile, .goreleaser.yml, CI workflows all have hardcoded paths and binary names. Update atomically with the file relocation. Checklist: BINARY var, builds.main, builds.binary, release.github.name, CI go build commands.

5. **`log.Fatal` in library code prevents clean shutdown and testability** -- Convert all `log.Fatal` calls to error returns when moving functions into `internal/` packages. Library code must never call `os.Exit`.

## Implications for Roadmap

### Phase 1: Module Rename
**Rationale:** Must happen first. Currently flat `package main` with no self-imports, so this is a zero-risk one-line change. All subsequent phases create packages using the new module path, avoiding double import-path updates.
**Delivers:** Updated `go.mod` module path (`sydlexius/mxlrcgo-svc`), cleaned `go.sum`.
**Addresses:** Module path rename (table stakes)
**Avoids:** Pitfall 2 (module rename breaks imports)

### Phase 2: Models Package + Type Export
**Rationale:** `models` is the leaf package with zero internal dependencies. Every other package depends on it, so it must exist first. This unblocks all other package creation.
**Delivers:** `internal/models/models.go` (all types exported), `internal/models/queue.go` (InputsQueue with exported methods).
**Addresses:** `internal/` hierarchy, exported types (table stakes)
**Avoids:** Pitfall 3 (unexported identifiers), Pitfall 1 (cycles -- models imports nothing)

### Phase 3: Domain Packages (musixmatch, lyrics, scanner)
**Rationale:** These three packages depend only on `models` (created in Phase 2) and are independent of each other. Can be done in any order or in parallel. Bulk of the file migration work.
**Delivers:** `internal/musixmatch/client.go` (Fetcher interface + Client), `internal/lyrics/writer.go` + `slugify.go`, `internal/scanner/scanner.go`. All `log.Fatal` calls converted to error returns. `isInArray` replaced with `slices.Contains`. Regex compiled once at package level. Constructor functions for each package.
**Addresses:** Fetcher interface, error return cleanup, three low-cost differentiators (table stakes + should-haves)
**Avoids:** Pitfall 1 (slugify in lyrics, not scanner), Pitfall 6 (test file moves with source), Pitfall 10 (log.Fatal converted)

### Phase 4: App Package + Global State Elimination
**Rationale:** Depends on all Phase 3 packages existing. The App struct replaces global `inputs`/`failed` vars, owns the processing loop, and integrates signal handling via context cancellation.
**Delivers:** `internal/app/app.go` (App struct, NewApp, Run method). Global mutable state eliminated. Signal handler uses context cancellation instead of direct goroutine queue access.
**Addresses:** Global state elimination, App struct (table stakes)
**Avoids:** Pitfall 4 (signal handler race -- context.Context pattern designed in from the start)

### Phase 5: Entry Point + Token Externalization + Build System
**Rationale:** The entry point depends on `app`, `models`, and `musixmatch` all being in place. Token externalization is implemented here (godotenv.Load before arg.MustParse). Build system updates must happen atomically with the main.go relocation to keep CI green.
**Delivers:** `cmd/mxlrcgo-svc/main.go` (thin entry point), token precedence (CLI > env > .env), updated Makefile, .goreleaser.yml, CI workflows. Hardcoded token removed. `Args` struct lives in `cmd/`. Behavior verification across all three input modes.
**Addresses:** Entry point, token externalization, build tooling, behavior preservation (table stakes)
**Avoids:** Pitfall 5 (build pipeline breakage), Pitfall 7 (Args in cmd/ not models), Pitfall 8 (token precedence in one place), Pitfall 9 (linter path updates)

### Phase 6: Dependency Upgrades
**Rationale:** Separate from restructuring to isolate breakage sources. If a test fails after Phase 5, it is the restructuring. If it fails after Phase 6, it is the dependency.
**Delivers:** Updated go-arg, fastjson, x/text, dhowden/tag. New godotenv dependency (may be pulled in during Phase 5 if needed for token work -- if so, just the version bumps happen here).
**Addresses:** Dependency currency, security updates
**Avoids:** Pitfall 14 (dependency updates mixed with restructuring)

### Phase Ordering Rationale

- **Strict dependency order:** Each phase produces artifacts the next phase depends on. Models before domain packages before app before entry point.
- **Module rename first, not last:** ARCHITECTURE.md suggested renaming last for "cleaner incremental commits" but PITFALLS.md correctly identifies that renaming first is zero-risk (no self-imports exist) and avoids the double-import-update problem. Renaming first wins.
- **Dependency upgrades last:** Isolates "does the restructured code work" from "did a dependency change break something." Clear bisection point.
- **Args stays in cmd/:** PITFALLS.md Pitfall 7 and ARCHITECTURE.md Anti-Pattern 4 agree: Args is a CLI concern. Scanner functions accept individual parameters, keeping internal packages framework-independent.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 4 (App + global state):** The signal handler refactoring with context.Context needs careful design. The current closeHandler has subtle behavior (writing failed items before exit) that must be preserved while eliminating the race condition. Worth a `/gsd-research-phase` to nail the exact context cancellation pattern.

Phases with standard patterns (skip research):
- **Phase 1 (Module rename):** One-line `go mod edit` + `go mod tidy`. Fully mechanical.
- **Phase 2 (Models):** Copy types, capitalize names. Fully mechanical.
- **Phase 3 (Domain packages):** File moves with export capitalization. Well-documented pattern. The dependency graph is already mapped.
- **Phase 5 (Entry point + build):** Thin main pattern is well-documented. godotenv + go-arg integration has a clear recipe from STACK.md.
- **Phase 6 (Dependency upgrades):** `go get` + `go mod tidy`. Fully mechanical.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All versions verified via pkg.go.dev. go-arg env tag support confirmed via Context7. godotenv is feature-complete and battle-tested. No speculative choices. |
| Features | HIGH | Features derived directly from PROJECT.md requirements + codebase inspection. Clear table-stakes vs. differentiator separation. Anti-features well-justified. |
| Architecture | HIGH | Based on official Go module layout guide (go.dev/doc/modules/layout). Import graph verified cycle-free by construction. File migration map is exact. |
| Pitfalls | HIGH | 14 pitfalls identified from direct codebase analysis + Go compilation semantics. All include detection and prevention strategies. Phase-specific warnings mapped. |

**Overall confidence:** HIGH

All four research dimensions used primary sources (official Go documentation, pkg.go.dev version verification, direct codebase analysis, Context7 with high-reputation libraries). No findings rely on blog posts, opinions, or unverified claims.

### Gaps to Address

- **Repository rename decision:** PITFALLS.md flags that the GitHub repo name (`mxlrc-go`) may diverge from the module name (`mxlrcgo-svc`). This needs a decision before Phase 1: rename the repo or document the discrepancy. Affects goreleaser config and user-facing install instructions.
- **godotenv loading timing in Phase 5 vs Phase 6:** If token externalization (Phase 5) requires godotenv, it must be added as a dependency in Phase 5, not Phase 6. The phasing should treat godotenv as part of the token work, not as a "dependency upgrade."
- **Existing test coverage for behavior preservation:** Only `utils_test.go` exists (tests `slugify`). Behavior preservation across all three input modes relies on manual smoke testing. No automated integration tests exist. This is an accepted gap per PROJECT.md ("move existing tests only, no new coverage").

## Sources

### Primary (HIGH confidence)
- Go official module layout guide: https://go.dev/doc/modules/layout
- Context7 /alexflint/go-arg -- env tag support, CLI flag > env var precedence
- Context7 /golang/go -- module system, internal directory enforcement
- pkg.go.dev -- version verification for all dependencies
- Direct codebase analysis -- all .go files read

### Secondary (MEDIUM confidence)
- Context7 /golang-standards/project-layout (benchmark score 91.6) -- community reference for Go project structure, not official but widely adopted
- Context7 /spf13/cobra -- consulted to confirm it is NOT needed for this project

---
*Research completed: 2026-04-10*
*Ready for roadmap: yes*

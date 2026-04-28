# MxLRC-Go (sydlexius/mxlrcgo-svc)

## What This Is

A Go CLI tool that fetches synced lyrics from the Musixmatch API and saves them as `.lrc` files. This is a fork of fashni/mxlrc-go being restructured into a maintainable Go project layout under a new module path, with global state eliminated and the hardcoded API token externalized.

## Core Value

The tool fetches synced lyrics reliably and writes correct `.lrc` files. Everything else (project structure, config handling, CI) exists to support that.

## Requirements

### Validated

- Fetch synced lyrics from Musixmatch desktop API -- existing
- Write `.lrc` files with metadata tags (artist, title, album, length) -- existing
- Support three input modes: CLI pairs, text file, directory scan -- existing
- Read audio file metadata via ID3/MP4/FLAC tags for directory mode -- existing
- Rate limiting via configurable cooldown between API calls -- existing
- Graceful shutdown with failed-item retry file (`_failed.txt`) -- existing
- Cross-platform builds (linux/darwin/windows, amd64/arm64) -- existing
- BFS/DFS directory traversal options -- existing
- Rename Go module to `sydlexius/mxlrcgo-svc` -- completed M0
- Restructure flat main package into `cmd/mxlrcgo-svc/` + `internal/` layout -- completed M0
- Eliminate global `inputs` and `failed` variables -- completed M0
- Externalize Musixmatch token (CLI flag > env var > .env file) -- completed M0
- Define `Fetcher` interface for the Musixmatch client -- completed M0
- Export types and methods from internal packages -- completed M0
- Update Makefile, CI workflows, and goreleaser for new binary name and paths -- completed M0
- Update README for new module path and binary name -- completed M0

### Out of Scope

- New features or behavioral changes beyond restructuring -- M0 is structural only
- Additional test coverage beyond relocating existing tests -- deferred to later milestone
- Database or persistent state -- not needed yet
- Web server or API mode -- not planned for M0

## Context

This is a fork of `fashni/mxlrc-go`, itself a Go port of the Python MxLRC tool. M0 completed the full restructuring: the codebase now uses `cmd/` + `internal/` layout, all global state is eliminated, and the API token is loaded from the correct precedence chain (CLI flag > env var > `.env` file).

The existing codebase has minimal test coverage (`slugify_test.go`). Quality gating relies heavily on linters (golangci-lint with 12 linters), pre-commit hooks, and CI.

Current layout after M0:
```
cmd/mxlrcgo-svc/main.go
internal/models/models.go
internal/musixmatch/client.go
internal/lyrics/writer.go
internal/lyrics/slugify.go
internal/scanner/scanner.go
internal/app/app.go
internal/app/queue.go
```

## Constraints

- **Binary name**: `mxlrcgo-svc` (matches new module name)
- **No CGO**: Must remain CGO_ENABLED=0 for cross-compilation
- **Go 1.25+**: Minimum Go version per go.mod (bumped from 1.22 during M0 for x/text v0.36.0 compatibility)
- **Behavior preservation**: All existing CLI flags and behaviors must work identically after restructuring
- **Token precedence**: CLI flag > environment variable (`MUSIXMATCH_TOKEN`) > `.env` file

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Module path: `sydlexius/mxlrcgo-svc` | New fork identity, distinct from upstream | Implemented in M0 |
| App struct for state ownership | Replaces global `inputs`/`failed` vars; enables testability | Implemented in M0 |
| Token: flag + env + .env | Maximum flexibility; flag for scripting, env for CI, .env for local dev | Implemented in M0 |
| `Fetcher` interface on Musixmatch client | Enables mocking in tests without hitting the real API | Implemented in M0 |
| Move existing tests only, no new coverage | Keep M0 scope tight; test coverage is a separate concern | Implemented in M0 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check -- still the right priority?
3. Audit Out of Scope -- reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-10 after M0 completion*

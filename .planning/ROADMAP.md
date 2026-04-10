# Roadmap: mxlrcsvc-go (M0: Fork & Restructure)

## Overview

Transform the flat `package main` codebase into an idiomatic Go project with `cmd/` + `internal/` layout, eliminate global mutable state, externalize the hardcoded API token, and update all build tooling for the new module identity. Four phases in strict dependency order: extract packages, wire the App struct, create the entry point, then update the build system and verify everything works.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3, 4): Planned milestone work
- Decimal phases (e.g., 2.1): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Package Extraction** - Rename module, create all internal packages, export types, introduce interfaces
- [ ] **Phase 2: State Elimination** - Create App struct, eliminate global variables, wire context-based signal handling
- [ ] **Phase 3: Entry Point & Token** - Create thin cmd/ entry point, externalize API token with precedence chain
- [ ] **Phase 4: Build & Verification** - Update Makefile/CI/GoReleaser/README, upgrade dependencies, verify behavior preservation

## Phase Details

### Phase 1: Package Extraction
**Goal**: All domain logic lives in well-defined internal packages with exported types and clean interfaces
**Depends on**: Nothing (first phase)
**Requirements**: MOD-01, MOD-02, LAYOUT-02, LAYOUT-03, LAYOUT-04, LAYOUT-05, LAYOUT-06, API-01, API-04, API-05
**Success Criteria** (what must be TRUE):
  1. `go.mod` shows module path `github.com/sydlexius/mxlrcsvc-go` and `go build ./...` succeeds
  2. Five internal packages exist (`models`, `musixmatch`, `lyrics`, `scanner`, `app`) with exported types and constructor functions
  3. `Fetcher` interface is defined in `internal/musixmatch` and `Client` implements it
  4. No `log.Fatal` calls exist in any `internal/` package -- all functions return errors
  5. `slices.Contains` replaces `isInArray` and slugify regex is compiled once at package level
**Plans:** 3 plans

Plans:
- [x] 01-01-PLAN.md — Foundation: module rename + internal/models + internal/app
- [x] 01-02-PLAN.md — Domain packages: internal/musixmatch + internal/lyrics + internal/scanner
- [x] 01-03-PLAN.md — Wire main.go to internal packages, delete old files, migrate tests

### Phase 2: State Elimination
**Goal**: Global mutable state is eliminated and all processing state is owned by the App struct
**Depends on**: Phase 1
**Requirements**: STATE-01, STATE-02, STATE-03
**Success Criteria** (what must be TRUE):
  1. No package-level mutable variables exist in any source file
  2. `App` struct owns input queue, failed queue, and orchestrates the processing loop via its `Run` method
  3. Signal handler uses `context.Context` cancellation -- no goroutine directly accesses queue state
**Plans:** 1 plan

Plans:
- [ ] 02-01-PLAN.md — Create App struct with Run(ctx), move processing loop + timer + failed handling into internal/app, rewrite main.go as thin entry point with signal.NotifyContext

### Phase 3: Entry Point & Token
**Goal**: A thin entry point at `cmd/mxlrcsvc-go/main.go` constructs dependencies and runs the app, with the API token loaded from the correct precedence chain
**Depends on**: Phase 2
**Requirements**: LAYOUT-01, API-02, API-03, BUILD-07
**Success Criteria** (what must be TRUE):
  1. `cmd/mxlrcsvc-go/main.go` exists as a thin wrapper (parse args, load .env, construct deps, call `App.Run`)
  2. Token is loaded with correct precedence: CLI `--token` flag > `MUSIXMATCH_TOKEN` env var > `.env` file value
  3. No hardcoded default token exists anywhere in the source code
  4. `go run ./cmd/mxlrcsvc-go` launches the tool successfully
**Plans**: TBD

### Phase 4: Build & Verification
**Goal**: All build tooling produces the correct binary name from the correct paths, dependencies are current, and all three input modes work identically to the original
**Depends on**: Phase 3
**Requirements**: BUILD-01, BUILD-02, BUILD-03, BUILD-04, BUILD-05, BUILD-06, BUILD-08
**Success Criteria** (what must be TRUE):
  1. `make build` produces a binary named `mxlrcsvc-go` built from `cmd/mxlrcsvc-go/`
  2. CI workflows and GoReleaser config reference the new build path and binary name
  3. All three input modes (CLI pairs, text file, directory scan) produce identical output to the pre-restructuring baseline
  4. `go.mod` shows Go 1.24 minimum and all dependencies are at target versions (go-arg v1.6.1, fastjson v1.6.10, x/text latest, dhowden/tag latest)
  5. README documents the new module path, binary name, and token configuration
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Package Extraction | 0/3 | Planned | - |
| 2. State Elimination | 0/0 | Not started | - |
| 3. Entry Point & Token | 0/0 | Not started | - |
| 4. Build & Verification | 0/0 | Not started | - |

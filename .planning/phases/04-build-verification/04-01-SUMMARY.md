---
phase: 04-build-verification
plan: "01"
subsystem: build-tooling
tags: [makefile, goreleaser, ci, binary-name, build-path]
dependency_graph:
  requires: []
  provides: [build-tooling-updated]
  affects: [ci-pipeline, release-pipeline]
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - Makefile
    - .goreleaser.yml
    - .github/workflows/ci.yml
decisions:
  - "Binary name mxlrcgo-svc and build path ./cmd/mxlrcgo-svc applied consistently across all three build configs"
metrics:
  duration: "~5min"
  completed: "2026-04-11"
  tasks_completed: 3
  files_modified: 3
---

# Phase 4 Plan 1: Build Tooling Updates Summary

**One-liner:** Updated Makefile, GoReleaser, and CI to build `mxlrcgo-svc` binary from `./cmd/mxlrcgo-svc` entry point.

## What Was Built

Three build configuration files updated to consistently reference the new binary name (`mxlrcgo-svc`) and new build path (`./cmd/mxlrcgo-svc`) after the Phase 1-3 restructuring moved the entry point from root to `cmd/mxlrcgo-svc/`.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Update Makefile | 5518532 | Makefile |
| 2 | Update GoReleaser config | fb027e4 | .goreleaser.yml |
| 3 | Update CI workflow build step | a550b5a | .github/workflows/ci.yml |

## Changes Made

### Makefile
- `BINARY=mxlrc-go` → `BINARY=mxlrcgo-svc`
- `go build -o $(BINARY) .` → `go build -o $(BINARY) ./cmd/mxlrcgo-svc`

### .goreleaser.yml
- `id: mxlrc-go` → `id: mxlrcgo-svc`
- `main: .` → `main: ./cmd/mxlrcgo-svc`
- `binary: mxlrc-go` → `binary: mxlrcgo-svc`
- `release.github.name: mxlrc-go` → `release.github.name: mxlrcgo-svc`

### .github/workflows/ci.yml
- `go build -ldflags="-s -w" -o mxlrc-go .` → `go build -ldflags="-s -w" -o mxlrcgo-svc ./cmd/mxlrcgo-svc`

## Verification Results

```
make build → produces ./mxlrcgo-svc binary ✓
make clean → removes ./mxlrcgo-svc ✓
.goreleaser.yml → 4 occurrences of mxlrcgo-svc ✓
CI workflow → correct build path ✓
go build ./... → module builds clean ✓
```

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None — config file changes only, no new network endpoints or auth paths.

## Self-Check: PASSED

- Makefile exists with BINARY=mxlrcgo-svc ✓
- .goreleaser.yml has id/binary/main/release.name set to mxlrcgo-svc ✓
- ci.yml has correct build command ✓
- Commits 5518532, fb027e4, a550b5a exist in git log ✓

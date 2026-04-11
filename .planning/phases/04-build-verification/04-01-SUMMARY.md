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
  - "Binary name mxlrcsvc-go and build path ./cmd/mxlrcsvc-go applied consistently across all three build configs"
metrics:
  duration: "~5min"
  completed: "2026-04-11"
  tasks_completed: 3
  files_modified: 3
---

# Phase 4 Plan 1: Build Tooling Updates Summary

**One-liner:** Updated Makefile, GoReleaser, and CI to build `mxlrcsvc-go` binary from `./cmd/mxlrcsvc-go` entry point.

## What Was Built

Three build configuration files updated to consistently reference the new binary name (`mxlrcsvc-go`) and new build path (`./cmd/mxlrcsvc-go`) after the Phase 1-3 restructuring moved the entry point from root to `cmd/mxlrcsvc-go/`.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Update Makefile | 5518532 | Makefile |
| 2 | Update GoReleaser config | fb027e4 | .goreleaser.yml |
| 3 | Update CI workflow build step | a550b5a | .github/workflows/ci.yml |

## Changes Made

### Makefile
- `BINARY=mxlrc-go` → `BINARY=mxlrcsvc-go`
- `go build -o $(BINARY) .` → `go build -o $(BINARY) ./cmd/mxlrcsvc-go`

### .goreleaser.yml
- `id: mxlrc-go` → `id: mxlrcsvc-go`
- `main: .` → `main: ./cmd/mxlrcsvc-go`
- `binary: mxlrc-go` → `binary: mxlrcsvc-go`
- `release.github.name: mxlrc-go` → `release.github.name: mxlrcsvc-go`

### .github/workflows/ci.yml
- `go build -ldflags="-s -w" -o mxlrc-go .` → `go build -ldflags="-s -w" -o mxlrcsvc-go ./cmd/mxlrcsvc-go`

## Verification Results

```
make build → produces ./mxlrcsvc-go binary ✓
make clean → removes ./mxlrcsvc-go ✓
.goreleaser.yml → 4 occurrences of mxlrcsvc-go ✓
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

- Makefile exists with BINARY=mxlrcsvc-go ✓
- .goreleaser.yml has id/binary/main/release.name set to mxlrcsvc-go ✓
- ci.yml has correct build command ✓
- Commits 5518532, fb027e4, a550b5a exist in git log ✓

---
phase: 04-build-verification
plan: "02"
subsystem: go-dependencies
tags: [go-version, dependencies, go-mod, upgrade]
dependency_graph:
  requires: []
  provides: [dependencies-upgraded]
  affects: [build, test-suite]
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - go.mod
    - go.sum
decisions:
  - "Go directive bumped to 1.25.0 (not 1.24 as planned) because x/text v0.36.0 requires Go 1.25 — accepted as correct behavior of go toolchain"
  - "go-scalar indirect dep also upgraded from v1.1.0 to v1.2.0 as transitive requirement of go-arg v1.6.1"
metrics:
  duration: "~3min"
  completed: "2026-04-11"
  tasks_completed: 2
  files_modified: 2
---

# Phase 4 Plan 2: Go Version + Dependency Upgrades Summary

**One-liner:** Bumped Go to 1.25.0 and upgraded all four direct dependencies (go-arg v1.6.1, fastjson v1.6.10, x/text v0.36.0, dhowden/tag 20240417) with all tests passing.

## What Was Built

Updated `go.mod` and `go.sum` with the latest stable versions of all direct dependencies. The Go minimum version was bumped from 1.22 to 1.25.0 — slightly higher than the planned 1.24 because `x/text v0.36.0` requires Go 1.25.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Bump Go minimum version to 1.24 | b2c009a | go.mod |
| 2 | Upgrade direct dependencies to target versions | 2a5c6b0 | go.mod, go.sum |

## Final go.mod State

```
module github.com/sydlexius/mxlrcgo-svc

go 1.25.0

require (
    github.com/alexflint/go-arg v1.6.1
    github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
    github.com/joho/godotenv v1.5.1
    github.com/valyala/fastjson v1.6.10
    golang.org/x/text v0.36.0
)

require github.com/alexflint/go-scalar v1.2.0 // indirect
```

## Verification Results

```
go.mod shows go 1.25.0 ✓
go-arg v1.6.1 ✓
fastjson v1.6.10 ✓
x/text v0.36.0 (> v0.3.8) ✓
dhowden/tag 20240417 (> 20220618) ✓
go build ./... ✓
go test ./... ✓ (1 package with tests: internal/lyrics)
go vet ./... ✓
go mod tidy → no changes ✓
```

## Deviations from Plan

### Auto-accepted: Go directive 1.24 → 1.25.0

**Found during:** Task 2 (upgrading x/text)
**Issue:** `golang.org/x/text@latest` (v0.36.0) declares a minimum Go requirement of 1.25. The `go get` command automatically updated the `go` directive in go.mod from `1.24` to `1.25.0`.
**Fix:** Accepted — Go 1.26.2 is installed, the build passes, and 1.25.0 is a correct minimum for the dependency set chosen. The plan's intent was "current stable minimum," which 1.25.0 satisfies.
**Files modified:** go.mod
**Commit:** 2a5c6b0

### Auto-accepted: go-scalar v1.1.0 → v1.2.0

**Found during:** Task 2 (upgrading go-arg)
**Issue:** `go-arg v1.6.1` requires `go-scalar v1.2.0` (was v1.1.0 as indirect dep).
**Fix:** Accepted — transitive dependency upgrade required by go-arg; fully backward compatible.
**Files modified:** go.mod, go.sum
**Commit:** 2a5c6b0

## Known Stubs

None.

## Threat Flags

None — checksums verified by Go toolchain against sum.golang.org (T-04-02-01 mitigated).

## Self-Check: PASSED

- go.mod exists with go 1.25.0 ✓
- go-arg v1.6.1 in go.mod ✓
- fastjson v1.6.10 in go.mod ✓
- x/text v0.36.0 in go.mod ✓
- dhowden/tag 20240417 in go.mod ✓
- Commits b2c009a, 2a5c6b0 exist in git log ✓

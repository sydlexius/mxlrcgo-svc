---
phase: 04-build-verification
plan: "03"
subsystem: documentation
tags: [readme, docs, smoke-test, checkpoint]
dependency_graph:
  requires: [build-tooling-updated, dependencies-upgraded]
  provides: [docs-updated, phase-4-complete]
  affects: [user-documentation]
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - README.md
decisions:
  - "README Token Configuration section added as new section after How to get the Musixmatch Token, documenting all three supply methods with priority order"
metrics:
  duration: "~3min (Task 1 only; Task 2 awaiting human smoke test)"
  completed: "2026-04-11 (pending checkpoint)"
  tasks_completed: 1
  files_modified: 1
---

# Phase 4 Plan 3: README Update + Smoke Test Summary

**One-liner:** Updated README to remove upstream fork references, use new module path/binary name throughout, and document token configuration — binary builds and no-token error path confirmed working.

## What Was Built

README rewritten to accurately reflect the restructured project identity:
- Badge points to `sydlexius/mxlrcsvc-go` CI workflow
- Install path: `go install github.com/sydlexius/mxlrcsvc-go/cmd/mxlrcsvc-go@latest`
- Go version requirement updated to 1.24+
- All command examples use `mxlrcsvc-go`
- New Token Configuration section documents 3 supply methods with priority order

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Rewrite README for new module identity | 0f46d6d | README.md |
| 2 | Smoke test (checkpoint) | awaiting human | — |

## Pre-Checkpoint Automated Verification

```
make build → produces ./mxlrcsvc-go binary ✓
./mxlrcsvc-go --help → shows "mxlrcsvc-go" in usage ✓
./mxlrcsvc-go "adele,hello" → clean error: "no API token provided: use --token flag, MUSIXMATCH_TOKEN env var, or .env file" ✓
go test ./... → all pass (internal/lyrics: ok) ✓
```

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

T-04-03-01 (README token docs) — mitigated. README explains token is required but does NOT show example token values; users supply their own via env var or .env file.

## Self-Check: PASSED

- README.md exists with sydlexius/mxlrcsvc-go references ✓
- No fashni/mxlrc-go references in README ✓
- MUSIXMATCH_TOKEN documented in README ✓
- Commit 0f46d6d exists in git log ✓
- Binary ./mxlrcsvc-go built successfully ✓
- No-token error path returns structured error message ✓

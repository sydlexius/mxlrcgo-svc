---
phase: quick
plan: 260411-p8r
subsystem: normalize
tags: [test, refactor, pointer-semantics]
dependency_graph:
  requires: []
  provides: [issue-36-fix]
  affects: [internal/normalize/normalize_test.go]
tech_stack:
  added: []
  patterns: [nil-pointer-as-sentinel]
key_files:
  modified:
    - internal/normalize/normalize_test.go
decisions:
  - Use *float64 with nil-as-skip instead of sentinel -1 to eliminate silent short-circuit of range checks
metrics:
  duration: ~1min
  completed: 2026-04-12
  tasks_completed: 1
  files_modified: 1
requirements_closed: [issue-36]
---

# Quick Task 260411-p8r: Replace wantEq float64 sentinel with *float64 pointer in TestMatchConfidence

**One-liner:** Replaced `wantEq float64` sentinel (-1) with `*float64` nil-as-skip pointer, eliminating silent dead-code paths in range-only test cases.

## What Was Done

Refactored `TestMatchConfidence` in `internal/normalize/normalize_test.go` to use idiomatic Go pointer semantics instead of a magic sentinel value.

**Root problem:** The old guard `if tc.wantEq >= 0` triggered for all test cases including those where `wantEq` was intentionally omitted (zero-valued float64 = `0.0 >= 0`). This caused range-only test cases (`near match transposition`, `completely different`) to silently assert `got == 0.0` and `return` early, making `wantGt`/`wantLt` checks dead code.

**Changes made:**
1. Added `func eqF(f float64) *float64 { return &f }` helper before `TestMatchConfidence`
2. Changed field declaration from `wantEq float64` to `wantEq *float64`
3. Updated 5 table entries to use `eqF(...)`: identical, both empty, one empty, case insensitive, accent insensitive
4. Removed `wantEq: -1` from 2 range-only entries: near match transposition, completely different
5. Updated guard from `if tc.wantEq >= 0` to `if tc.wantEq != nil` with `*tc.wantEq` dereference

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | `8515549` | test(quick-260411-p8r): replace wantEq sentinel with *float64 pointer |

## Verification

```
go test ./internal/normalize/... -v -run TestMatchConfidence
# All 7 subtests: PASS

go test ./internal/normalize/...
# Full package: PASS (16 tests)

go vet ./internal/normalize/...
# Clean

grep "wantEq: -1\|wantEq >= 0\|wantEq float64" internal/normalize/normalize_test.go
# No matches (sentinel patterns eliminated)
```

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None. All changes are in `_test.go`; no production attack surface touched.

## Self-Check: PASSED

- [x] `internal/normalize/normalize_test.go` modified correctly
- [x] Commit `8515549` exists in git log
- [x] No sentinel patterns remain in the file
- [x] All pointer patterns present: `wantEq *float64`, `eqF`, `wantEq != nil`
- [x] `go test ./internal/normalize/...` passes (16/16)
- [x] `go vet ./internal/normalize/...` clean

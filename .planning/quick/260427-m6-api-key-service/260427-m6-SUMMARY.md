# Summary: Issue #21 API Key Service

## Completed

- Added `internal/auth` with API key generation, SHA-256 hashing, validation, revocation, listing, and scope helpers.
- Generated raw keys use the `mxlrc_` prefix with 32 bytes of `crypto/rand` entropy hex-encoded after the prefix.
- Added scope handling for `webhook` and `admin`, with `admin` implying all scopes.
- Added an in-memory `Store` implementation behind a persistence interface for later DB/server integration.
- Added focused tests for generation, hashing, validation, admin implication, revocation, invalid scopes, and duplicate hashes.

## Verification

```bash
go test ./internal/auth
go test ./...
golangci-lint run
git diff --check
```

All checks passed.

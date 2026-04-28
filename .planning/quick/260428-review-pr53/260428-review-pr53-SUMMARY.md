# PR 53 Review Fixes Summary

## Outcome

Resolved the PR 53 CodeRabbit findings and the failed CodeQL status finding in one pass.

## Changes

- Replaced plain SHA-256 API key hashing with PBKDF2-SHA256.
- Added HTTP server read, write, and idle timeouts.
- Trimmed and validated configured webhook API keys before registering them.
- Made Bearer auth scheme parsing case-insensitive.
- Enforced `CGO_ENABLED=0` in `make build`.
- Added README language tags to fenced code blocks.
- Extended queue cleanup tests for failed and done rows.

## Verification

- `go test -count=1 ./cmd/mxlrcgo-svc ./internal/auth ./internal/server ./internal/queue`
- `go test -count=1 ./...`
- `golangci-lint run ./...`
- `git diff --check`

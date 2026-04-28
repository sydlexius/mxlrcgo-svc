---
phase: quick
plan: 260428-review-pr53
type: execute
autonomous: true
requirements:
  - pr-53-review
files_modified:
  - cmd/mxlrcgo-svc/main.go
  - internal/auth/service.go
  - internal/auth/service_test.go
  - internal/server/server.go
  - internal/server/server_test.go
  - internal/queue/queue_test.go
  - Makefile
  - README.md
---

<objective>
Resolve PR 53 CodeRabbit review comments and the failed CodeQL status in one pass.
</objective>

<tasks>

<task type="auto">
  <name>Fix security and server review findings</name>
  <done>
    - API key hashes no longer use plain SHA-256
    - HTTP server has read, write, and idle timeouts
    - Webhook keys are trimmed and format-validated before registration
    - Bearer authorization scheme parsing is case-insensitive
  </done>
</task>

<task type="auto">
  <name>Fix build, docs, and test coverage findings</name>
  <done>
    - Make build disables CGO
    - README fenced blocks have language tags
    - Queue cleanup tests cover pending, failed, processing, and done states
  </done>
</task>

<task type="auto">
  <name>Verify and respond</name>
  <done>
    - gofmt applied
    - go test ./... passes
    - golangci-lint run ./... passes when available
    - One commit is pushed and review threads are replied to
  </done>
</task>

</tasks>

<verification>
```bash
go test -count=1 ./...
golangci-lint run ./...
```
</verification>

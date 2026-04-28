---
phase: quick
plan: 260428-review-pr53-followup
type: execute
autonomous: true
requirements:
  - pr-53-review-followup
files_modified:
  - cmd/mxlrcgo-svc/main.go
  - cmd/mxlrcgo-svc/main_test.go
  - internal/auth/service.go
  - internal/auth/service_test.go
  - internal/auth/webhook.go
  - internal/auth/webhook_test.go
  - internal/server/server.go
  - internal/server/server_test.go
  - README.md
---

<objective>
Resolve the follow-up PR 53 CodeRabbit findings after commit 070748a.
</objective>

<tasks>

<task type="auto">
  <name>Auth cleanup</name>
  <done>
    - Webhook auth bootstrap lives under internal/auth
    - HashKey returns an error and callers handle it before slicing
    - PBKDF2 salt is immutable
  </done>
</task>

<task type="auto">
  <name>Server behavior and docs</name>
  <done>
    - Lidarr auth maps backend failures to HTTP 500
    - README documents new server flags and webhook configuration
  </done>
</task>

<task type="auto">
  <name>Verification</name>
  <done>
    - gofmt applied
    - go test ./... passes
    - golangci-lint run ./... passes
  </done>
</task>

</tasks>

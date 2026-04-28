---
phase: quick
plan: 260428-review-pr53-followup2
type: execute
autonomous: true
requirements:
  - pr-53-review-followup2
files_modified:
  - cmd/mxlrcgo-svc/main.go
  - internal/auth/webhook.go
  - internal/auth/webhook_test.go
  - internal/server/server.go
  - README.md
---

<objective>
Resolve the second follow-up PR 53 CodeRabbit findings after commit adde657.
</objective>

<tasks>

<task type="auto">
  <name>Small behavior and docs corrections</name>
  <done>
    - CLI help mentions artist/title, .txt, and directory inputs
    - README blockquote no longer violates MD028
    - Webhook key bootstrap receives startup context
    - Webhook enqueue and cleanup 500 paths log underlying errors
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

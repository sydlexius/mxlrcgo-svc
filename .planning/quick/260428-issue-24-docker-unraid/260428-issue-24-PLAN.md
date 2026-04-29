---
phase: quick
plan: 260428-issue-24
type: execute
files_modified:
  - Dockerfile
  - .dockerignore
  - build/docker/entrypoint.sh
  - docker-compose.example.yml
  - unraid/mxlrcgo-svc.xml
  - internal/config/
  - README.md
  - .planning/STATE.md
autonomous: true
requirements:
  - issue-24
must_haves:
  truths:
    - "Dockerfile uses a multi-stage CGO_ENABLED=0 Go build and an Alpine runtime"
    - ".dockerignore excludes local/build/test artifacts without hiding required source files"
    - "Compose example exposes host/container port 50705 and maps /config and /music"
    - "Unraid template follows the local unraid-templates examples"
    - "MXLRC_DOCKER=true makes default config/data paths resolve under /config"
    - "Focused config tests and go test ./... pass"
---

<objective>
Implement issue #24: Dockerfile, compose example, Unraid Community Applications
template, and explicit Docker storage path detection.
</objective>

<tasks>

<task type="auto">
  <name>Task 1: Docker storage defaults</name>
  <files>internal/config/</files>
  <done>
    - MXLRC_DOCKER=true forces default config and DB paths into /config
    - Existing /.dockerenv fallback behavior remains available
    - Tests cover explicit Docker mode
  </done>
</task>

<task type="auto">
  <name>Task 2: Container assets</name>
  <files>Dockerfile, .dockerignore, docker-compose.example.yml, unraid/mxlrcgo-svc.xml</files>
  <done>
    - Multi-stage Dockerfile builds static Linux binary with CGO disabled
    - Compose example runs serve mode on port 50705 with /config and /music mounts
    - Unraid XML follows the example template conventions from ~/Developer/unraid-templates
  </done>
</task>

<task type="auto">
  <name>Task 3: Documentation and verification</name>
  <files>README.md, all touched files</files>
  <done>
    - README documents Docker usage
    - gofmt applied
    - go test ./... passes
  </done>
</task>

</tasks>

<verification>
```bash
gofmt -w internal/config
go test ./...
```
</verification>

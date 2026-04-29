---
phase: quick
plan: 260429-issue-25
type: execute
files_modified:
  - internal/config/
  - internal/providers/
  - internal/verification/
  - internal/musixmatch/
  - internal/commands/
  - cmd/mxlrcgo-svc/
  - internal/queue/
  - internal/scan/
  - internal/scanner/
  - internal/worker/
  - internal/db/migrations/
  - config.example.toml
  - .planning/STATE.md
autonomous: true
requirements:
  - issue-25
must_haves:
  truths:
    - "Musixmatch remains the default and only active lyrics provider"
    - "Provider selection is configurable without changing existing token precedence or CLI behavior"
    - "Unsupported or disabled providers fail clearly before fetch/serve starts"
    - "Verification config is parsed and inspectable, but no STT sidecar is invoked yet"
    - "Worker can optionally call a Whisper-compatible HTTP sidecar for low-confidence scanned audio"
    - "Focused tests and go test ./... pass"
---

<objective>
Implement issue #25 in slices: introduce a lyrics provider abstraction,
wire Musixmatch through it, add verification/provider config, and add optional
low-confidence STT verification for scanned audio.
</objective>

<tasks>

<task type="auto">
  <name>Task 1: Provider abstraction</name>
  <files>internal/providers/, internal/musixmatch/, internal/commands/, cmd/mxlrcgo-svc/</files>
  <done>
    - LyricsProvider interface exposes Name and FindLyrics
    - Musixmatch client satisfies the provider contract
    - Commands select the configured provider before constructing app/worker dependencies
  </done>
</task>

<task type="auto">
  <name>Task 2: Config surface</name>
  <files>internal/config/, internal/commands/, config.example.toml</files>
  <done>
    - providers.primary defaults to musixmatch
    - providers.disabled can disable named providers
    - verification fields parse with safe defaults and config command visibility
  </done>
</task>

<task type="auto">
  <name>Task 4: Optional STT verification</name>
  <files>internal/verification/, internal/worker/, internal/queue/, internal/scan/, internal/scanner/, internal/db/migrations/</files>
  <done>
    - Scanned audio source paths survive enqueue/dequeue
    - Worker invokes verification only when enabled, source path is known, and metadata confidence is low
    - Whisper-compatible HTTP transcription results are compared against candidate lyrics
  </done>
</task>

<task type="auto">
  <name>Task 3: Verification</name>
  <files>all touched files</files>
  <done>
    - gofmt applied
    - go test ./... passes
  </done>
</task>

</tasks>

<verification>
```bash
gofmt -w cmd/mxlrcgo-svc internal/config internal/commands internal/musixmatch internal/providers
go test ./...
```
</verification>

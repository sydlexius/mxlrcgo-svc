# Summary: Issue #25 Provider Abstraction Slice

## Completed

- Added `internal/providers` with a named `LyricsProvider` abstraction and provider selection helper.
- Kept Musixmatch as the default and only active provider, with disabled/unsupported provider failures before app or worker startup.
- Added provider config:
  - `providers.primary`
  - `providers.disabled`
  - `MXLRC_PROVIDER_PRIMARY`
  - `MXLRC_PROVIDERS_DISABLED`
- Added verification config:
  - `verification.enabled`
  - `verification.whisper_url`
  - `verification.sample_duration_seconds`
  - `verification.min_confidence`
  - `verification.min_similarity`
  - matching environment overrides
- Added `internal/verification` with a Whisper-compatible HTTP transcription client and transcript/lyrics similarity scoring.
- Preserved scanned audio source paths through scanner, scan enqueuer, durable queue, and a new DB migration.
- Wired the worker to run verification only for non-cached, low-confidence scanned results when verification is enabled.
- Moved cache storage until after verification accepts or is skipped, so rejected lyrics are not cached.
- Exposed the new keys through `mxlrcgo-svc config get|set|list`.
- Updated `config.example.toml` and `README.md`.
- Opened follow-up issue #57 for ffmpeg-based audio sampling before Whisper upload.

## Verification

```bash
gofmt -w cmd/mxlrcgo-svc internal/config internal/commands internal/models internal/scanner internal/scan internal/queue internal/worker internal/verification
go test ./...
golangci-lint run
git diff --check
```

All checks passed.

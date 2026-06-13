# Per-library settings: recording enrichment + instrumental detection

Status: design of record (draft)
Date: 2026-06-12
Related: v1.6.0 recording enrichment (#191), instrumental detection sidecar (#187)

## Summary

Make two v1.6.0 behaviors controllable per library, with a global default and a
per-run CLI override:

1. **Recording enrichment** (ISRC / MusicBrainz recording ID / duration read from
   audio tags and fed to the matcher) - currently unconditional.
2. **Instrumental detection** (the optional audio-classifier sidecar) - currently
   gated by a single global config flag.

Resolution precedence (highest wins): **CLI flag > per-library setting > global
config default > built-in default.**

## Motivation

Operators run multiple libraries with different economics. The driving need is
**cost/load control**: instrumental detection samples audio with ffmpeg and calls
an inference sidecar per track, which is worth running on a small curated or
soundtrack-heavy library but wasteful on a large bulk library. Recording
enrichment is cheap, but the same per-library control should exist for it as a
matter of consistent, predictable configuration ("expose the knobs").

A single global flag cannot express "detect instrumentals on library A but not
library B."

## Goals

- Persistent per-library on/off for recording enrichment and instrumental detection.
- A global config default for each (preserving current behavior when unset).
- A per-run CLI override on `scan`.
- One shared resolution rule and one shared per-library settings surface, so adding
  a third knob later is mechanical.

## Non-goals

- No change to the enrichment extraction logic or the detector implementation.
- No tag-quality or content-type heuristics (manual control only).
- No per-library settings for unrelated config (verification, providers, output).
  Only these two columns are added now (YAGNI); the infrastructure stays extensible.

## Current state (as-is)

- `libraries` table: `id, path, name, created_at, updated_at`. No per-feature columns.
- Scanner (`internal/scanner/scanner.go`): always calls `extractISRC` /
  `extractRecordingMBID` and reads duration into `models.Track`.
- Instrumental detection: `config.InstrumentalDetector.Enabled` (global, default
  false). The detector client is built in `newAudioDetector` (`commands.go`), wired
  onto the worker via `EnableAudioDetector`; the worker calls `audioDetector.Detect`
  at fetch time (`worker.go`).
- Work queue: rows link to `scan_results` (which carries `library_id`) through the
  `work_queue_scan_results` junction. `WorkItem.ProvidersVersion` is the existing
  precedent for a value **resolved and stamped onto the row at enqueue time** and
  read back by the worker later.

## Design

### Component 1 - Foundation: per-library settings infrastructure

Lands first; the two features build on it.

- **Migration** (additive `ALTER TABLE ... ADD COLUMN`, no table rebuild): add two
  nullable tri-state columns to `libraries`:
  - `enrich_recording INTEGER` - `NULL` = inherit global default, `0` = off, `1` = on
  - `detect_instrumental INTEGER` - `NULL` = inherit, `0` = off, `1` = on
  Nullable so "unset/inherit" is distinct from an explicit "off".
- **`models.Library`**: add `EnrichRecording *bool` and `DetectInstrumental *bool`
  (pointer = tri-state).
- **`internal/library` repository**: `Add` / `Update` write the columns; `List` /
  `Get` / `GetByName` return them. `Update` leaves a column unchanged when its flag
  is absent.
- **Resolver** (small helper, e.g. in `internal/config`):
  `ResolveBool(cli *bool, lib *bool, globalDefault bool) bool` - returns the first
  non-nil of `cli`, then `lib`, else `globalDefault`. The built-in default folds
  into the global default.
- **Library CLI** (`library add` / `library update`): tri-state `*bool` flags
  `--enrich` / `--enrich=false` and `--detect-instrumental` /
  `--detect-instrumental=false`. Absent on `add` -> `NULL` (inherit); absent on
  `update` -> leave unchanged.

### Component 2 - Recording enrichment per-library (scan-time)

- **Global default**: a config field defaulting to `true` (preserves current
  always-on behavior), with an env override. Built-in default `true`.
- **Scanner**: for the library being scanned, resolve
  `enrich = ResolveBool(cliOverride, lib.EnrichRecording, globalDefault)`. When
  `false`, skip ISRC, MBID, and duration extraction (treat the whole #191
  enrichment unit as one switch). The cache then keys those tracks at
  `duration_bucket = 0`, which is the existing fallback - no behavior regression.
- **`scan` CLI**: `--enrich` / `--no-enrich` (per-run override).
- No work-queue change; enrichment is entirely scan-time.

Decision: enrichment-off disables duration extraction too (not just ISRC/MBID),
keeping "recording enrichment" a single user-facing concept. This intersects the
separate lyrics-cache duration-bucket gap (the worker still hardcodes bucket 0);
that gap is tracked independently and does not block this work.

### Component 3 - Instrumental detection per-library + CLI (stamp-at-enqueue)

Mirrors the `ProvidersVersion` resolve-and-stamp pattern so that
`--detect-instrumental` on `scan` is meaningful even though `scan` only enqueues
and the worker does the detecting later.

- **Global default**: `config.InstrumentalDetector.Enabled` stays the global
  default (already exists, default false).
- **Migration**: add nullable `detect_instrumental INTEGER` to `work_queue`.
  `NULL` = "no decision stamped; fall back to the global setting at worker time"
  (covers all pre-existing rows, preserving current behavior). `0` / `1` = explicit
  resolved decision.
- **`WorkItem`**: add `DetectInstrumental *bool`.
- **Enqueue**: resolve
  `detect = ResolveBool(cliOverride, lib.DetectInstrumental, globalDefault)` and
  stamp `0`/`1` on initial insert. Like `ProvidersVersion`, the value is written
  on initial insert only; `ON CONFLICT` refresh, `Defer`, and `RecheckDeferred`
  leave it unchanged. A per-library setting change therefore takes effect for a
  given track only when that track is next freshly enqueued (documented limitation;
  a restamp/recheck path is out of scope for v1).
- **Detector construction**: decouple from the global `Enabled` flag - build the
  detector client whenever `classifier_url` is configured, so per-library detection
  works even with global `Enabled = false`. The worker gates the `Detect` call on
  the resolved per-item flag.
- **`scan` CLI**: `--detect-instrumental` / `--no-detect-instrumental` (per-run
  override, stamped onto the rows enqueued by that run).

### Data flow

1. `library add/update` writes the tri-state `libraries` columns.
2. `scan` (optionally with CLI overrides): per library, resolve enrichment
   (gates tag extraction, scan-time) and resolve detection (stamped onto each
   enqueued `work_queue` row).
3. Worker: reads the per-item `DetectInstrumental` flag (or the global default when
   `NULL`) and runs `Detect` accordingly.

## Error handling

- If a work item requests detection but no `classifier_url` is configured, the
  worker logs an error and proceeds without detection - loud, never a silent no-op
  (per the repo's no-silent-failure rule).
- Tri-state CLI flags reject invalid values via go-arg parsing.
- Migrations are additive `ADD COLUMN`s (safe in SQLite without a table rebuild).

## Testing

- **Foundation**: repository round-trip for the tri-state columns (`NULL`/`0`/`1`);
  resolver precedence unit tests; library CLI flag tests.
- **Enrichment**: scanner test that enrich-off skips ISRC/MBID/duration and enrich-on
  extracts; precedence (CLI > library > global).
- **Detection**: enqueue stamps the resolved decision; worker honors the per-item
  flag; `NULL` falls back to the global default; loud-skip when the classifier is
  unconfigured. Integration tests against real SQLite (in-memory or temp file).
- Patch coverage >= 70% per lane.

## Decomposition / rollout

1. **Issue A - foundation** (blocks B and C): `libraries` migration + `models.Library`
   + repository + `ResolveBool` + library CLI tri-state flags.
2. **Issue B - enrichment per-library**: global default config field + scanner gating
   + `scan --enrich/--no-enrich`.
3. **Issue C - detection per-library + CLI**: `work_queue` migration + `WorkItem`
   field + enqueue stamping + worker per-item gating + detector-construction
   decoupling + `scan --detect-instrumental/--no-detect-instrumental`.

Foundation lands as its own small PR before the features, per the repo's
decompose-before-building rule.

## Open decisions (confirm before implementation)

- Enrichment-off disabling duration extraction (proposed: yes, as a unit). Confirmed
  acceptable because cache bucket-0 fallback already exists.
- Setting-change propagation to already-queued rows (proposed: stamp-on-insert only,
  matching `ProvidersVersion`; re-enqueue required for a change to take effect).

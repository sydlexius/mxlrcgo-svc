# Design: serve-mode Reports workspace (issue #186)

Status: approved 2026-06-10 (maintainer-reviewed high-level design)
Scope: design of record for the web observability surface in serve mode.
Implementation is tracked in separate issues (see "Decomposition").

## Problem

Serve mode runs as a long-lived daemon (Docker or native service) processing a
lyrics queue against rate-limited providers. Today its only observability is
four JSON endpoints and container logs. Operators need to answer questions like
"what is the queue doing", "which provider lanes are earning their keep", and
"what failed and why" without shelling into the box.

## Shape

A read-only web UI served by the existing serve-mode listener:

- Sidebar shell (ported from the stillwater M55 layout) with two nav items:
  **Reports** and **Config**.
- **Reports** is a two-pane workspace, adapted from stillwater's next-UI
  reports prototype (stillwater #1337): a left rail (~260px) lists canned
  reports with last-run timestamps; the right pane renders the selected
  report's results with a "Run now" action.
- **Config** shows the effective TOML configuration with secrets redacted.

Deliberate exclusions, per #186's guardrails and this design pass:

- No live tiles, no polling, no SSE. Every report runs on demand against
  SQLite and is stamped with its run time. The system moves on provider
  cooldowns; the UI is honest about that.
- No search or fetch affordances (no instant-result UX; Musixmatch is bursty).
- No prefs drawer or per-user preference machinery.
- No write operations of any kind.

## v1 report rail

Five canned reports, all queries over data the DB already holds. A report that
turns out to need a migration is a red flag to re-scope, not a reason to add
one.

1. **Queue summary** - counts by status (pending / processing / done /
   failed / deferred).
2. **Recent outcomes** - last N processed tracks: result (synced / unsynced /
   instrumental / miss), provider lane, timestamp.
3. **Provider effectiveness** - hits and misses per provider lane.
4. **Instrumental inventory** - tracks flagged instrumental, attributed to
   the flag source (provider data vs the optional audio detector, #187).
5. **Failure analysis** - failed and deferred tracks grouped by reason.

(A cache-performance report was considered and cut from v1.)

## Stack and brand

A direct port of the stillwater M55 system so both projects share one design
language:

- Templ + HTMX + Tailwind; all assets shipped via `go:embed` so the single
  binary remains the deliverable. No CGO. No node runtime dependency at run
  time (Tailwind builds at dev time).
- stillwater's `web/static/css/design-tokens.css` as the token source: dark
  default, blue-600 (`#2563eb`) primary / blue-500 (`#3b82f6`) accent.
- Two font families, under the three-family cap: Inter (UI text) and
  JetBrains Mono (code, IDs, timestamps), self-hosted.
- Reference implementation paths (stillwater repo): layout shell
  `web/templates/next/layout.templ`, sidebar `web/templates/next/sidebar.templ`,
  reports prototype per issue #1337, proto screenshot
  `m55-sidebar-dashboard-proto.png`.

## Exposure and auth

As locked in #186: served on the existing serve listener, localhost by
default; the webhook API-key gate covers remote exposure. The dashboard does
not grow its own auth - authenticated web access, onboarding, and TLS arrive
with #204 and supersede this section.

## Decomposition

Three implementation issues, each referencing this document:

1. **UI foundation** - templ/tailwind toolchain + Make targets, design-token
   port, sidebar shell, Config view, `go:embed` wiring. Lands first.
2. **Reports workspace** - two-pane layout, the five canned reports, and
   their read-only queries. Builds on the foundation.
3. **`/metrics` Prometheus endpoint** - counters/gauges over the same
   queue/provider queries; independent of the UI lanes.

## Testing

Each lane carries its own tests: token/shell templates get golden-render
tests, report queries get in-package tests against real SQLite (repo
convention: no mocks for storage), and `/metrics` gets a scrape-shape test.
Patch coverage follows the repo's 70% gate.

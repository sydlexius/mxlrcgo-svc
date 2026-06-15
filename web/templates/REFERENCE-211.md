# M55 design references for the Reports workspace (#211)

Captured from the live stillwater M55 instance (dark mode) during #210.
These are NOT implemented in #210 (foundation = shell + Config view + Reports
placeholder). They are the pinned visual reference for the #211 Reports lane,
which renders the five canned reports from the #186 design of record.

Maintainer note (2026-06-14): "the recent activity and action queue might be
good to copy."

- **Action Queue** (dashboard center column): per-track rows with avatar
  thumbnail, artist/title, a count badge, and per-row actions. Maps to #186
  report 2 "Recent outcomes" and report 1 "Queue summary".
- **Recent Activity** (dashboard right rail): compact event list with mono
  timestamps and an actor/line per entry. Maps to the activity surfacing in the
  Reports two-pane right pane.

Reference screenshots (repo-root, gitignored .playwright-mcp output):
`m55-ref-03-dashboard-dark-real.png`.

Live token table confirmed for the dark theme port (matches design-tokens.css):
sidebar-bg #0f172a, sidebar-text #94a3b8, active-bg rgba(59,130,246,0.15),
active-text #93c5fd, content-text #e2e8f0, accent #3b82f6, Inter + JetBrains Mono.

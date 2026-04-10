# Requirements: mxlrcsvc-go (M0: Fork & Restructure)

**Defined:** 2026-04-10
**Core Value:** The tool fetches synced lyrics reliably and writes correct `.lrc` files. Everything else exists to support that.

## v1 Requirements

Requirements for M0. Each maps to roadmap phases.

### Module Identity

- [ ] **MOD-01**: Go module path renamed to `github.com/sydlexius/mxlrcsvc-go` in go.mod
- [ ] **MOD-02**: All import paths updated to reflect new module path

### Project Layout

- [ ] **LAYOUT-01**: Entry point lives at `cmd/mxlrcsvc-go/main.go` as a thin wrapper (parse args, construct deps, call App.Run)
- [ ] **LAYOUT-02**: Internal packages created: `internal/models`, `internal/musixmatch`, `internal/lyrics`, `internal/scanner`, `internal/app`
- [ ] **LAYOUT-03**: All types and methods exported from internal packages (uppercase names)
- [ ] **LAYOUT-04**: Constructor functions (`NewClient`, `NewWriter`, etc.) for each internal package
- [ ] **LAYOUT-05**: Regex in slugify compiled once at package level instead of per-call
- [ ] **LAYOUT-06**: `isInArray` replaced with `slices.Contains` from stdlib (removes reflect dependency)

### State Management

- [ ] **STATE-01**: Global `inputs` and `failed` package-level variables eliminated
- [ ] **STATE-02**: App struct owns input queue, failed queue, and processing loop orchestration
- [ ] **STATE-03**: Signal handler uses context.Context cancellation instead of direct goroutine queue access

### API & Config

- [ ] **API-01**: `Fetcher` interface defined for Musixmatch client (`FindLyrics(Track) (Song, error)`)
- [ ] **API-02**: Musixmatch token loaded with precedence: CLI flag > environment variable (`MUSIXMATCH_TOKEN`) > `.env` file
- [ ] **API-03**: Hardcoded default token removed from source code
- [ ] **API-04**: `writeLRC` returns `error` instead of `bool`
- [ ] **API-05**: All `log.Fatal` calls in library code converted to error returns

### Build & Verification

- [ ] **BUILD-01**: Makefile updated for `cmd/mxlrcsvc-go/` build path and `mxlrcsvc-go` binary name
- [ ] **BUILD-02**: GoReleaser config updated for new binary name and main path
- [ ] **BUILD-03**: CI workflows updated for new build paths
- [ ] **BUILD-04**: README updated for new module path and binary name
- [ ] **BUILD-05**: All three input modes (CLI pairs, text file, directory scan) work identically after restructuring
- [ ] **BUILD-06**: Dependencies upgraded: go-arg to v1.6.1, fastjson to v1.6.10, x/text to latest, dhowden/tag to latest
- [ ] **BUILD-07**: godotenv v1.5.1 added for .env file loading
- [ ] **BUILD-08**: go.mod minimum Go version bumped to 1.24

## v2 Requirements

Deferred to future milestones. Tracked but not in M0 roadmap.

### Logging & Observability

- **LOG-01**: Structured logging via `log/slog` replacing `log` stdlib
- **LOG-02**: Configurable log levels (debug, info, warn, error)

### Testability

- **TEST-01**: Unit tests for each internal package
- **TEST-02**: Integration tests for all three input modes
- **TEST-03**: Automated behavior verification (not just manual smoke test)

### HTTP Client

- **HTTP-01**: Reusable HTTP client with connection pooling (instead of per-request client)
- **HTTP-02**: Configurable timeouts on HTTP client

### Error Handling

- **ERR-01**: Consistent error wrapping with `%w` across all packages
- **ERR-02**: Sentinel errors for known failure modes (rate limit, not found, auth failure)

### Context Propagation

- **CTX-01**: Full context propagation from entry point through all internal packages
- **CTX-02**: Request-scoped timeouts on API calls

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Switch to cobra/viper | go-arg handles single-command CLI with env var support. Cobra adds complexity for zero benefit. |
| `pkg/` directory | No public API. `internal/` provides compiler-enforced boundaries. |
| Config file support (YAML/TOML) | 7 CLI flags. Token via env/.env is sufficient. |
| Dependency injection framework | 5 packages. Manual wiring in main.go is clearer. |
| New test coverage | Explicitly deferred. Move existing tests only. |
| Concurrency / goroutine pool | Sequential processing with cooldown is intentional (rate limiting). |
| Plugin architecture | Single-purpose tool. Extension system is over-engineering. |
| `internal/app/` nesting | Single binary. Flat `internal/` packages preferred. |
| Go workspace (separate go.mod per package) | Single module. Workspaces are for multi-module repos. |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| MOD-01 | TBD | Pending |
| MOD-02 | TBD | Pending |
| LAYOUT-01 | TBD | Pending |
| LAYOUT-02 | TBD | Pending |
| LAYOUT-03 | TBD | Pending |
| LAYOUT-04 | TBD | Pending |
| LAYOUT-05 | TBD | Pending |
| LAYOUT-06 | TBD | Pending |
| STATE-01 | TBD | Pending |
| STATE-02 | TBD | Pending |
| STATE-03 | TBD | Pending |
| API-01 | TBD | Pending |
| API-02 | TBD | Pending |
| API-03 | TBD | Pending |
| API-04 | TBD | Pending |
| API-05 | TBD | Pending |
| BUILD-01 | TBD | Pending |
| BUILD-02 | TBD | Pending |
| BUILD-03 | TBD | Pending |
| BUILD-04 | TBD | Pending |
| BUILD-05 | TBD | Pending |
| BUILD-06 | TBD | Pending |
| BUILD-07 | TBD | Pending |
| BUILD-08 | TBD | Pending |

**Coverage:**
- v1 requirements: 24 total
- Mapped to phases: 0
- Unmapped: 24

---
*Requirements defined: 2026-04-10*
*Last updated: 2026-04-10 after initial definition*

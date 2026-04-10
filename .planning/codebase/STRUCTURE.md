# Codebase Structure

**Analysis Date:** 2026-04-10

## Directory Layout

```
mxlrc-go/
├── .claude/                # Claude Code configuration
│   ├── commands/           # Custom slash commands (new-issue, prep-pr, post-merge-cleanup)
│   ├── settings.json       # Shared Claude settings
│   └── settings.local.json # Local Claude settings (gitignored)
├── .githooks/              # Git hooks
│   └── pre-commit          # Pre-commit quality checks
├── .github/                # GitHub configuration
│   ├── workflows/          # CI/CD pipelines
│   │   ├── ci.yml          # Lint, test, build matrix
│   │   ├── codeql.yml      # Security scanning
│   │   ├── release.yml     # GoReleaser on version tags
│   │   ├── dependabot-auto-approve.yml
│   │   └── dependabot-merge.yml
│   └── dependabot.yml      # Dependency update config
├── .planning/              # Planning documents
│   └── codebase/           # Codebase analysis docs (this directory)
├── main.go                 # CLI entry point, orchestration loop
├── musixmatch.go           # Musixmatch API client
├── lyrics.go               # LRC file formatting and writing
├── structs.go              # All data types and InputsQueue
├── utils.go                # Input parsing, directory scanning, helpers
├── utils_test.go           # Tests for utility functions
├── go.mod                  # Go module definition
├── go.sum                  # Dependency checksums
├── Makefile                # Build, test, lint, format commands
├── CLAUDE.md               # Project guidance for AI assistants
├── README.md               # Project documentation
├── LICENSE                 # MIT license
├── .golangci.yml           # Linter configuration
├── .goreleaser.yml         # Cross-platform release builds
├── .pre-commit-config.yaml # Pre-commit framework hooks
├── .editorconfig           # Editor formatting rules
├── .gitattributes          # Git file attributes
├── .gitignore              # Ignored files/directories
├── .typos.toml             # Spell-checker config
└── .coderabbit.yml         # CodeRabbit review config
```

## Directory Purposes

**Root (`.`):**
- Purpose: All application source code lives here (flat structure, single `main` package)
- Contains: 5 Go source files, 1 test file, module files, config files
- Key files: `main.go`, `musixmatch.go`, `lyrics.go`, `structs.go`, `utils.go`

**`.github/workflows/`:**
- Purpose: CI/CD pipeline definitions
- Contains: 5 workflow YAML files
- Key files: `ci.yml` (lint/test/build), `release.yml` (GoReleaser)

**`.githooks/`:**
- Purpose: Local git hook scripts (installed via `make hooks`)
- Contains: `pre-commit` hook running typos, gofmt, go build, golangci-lint, govulncheck

**`.planning/codebase/`:**
- Purpose: Codebase analysis documents consumed by planning/execution agents
- Contains: Architecture, structure, conventions, concerns documentation
- Generated: Yes (by codebase mapping)
- Committed: Yes

## Key File Locations

**Entry Points:**
- `main.go`: CLI entry point. `func main()` at line 18. Parses args, runs processing loop.

**Configuration:**
- `go.mod`: Module path `github.com/fashni/mxlrc-go`, Go 1.22
- `.golangci.yml`: Linter rules (errcheck, govet, staticcheck, gosec, revive, etc.)
- `.goreleaser.yml`: Cross-platform build matrix (linux/darwin/windows, amd64/arm64)
- `Makefile`: Build/test/lint/format commands
- `.pre-commit-config.yaml`: Pre-commit framework hooks

**Core Logic:**
- `musixmatch.go`: API client. `Musixmatch.findLyrics(Track) (Song, error)` at line 24.
- `lyrics.go`: LRC output. `writeLRC(Song, string, string) bool` at line 12.
- `utils.go`: Input parsing. `parseInput(Args, *InputsQueue) string` at line 122.
- `structs.go`: All types. `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`.

**Testing:**
- `utils_test.go`: Unit tests for `slugify()`. Only test file in the project.

## Naming Conventions

**Files:**
- Go source: lowercase, single-word or compound-word names: `main.go`, `musixmatch.go`, `lyrics.go`, `structs.go`, `utils.go`
- Test files: `{source}_test.go` pattern: `utils_test.go`
- Config files: dotfiles with standard names: `.golangci.yml`, `.goreleaser.yml`, `.editorconfig`

**Functions:**
- camelCase: `findLyrics`, `writeLRC`, `parseInput`, `getSongDir`, `getSongMulti`, `getSongText`, `assertInput`, `slugify`, `isInArray`
- Exported functions: PascalCase (none currently -- all functions are unexported `main` package functions)
- Method receivers: short names matching struct initial: `mx Musixmatch`, `q *InputsQueue`

**Variables:**
- snake_case used in some older code: `song_list`, `text_fn`, `save_path`, `lrc_file` (in `utils.go`)
- camelCase used in newer code: `baseURL`, `maxSec`, `errBody` (in `musixmatch.go`, `main.go`)
- Package-level globals: short names: `inputs`, `failed` (`main.go:15-16`)

**Types:**
- PascalCase structs: `Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`, `InputsQueue`
- JSON tags use Musixmatch API field names: `json:"track_name,omitempty"`

**Constants:**
- ALL_CAPS for the single constant: `URL` (`musixmatch.go:18`)

## Where to Add New Code

**New Feature (e.g., new input mode, new output format):**
- Primary code: Add to the appropriate existing file based on responsibility:
  - API interactions: `musixmatch.go`
  - Output formatting: `lyrics.go`
  - Input parsing/scanning: `utils.go`
  - New types: `structs.go`
  - CLI flags/orchestration: `main.go`
- Tests: Create `{file}_test.go` alongside the source (e.g., `lyrics_test.go`, `musixmatch_test.go`)

**New Subcommand or Major Feature (e.g., database caching, batch mode):**
- If it stays in `main` package: Add a new file named for the feature (e.g., `cache.go`, `database.go`)
- If it warrants separation: Consider creating internal packages (`internal/api/`, `internal/lrc/`, etc.) -- but note the project currently follows a deliberate flat structure

**New Utility Function:**
- Shared helpers: `utils.go`
- Tests: `utils_test.go`

**New External Integration:**
- Add dependency: `go get <package>` (updates `go.mod` and `go.sum`)
- Wrap in a new file or add to existing file matching the responsibility area

**New CI/CD Step:**
- Workflow files: `.github/workflows/`
- Local hooks: `.githooks/pre-commit`

## Special Directories

**`.planning/`:**
- Purpose: Codebase analysis and planning documents for AI-assisted development
- Generated: Yes (by GSD codebase mapper)
- Committed: Yes

**`.claude/`:**
- Purpose: Claude Code configuration and custom commands
- Generated: Partially (settings are manual, some commands are generated)
- Committed: Yes (except `settings.local.json`)

**`.opencode/`:**
- Purpose: OpenCode/GSD (Get Shit Done) framework configuration, agents, workflows
- Generated: Yes (framework installation)
- Committed: Yes

**`.serena/`:**
- Purpose: Serena MCP configuration
- Generated: Yes
- Committed: No (gitignored)

**`dist/` (when present):**
- Purpose: GoReleaser build output
- Generated: Yes (by `goreleaser build`)
- Committed: No (gitignored)

---

*Structure analysis: 2026-04-10*

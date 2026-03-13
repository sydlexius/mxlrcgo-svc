# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MxLRC-Go is a Go CLI tool that fetches synced lyrics from the Musixmatch API and saves them as `.lrc` files. It is a Go port of the Python [MxLRC](https://github.com/fashni/MxLRC) project.

## Build & Test Commands

```bash
make build        # Build binary
make test         # Run all tests (with race detector)
make test-cover   # Tests + coverage report
make lint         # Run golangci-lint
make fmt          # Format Go files (gofmt + goimports)
make hooks        # Install git pre-commit hook
make clean        # Remove build artifacts
make help         # Show all targets
```

Direct Go commands also work:
```bash
go build ./...           # Build
go run .                 # Run directly
go test -run TestFoo     # Run a single test
```

## Quality Gating

**Pre-commit hook** (`.githooks/pre-commit`, install via `make hooks`):
1. typos — spell check staged files
2. gofmt — format check
3. go build — compilation check
4. golangci-lint — full linter suite
5. govulncheck — dependency vulnerability scan

**Pre-commit framework** (`.pre-commit-config.yaml`): trailing whitespace, EOF fixer, YAML check, large file guard, merge conflict detection, typos, gitleaks (secret scanning), golangci-lint, gofmt, conventional commit messages.

**Linting** (`.golangci.yml`): errcheck, govet, staticcheck, unused, bodyclose, gosec, noctx, unconvert, unparam, wastedassign, misspell, revive. Test files are excluded from gosec/errcheck/noctx.

## CI/CD Pipeline

- **ci.yml** — Lint → Test → Build (multi-platform: linux/darwin/windows, amd64/arm64). Build only runs after lint+test pass.
- **release.yml** — GoReleaser on `v*.*.*` tags. Builds cross-platform binaries and creates GitHub Releases with conventional-commit changelogs.
- **codeql.yml** — Security scanning on push/PR/weekly schedule.
- **dependabot.yml** — Weekly gomod + github-actions updates. Auto-approve (patch/minor) and auto-merge workflows included.

## Architecture

Single `main` package, flat file structure — no subdirectories:

- **main.go** — Entry point, CLI orchestration loop. Parses args via `go-arg`, iterates the `InputsQueue`, calls `Musixmatch.findLyrics()` then `writeLRC()` for each song. Handles graceful shutdown (SIGTERM) and writes a `_failed.txt` file for retries.
- **musixmatch.go** — `Musixmatch` struct with `findLyrics(Track)` method. Calls the `apic-desktop.musixmatch.com` API, parses the nested JSON response with `fastjson`, and returns a `Song` with track metadata + lyrics/subtitles.
- **lyrics.go** — `writeLRC()` dispatches to `writeSyncedLRC`, `writeUnsyncedLRC`, or `writeInstrumentalLRC` based on what content is available. Generates LRC tags (artist, title, album, length) and writes buffered output.
- **utils.go** — Input parsing (`parseInput` detects mode: CLI/text-file/directory), directory scanning with `dhowden/tag` for audio metadata, `slugify` for safe filenames, and `isInArray` generic helper.
- **structs.go** — All data types (`Track`, `Song`, `Lyrics`, `Synced`, `Lines`, `Time`, `Args`, `Inputs`) and `InputsQueue` (simple FIFO queue with `next/pop/push/len/empty`).

## Input Modes

`parseInput` in utils.go determines how to process the `Song` positional args:

1. **CLI** — `artist,title` pairs passed directly (e.g., `adele,hello`)
2. **Text file** — A `.txt` file with one `artist,title` per line
3. **Directory** — Recursively scans for audio files (.mp3, .m4a, .flac, etc.), reads metadata via `dhowden/tag`, and fetches lyrics for each. DFS by default, BFS optional. Overrides `--outdir` to save `.lrc` alongside audio files.

## Key Dependencies

- `github.com/alexflint/go-arg` — CLI argument parsing via struct tags
- `github.com/dhowden/tag` — Audio file metadata reading (ID3, MP4, FLAC, etc.)
- `github.com/valyala/fastjson` — Fast JSON parsing for Musixmatch API responses
- `golang.org/x/text` — Unicode normalization (NFKC) in `slugify`

## Style and Conventions

- **No emoji** in code, commits, comments, or documentation
- **No em-dashes** in any output
- Use conventional commits: `feat:`, `fix:`, `docs:`, `ci:`, `chore:`, etc.
- Run `make lint` before pushing -- the pre-commit hook and CI enforce the same checks.
- Releases are cut by tagging: `git tag v1.0.0 && git push --tags`

## Database

When adding stateful features, follow these patterns:

- Pure Go SQLite via `modernc.org/sqlite` (no CGO required, cross-compiles cleanly)
- WAL mode for concurrent read access
- Migrations managed by goose (SQL files in a `migrations/` directory)
- Repository pattern for data access via interfaces -- keeps storage swappable for testing
- Integration tests use real SQLite (in-memory with `file::memory:?cache=shared` or temp file), not mocks

## PR Workflow

**Run review locally BEFORE pushing and opening the PR -- never after.**

Copilot reviews only the diff on each push, not the whole file. Opening the PR fires Copilot
immediately; each fixup commit then exposes the next layer of issues it did not see before.
Running the local review first surfaces everything in one pass.

Correct order:
1. Write code and tests
2. `make test` -- all tests pass
3. Run local review -- fix any critical or important findings
4. Commit all fixes
5. Squash all development commits into clean, coherent commits (see below)
6. Push and open the PR

### Squash before first push

Squash all development/fixup commits into clean, logical commits before the first push.
Copilot reviews only the diff it sees on each push. Incremental commits hide the full
changeset from it, causing it to rediscover issues on each push. Squashing presents the
final state once.

```bash
# Squash all commits since branching from main into one clean commit:
git rebase -i main
# In the editor: mark the first commit "pick", all others "squash" or "fixup"
```

For larger PRs with logically distinct phases, two or three coherent commits is fine.
The goal is coherence, not a single commit at all costs.

**Do not squash after opening the PR.** Force-pushing a rebase after opening destroys
review context and resets Copilot's diff window.

### Review comment scope policy

**Default: fix now.** When a review comment or adjacent code issue is discovered
during PR work, the default action is to fix it in the current PR.

**To defer, you must justify.** A fix should only be deferred to a separate issue when:
- It requires architectural changes that would fundamentally alter the PR's scope
- It touches subsystems unrelated to the PR's purpose AND requires its own test suite

**Never reply "out of scope" without creating a tracking issue.** If you defer, open an
issue immediately, link it to the current PR, and reply to the review comment with the
issue number.

### Reading PR comments (gh API)

The `!` character triggers bash history expansion, even inside double quotes. This breaks
`--jq` filters that use `!=`. Always use one of these safe patterns:

```bash
# List all PR review comments (safe -- no != operator):
gh api "repos/{owner}/{repo}/pulls/{number}/comments" --paginate \
  --jq '[.[] | select(.body | length > 0) | {id, user: .user.login, path, line, body}]'

# Filter out a specific user (use "== X | not" instead of "!= X"):
gh api "repos/{owner}/{repo}/pulls/{number}/comments" --paginate \
  --jq '[.[] | select(.user.login == "some-bot" | not) | {id, user: .user.login, body}]'

# Reply to a review comment:
gh api "repos/{owner}/{repo}/pulls/{number}/comments/{comment_id}/replies" \
  -f body='Fixed in <commit>.'
```

**Never use `!=` in `--jq` expressions from bash.** Use `select(.field == "value" | not)`.

### Copilot re-review limitation

The GitHub API does not support re-requesting review from Copilot (bot accounts return
422). This means the first push to a PR is the only chance to get a clean Copilot review.
Get it right before pushing.

## Known Issues

- Global mutable state: `inputs` and `failed` are package-level `InputsQueue` vars in main.go.

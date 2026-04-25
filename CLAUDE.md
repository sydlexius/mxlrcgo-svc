# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`mxlrcsvc-go` (module `github.com/sydlexius/mxlrcsvc-go`) is a Go CLI tool that fetches synced lyrics from the Musixmatch API and saves them as `.lrc` files. It is a fork of `fashni/mxlrc-go`, restructured to eliminate global state, externalize the API token, and add stateful features (TOML config, SQLite cache).

For deeper detail on the stack, conventions, architecture, and data flow, read `AGENTS.md` -- it is the auto-generated reference and stays in sync with the codebase. Read it whenever you need detail this file omits.

## What to work on next

When the user says **"next"**, **"what's next"**, **"keep going"**, or any equivalent lazy prompt with no specific task, inspect the open GitHub issues and milestones before starting. Confirm scope with the user first. The current dependency chain is scanner decoupling before M3 library-management work: issue #31 first, then M3 issues #15, #16, and #17.

## Build & Test

`make help` lists every target. Two non-obvious points worth knowing up front:

- The entrypoint lives in `cmd/mxlrcsvc-go`, so `go run .` does not work. Use `go run ./cmd/mxlrcsvc-go [args]`.
- A single test: `go test -run TestFoo ./internal/<pkg>` (tests live next to the code they cover under `internal/`).

## Architecture (one-paragraph orientation)

Cmd/internal layout. `cmd/mxlrcsvc-go/main.go` is the only entry point and owns no business logic; it parses args, loads config + DB, builds the dependency graph, and runs `app.App.Run`. Under `internal/`: `app` owns the processing loop and queues; `musixmatch` calls the API (exposes a `Fetcher` interface); `lyrics` writes `.lrc` / `.txt` / instrumental output (exposes a `Writer` interface); `scanner` parses CLI/text-file/directory input into the queue; `config` resolves TOML config (XDG paths) with token precedence CLI > env > file; `db` is pure-Go SQLite (`modernc.org/sqlite`, no CGO) with goose migrations in `internal/db/migrations/`; `cache` is the lyrics cache repo over the DB; `normalize` builds NFKC cache lookup keys; `models` holds the shared data types and depends on nothing else internal. `app` depends on `Fetcher` and `Writer` interfaces, never concrete types -- mock at those boundaries. There is no global mutable state. See `AGENTS.md` for full layer/data-flow detail.

## CLI usage and input modes

See `README.md` for flags and examples. Worth flagging: directory mode overrides `--outdir` (writes `.lrc` next to the audio file), and `--upgrade` re-fetches songs that previously got `.txt` (unsynced) to promote them when synced lyrics become available.

## Quality gating and CI

- Pre-commit hook: `make hooks` installs `.githooks/pre-commit` (typos -> gofmt -> build -> golangci-lint -> govulncheck).
- Linter config: `.golangci.yml`. Always include a `// reason` comment after any `//nolint:linter` directive.
- CI workflows live in `.github/workflows/` (`ci.yml`, `release.yml`, `codeql.yml`).
- Releases: `git tag vX.Y.Z && git push --tags` triggers GoReleaser.

## Style (non-discoverable rules)

- **No emoji** in code, commits, comments, or docs.
- **No em-dashes** in any output.
- Conventional commits: `feat:`, `fix:`, `docs:`, `ci:`, `chore:`, etc.
- `slog` for structured logs; `fmt.Printf` only for direct user-facing CLI output (timer, counts).
- Wrap errors with `fmt.Errorf("context: %w", err)`.

Everything else (formatting, naming, file layout) is enforced by `gofmt` + `.golangci.yml` -- follow the linter, not a written rule.

## Database (when adding stateful features)

- Pure-Go SQLite via `modernc.org/sqlite`. **Never reintroduce CGO** -- it breaks cross-compilation.
- WAL mode; goose-managed migrations in `internal/db/migrations/`.
- Repository pattern over interfaces (see `internal/cache/`) so storage stays swappable.
- Integration tests use real SQLite (in-memory `file::memory:?cache=shared` or temp file), not mocks.

## PR Workflow

Prefer the global slash commands -- they encode the full workflow and stay maintained outside this repo:

- `/commit` -- single commit (use during development)
- `/prep-pr` -- pre-push gate: runs all checks, then squashes and pushes
- `/commit-push-pr` -- one-shot: commit + push + open PR (small/simple changes)
- `/handle-review` -- triage open bot review comments, fix in one pass, reply in batch, push once
- `/review-stack` -- same as `/handle-review` but across an entire PR stack in dependency order
- `/merge-pr` -- merge with CodeRabbit status check, squash, post-merge cleanup
- `/post-merge-cleanup` -- update main, delete merged branches, prune refs
- `/clean_gone` -- prune local branches whose remote is [gone]
- `/review` or `/code-review:code-review` -- local code review before pushing
- `/security-review` -- security review of pending changes

Typical flow: develop -> `/commit` (repeat) -> `/review` -> fix findings -> `/prep-pr` -> open PR -> CodeRabbit reviews automatically -> `/handle-review` -> `/merge-pr`.

### Review comment scope

**Default: fix now.** Defer to a separate issue only when the fix would fundamentally alter the PR's scope OR touches an unrelated subsystem requiring its own test suite. **Never reply "out of scope" without opening a tracking issue** and linking it from the review reply.

### Reading PR comments (gh API gotcha)

If you fall back to raw `gh` instead of `/handle-review`: the `!` character triggers bash history expansion even inside double quotes, which breaks `--jq` filters using `!=`. Always use `select(.field == "value" | not)` instead:

```bash
gh api "repos/{owner}/{repo}/pulls/{number}/comments" --paginate \
  --jq '[.[] | select(.user.login == "some-bot" | not) | {id, user: .user.login, body}]'
```

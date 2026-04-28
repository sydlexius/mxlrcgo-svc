# Plan: Rename Project to mxlrcgo-svc

## Goal

Break project identity away from the upstream fork name and rename local code/docs/tooling from `mxlrcsvc-go` / `mxlrc-go` to `mxlrcgo-svc`.

## Tasks

- Update module path and all internal imports to `github.com/sydlexius/mxlrcgo-svc`.
- Rename the CLI entrypoint directory and binary references to `mxlrcgo-svc`.
- Update docs, examples, smoke script, config defaults, and tests.
- Check GitHub fork/remote state and document what can be detached automatically.
- Run formatting and tests.

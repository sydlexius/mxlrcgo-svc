# Summary: Rename Project to mxlrcgo-svc

## Completed

- Renamed GitHub repository from `sydlexius/mxlrc-go` to `sydlexius/mxlrcgo-svc`.
- Updated local `origin` to `git@github.com:sydlexius/mxlrcgo-svc.git`.
- Updated Go module, imports, CLI command path, binary name, release config, CI build path, smoke script, docs, and config defaults.
- Removed upstream project references from local docs and repository description.
- Deleted local branches whose PR heads were already squash-merged into `main`.

## Verification

- `go test ./...`
- `make smoke`

## Remaining

- GitHub still reports the repository as a fork of `fashni/mxlrc-go`; renaming did not detach the fork network.

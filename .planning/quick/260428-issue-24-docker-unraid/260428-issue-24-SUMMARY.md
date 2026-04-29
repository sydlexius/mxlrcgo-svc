# Summary: Issue #24 Docker and Unraid Packaging

## Completed

- Added a multi-stage `Dockerfile` that builds `mxlrcgo-svc` with `CGO_ENABLED=0` and runs it from Alpine.
- Added a Docker entrypoint that supports Unraid-style `PUID`/`PGID` remapping and drops privileges with `su-exec`.
- Added `.dockerignore` and `docker-compose.example.yml` with port `50705`, `/config`, `/music`, `MXLRC_DOCKER=true`, Musixmatch token, and webhook key settings.
- Added an Unraid Community Applications template at `unraid/mxlrcgo-svc.xml`, following the structure used by the local `~/Developer/unraid-templates` examples.
- Added explicit `MXLRC_DOCKER=true` / `1` storage detection so default config and DB paths resolve under `/config`.
- Documented Docker and Unraid usage in `README.md`.

## Verification

```bash
gofmt -w internal/config
go test ./...
git diff --check
xmllint --noout unraid/mxlrcgo-svc.xml
docker build -t mxlrcgo-svc:test .
docker run --rm mxlrcgo-svc:test mxlrcgo-svc config get db.path
```

All checks passed. The container path smoke test printed `/config/mxlrcgo.db`.

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/mxlrcgo-svc-smoke.XXXXXX")"
trap 'rm -rf "$TMPDIR"' EXIT

BIN="$TMPDIR/mxlrcgo-svc"
CONFIG_HOME="$TMPDIR/config"
DATA_HOME="$TMPDIR/data"
OUTDIR="$TMPDIR/out"

mkdir -p "$CONFIG_HOME" "$DATA_HOME" "$OUTDIR"

CGO_ENABLED=0 go build -o "$BIN" "$ROOT/cmd/mxlrcgo-svc"

help_output="$("$BIN" --help)"
if [[ "$help_output" != *"Usage: mxlrcgo-svc"* ]]; then
  echo "smoke: --help output did not include usage" >&2
  exit 1
fi

set +e
missing_token_output="$(
  cd "$TMPDIR" && env -i \
      PATH="$PATH" \
      HOME="$TMPDIR/home" \
      XDG_CONFIG_HOME="$CONFIG_HOME" \
      XDG_DATA_HOME="$DATA_HOME" \
      "$BIN" --outdir "$OUTDIR" "Artist,Title" 2>&1
)"
missing_token_status=$?
set -e

if [[ $missing_token_status -eq 0 ]]; then
  echo "smoke: missing-token command succeeded; want non-zero exit" >&2
  exit 1
fi
if [[ "$missing_token_output" != *"no API token provided"* ]]; then
  echo "smoke: missing-token output did not include expected error" >&2
  echo "$missing_token_output" >&2
  exit 1
fi
if [[ -e "$DATA_HOME/mxlrcgo-svc/mxlrcgo.db" ]]; then
  echo "smoke: missing-token command created a database" >&2
  exit 1
fi

echo "smoke: ok"

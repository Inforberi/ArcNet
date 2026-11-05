#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

go test -count=1 ./...
go vet ./...
go test -count=1 -race ./internal/routes ./internal/store

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
go build -o "$tmp_dir/arcvpn" ./cmd/arcvpn

if [[ "${1:-}" == '--full' ]]; then
  if [[ ! -f frontend/package-lock.json ]]; then
    echo 'BLOCKED: frontend/package-lock.json absent; no React typecheck/build verification' >&2
    exit 1
  fi
  (cd frontend && npm ci && npm run test && npm run typecheck && npm run build)
fi
printf '%s\n' 'STAGE5_GO_SOURCE_PASS (not a deployed VPN or React acceptance)'

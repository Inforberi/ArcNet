#!/usr/bin/env bash
# Repeatable source-only checks. No VPN services, root privileges, or network needed.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
command -v go >/dev/null || { echo 'Go toolchain unavailable' >&2; exit 1; }
echo '[1/4] Go tests'
go test ./...
echo '[2/4] Static analysis'
go vet ./...
echo '[3/4] Core concurrency tests'
go test -race ./internal/app ./internal/httpapi ./internal/backup ./internal/protocols ./internal/engines ./internal/xray
echo '[4/4] Shell syntax'
for script in install.sh deploy/*.sh scripts/*.sh; do
  bash -n "$script"
done
printf '\nLocal source checks: PASS\n'
printf 'Not covered: npm typecheck/build, browser E2E, upstream AWG build, real Linux systemd or external VPN connectivity.\n'

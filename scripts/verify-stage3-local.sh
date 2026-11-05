#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo '[stage3] Go tests'
go test ./...
echo '[stage3] Go vet'
go vet ./...
echo '[stage3] Bash syntax'
for script in install.sh scripts/*.sh deploy/*.sh web/node-install.sh; do bash -n "$script"; done
echo '[stage3] Compile local Go development binary'
mkdir -p .local/bin
CGO_ENABLED=1 go build -trimpath -o .local/bin/arcvpn ./cmd/arcvpn
.local/bin/arcvpn version
if [[ -d frontend/node_modules && -f frontend/package-lock.json ]];then
 echo '[stage3] React TypeScript + production Vite'
 (cd frontend && npm run build)
else
 echo '[stage3] React build NOT RUN: package-lock.json and/or node_modules missing'
fi

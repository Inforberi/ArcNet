#!/usr/bin/env bash
# Produce a release only if the actual React UI and Go binary are built together.
set -Eeuo pipefail
umask 022
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

command -v go >/dev/null || { echo 'Go is required' >&2; exit 1; }
command -v npm >/dev/null || { echo 'Node.js/npm are required for the build machine' >&2; exit 1; }
command -v gcc >/dev/null || { echo 'A C compiler is required for SQLite/QR code bindings' >&2; exit 1; }
[[ "$(uname -s)" == "Linux" && "$(uname -m)" == "x86_64" ]] || { echo 'This CGO build script currently requires Linux x86_64; use a Linux CI runner.' >&2; exit 1; }

echo '[1/5] Install pinned frontend dependencies'
(
  cd frontend
  if [[ ! -f package-lock.json ]]; then
    echo 'Missing frontend/package-lock.json. Generate it once on a networked build machine with: cd frontend && npm install --package-lock-only --ignore-scripts' >&2
    exit 1
  fi
  npm ci --no-audit --no-fund
)

echo '[2/5] Type-check and build React UI'
(cd frontend && npm run build)
[[ -s frontend/dist/index.html ]] || { echo 'Frontend build produced no index.html' >&2; exit 1; }

echo '[3/5] Fast backend verification'
go test ./...
go vet ./...
bash -n install.sh

echo '[4/5] Embed React and build Linux x86_64 binary'
mkdir -p dist
rm -rf web/react-dist
trap 'rm -rf "$ROOT/web/react-dist"' EXIT
cp -a frontend/dist web/react-dist
ARCVPN_BUILD_VERSION="${ARCVPN_BUILD_VERSION:-0.7.0-stage5}"
[[ "$ARCVPN_BUILD_VERSION" =~ ^0\.(6|7)\.(0|[1-9][0-9]{0,5})(-[A-Za-z0-9_-]+)?$ ]] || { echo 'Unsupported panel release version; use 0.6.x or 0.7.x' >&2; exit 65; }
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags arcvpn_embed_react -trimpath -buildvcs=false -ldflags="-s -w -X github.com/arcvpn/arcvpn/internal/nodeagent.Version=$ARCVPN_BUILD_VERSION" -o dist/arcvpn-linux-amd64 ./cmd/arcvpn
./dist/arcvpn-linux-amd64 frontend-check
echo '[5/5] Package the installer and verified runtime'
# Legacy compatibility only; not part of the default supported scope.
ARCVPN_WITH_AMNEZIAWG="${ARCVPN_WITH_AMNEZIAWG:-0}"
[[ "$ARCVPN_WITH_AMNEZIAWG" == 0 || "$ARCVPN_WITH_AMNEZIAWG" == 1 ]] || { echo 'ARCVPN_WITH_AMNEZIAWG must be 0 or 1' >&2; exit 65; }
if [[ "$ARCVPN_WITH_AMNEZIAWG" == 1 ]]; then "$ROOT/scripts/build-amneziawg.sh"; fi
printf '%s\n' "$ARCVPN_BUILD_VERSION" > dist/VERSION
ARCVPN_WITH_AMNEZIAWG="$ARCVPN_WITH_AMNEZIAWG" "$ROOT/scripts/package-release.sh"
printf '\nRelease build complete: dist/arcvpn-linux-amd64 (React embedded)\n'
printf 'The installer verifies the embedded React frontend before changing the VPS.\n'
printf 'CGO requires libsqlite3.so.0 and libqrencode.so.4 on the runtime VPS.\n'

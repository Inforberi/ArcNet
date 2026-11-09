#!/usr/bin/env bash
# Produce a release only if the actual React UI and Go binary are built together.
set -Eeuo pipefail
umask 022
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

command -v go >/dev/null || { echo 'Go is required' >&2; exit 1; }
command -v npm >/dev/null || { echo 'Node.js/npm are required for the build machine' >&2; exit 1; }
command -v gcc >/dev/null || { echo 'A C compiler is required for SQLite/QR code bindings' >&2; exit 1; }
command -v python3 >/dev/null || { echo 'Python 3 is required only on the release host for build receipts' >&2; exit 1; }
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
install -m 0644 deploy/update-public.hex dist/update-public.hex
rm -rf web/react-dist
trap 'rm -rf "$ROOT/web/react-dist"' EXIT
cp -a frontend/dist web/react-dist
ARCVPN_BUILD_VERSION="${ARCVPN_BUILD_VERSION:-$(cat VERSION)}"
[[ "$ARCVPN_BUILD_VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid SemVer release version' >&2; exit 65; }
CGO_ENABLED=0 go run ./cmd/arcvpn-release version-check --version "$ARCVPN_BUILD_VERSION"
revision="${ARCVPN_SOURCE_REVISION:-$(git rev-parse --verify HEAD)}"
dirty="${ARCVPN_SOURCE_DIRTY:-}"
if [[ -z "$dirty" ]]; then
 dirty=true
 if [[ -z "$(git status --porcelain --untracked-files=normal)" ]]; then dirty=false; fi
fi
[[ "$revision" =~ ^[0-9a-f]{40}$ && ( "$dirty" == true || "$dirty" == false ) ]] || { echo 'Invalid source revision/dirty metadata' >&2; exit 65; }
[[ -s SOURCE_MANIFEST.sha256 ]] || { echo 'Source manifest is required' >&2; exit 65; }
sha256sum --status -c SOURCE_MANIFEST.sha256
source_sha="$(sha256sum SOURCE_MANIFEST.sha256 | cut -d ' ' -f 1)"
go_version="$(go env GOVERSION)"
node_version="$(node --version)"
build_os="$(. /etc/os-release; printf '%s-%s' "$ID" "$VERSION_ID")"
[[ "$go_version" =~ ^go[0-9.]+$ && "$node_version" =~ ^v[0-9.]+$ && "$build_os" =~ ^[a-z0-9.-]+$ ]] || { echo 'Invalid toolchain metadata' >&2; exit 65; }
printf '{"format":"arcvpn-build-v1","version":"%s","target":"linux/amd64","source_commit":"%s","source_dirty":%s,"source_manifest_sha256":"%s","go":"%s","node":"%s","build_os":"%s"}\n' \
 "$ARCVPN_BUILD_VERSION" "$revision" "$dirty" "$source_sha" "$go_version" "$node_version" "$build_os" > dist/BUILDINFO.json
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags arcvpn_embed_react -trimpath -buildvcs=false -ldflags="-s -w -X github.com/arcvpn/arcvpn/internal/nodeagent.Version=$ARCVPN_BUILD_VERSION" -o dist/arcvpn-linux-amd64 ./cmd/arcvpn
./dist/arcvpn-linux-amd64 frontend-check
./dist/arcvpn-linux-amd64 version | grep -Fx "arcvpn $ARCVPN_BUILD_VERSION"
./dist/arcvpn-linux-amd64 help | grep -Fx 'local management protocol: lifecycle-v1'
python3 - <<'PYBUILDINFO'
from pathlib import Path
import hashlib,json
p=Path('dist/BUILDINFO.json');info=json.loads(p.read_text());info['core_sha256']=hashlib.sha256(Path('dist/arcvpn-linux-amd64').read_bytes()).hexdigest();info['frontend_verified']=True;p.write_text(json.dumps(info,separators=(',',':'))+'\n')
PYBUILDINFO
echo '[5/5] Package the installer and verified runtime'
# Legacy compatibility only; not part of the default supported scope.
ARCVPN_WITH_AMNEZIAWG="${ARCVPN_WITH_AMNEZIAWG:-0}"
[[ "$ARCVPN_WITH_AMNEZIAWG" == 0 || "$ARCVPN_WITH_AMNEZIAWG" == 1 ]] || { echo 'ARCVPN_WITH_AMNEZIAWG must be 0 or 1' >&2; exit 65; }
if [[ "$ARCVPN_WITH_AMNEZIAWG" == 1 ]]; then "$ROOT/scripts/build-amneziawg.sh"; fi
printf '%s\n' "$ARCVPN_BUILD_VERSION" > dist/VERSION
ARCVPN_WITH_AMNEZIAWG="$ARCVPN_WITH_AMNEZIAWG" "$ROOT/scripts/package-release.sh"
cut -c 67- SOURCE_MANIFEST.sha256 > dist/source-files.txt
tar -czf "dist/arcvpn-$ARCVPN_BUILD_VERSION-source.tar.gz" --files-from=dist/source-files.txt SOURCE_MANIFEST.sha256
printf '\nRelease build complete: dist/arcvpn-linux-amd64 (React embedded)\n'
printf 'The installer verifies the embedded React frontend before changing the VPS.\n'
printf 'CGO requires libsqlite3.so.0 and libqrencode.so.4 on the runtime VPS.\n'

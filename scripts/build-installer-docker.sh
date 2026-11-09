#!/usr/bin/env bash
# Optional build-host tooling, never a requirement on the VPS.
set -Eeuo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
version="${ARCVPN_BUILD_VERSION:-$(cat VERSION)}"
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release version' >&2;exit 65; }
revision="$(git rev-parse --verify HEAD)"
dirty=true
if [[ -z "$(git status --porcelain --untracked-files=normal)" ]]; then dirty=false; fi
docker buildx build --platform linux/amd64 -f deploy/release.Dockerfile \
 --build-arg "ARCVPN_BUILD_VERSION=$version" \
 --build-arg "ARCVPN_SOURCE_REVISION=$revision" --build-arg "ARCVPN_SOURCE_DIRTY=$dirty" \
 --metadata-file "${TMPDIR:-/tmp}/arcvpn-$version-build-metadata.json" \
 --output "type=local,dest=dist/installer-$version" .

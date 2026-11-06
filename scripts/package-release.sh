#!/usr/bin/env bash
# Package already-built artifacts. Compilation/signing/VM acceptance are separate.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
IFS= read -r version < dist/VERSION
[[ "$version" =~ ^0\.(6|7)\.(0|[1-9][0-9]{0,5})(-[A-Za-z0-9_-]+)?$ ]] || { echo 'Invalid dist/VERSION' >&2; exit 65; }
files=(dist/arcvpn-linux-amd64 dist/VERSION dist/BUILDINFO.json SOURCE_MANIFEST.sha256 install.sh deploy/arcvpn-menu.sh deploy/arcvpn-tunnel-net.sh README.md LICENSE
 docs/ops/install-and-update.md docs/ops/local-admin-recovery.md docs/ops/local-service-control.md docs/planning/short-work-blocks.md)
legacy="${ARCVPN_WITH_AMNEZIAWG:-0}"
[[ "$legacy" == 0 || "$legacy" == 1 ]] || { echo 'ARCVPN_WITH_AMNEZIAWG must be 0 or 1' >&2; exit 65; }
if [[ "$legacy" == 1 ]]; then
 files+=(dist/awg-linux-amd64 dist/awg-quick-linux-amd64 dist/amneziawg-go-linux-amd64
   dist/sources/amneziawg-tools-v3.1.20260812.tar.gz dist/sources/amneziawg-go-v3.1.20260828.tar.gz dist/AMNEZIAWG-SOURCES.txt)
fi
if [[ -s dist/update-public.hex ]]; then files+=(dist/update-public.hex); fi
for file in "${files[@]}"; do
 [[ -f "$file" && ! -L "$file" && -s "$file" ]] || { echo "Missing or unsafe release artifact: $file" >&2; exit 1; }
done
sha256sum "${files[@]}" > dist/SHA256SUMS
sha256sum --status -c dist/SHA256SUMS
archive="dist/arcvpn-$version-linux-amd64.tar.gz"
tar -czf "$archive" "${files[@]}" dist/SHA256SUMS
sha256sum "$archive" > "$archive.sha256"
printf 'Installer archive: %s\n' "$archive"
printf 'Private signing keys, databases and unlisted files are excluded.\n'

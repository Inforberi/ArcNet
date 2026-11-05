#!/usr/bin/env bash
# Fast package/integrity fixture: no compilation, installer execution or OS changes.
set -Eeuo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/scripts" "$fixture/dist"
cp "$root/scripts/package-release.sh" "$fixture/scripts/"
for file in install.sh deploy/arcvpn-menu.sh deploy/arcvpn-tunnel-net.sh README.md LICENSE docs/ops/install-and-update.md docs/ops/local-admin-recovery.md docs/planning/short-work-blocks.md; do
 mkdir -p "$fixture/$(dirname "$file")"
 cp "$root/$file" "$fixture/$file"
done
printf 'synthetic binary fixture; not executable\n' > "$fixture/dist/arcvpn-linux-amd64"
printf '0.7.0-stage5\n' > "$fixture/dist/VERSION"
printf 'must never be distributed\n' > "$fixture/dist/update-private.hex"
printf '%064d\n' 0 > "$fixture/dist/update-public.hex"
# Stale/unlisted optional artifacts must not silently re-enable the old default.
printf 'unlisted legacy artifact\n' > "$fixture/dist/awg-linux-amd64"
bash "$fixture/scripts/package-release.sh" >/dev/null
archive="$fixture/dist/arcvpn-0.7.0-stage5-linux-amd64.tar.gz"
tar -tzf "$archive" > "$fixture/contents"
grep -qx 'install.sh' "$fixture/contents"
grep -qx 'deploy/arcvpn-menu.sh' "$fixture/contents"
grep -qx 'docs/ops/install-and-update.md' "$fixture/contents"
grep -qx 'dist/update-public.hex' "$fixture/contents"
if grep -Eq 'update-private|awg-linux|panel.sqlite' "$fixture/contents"; then
 echo 'Unexpected private/unlisted artifact in package' >&2; exit 1
fi
mkdir "$fixture/unpacked"
tar -xzf "$archive" -C "$fixture/unpacked"
(
 cd "$fixture/unpacked"
 sha256sum --status -c dist/SHA256SUMS
 printf 'tampered\n' >> install.sh
 if sha256sum --status -c dist/SHA256SUMS >/dev/null 2>&1; then
  echo 'Tampered installer passed integrity check' >&2; exit 1
 fi
)
if ARCVPN_WITH_AMNEZIAWG=1 bash "$fixture/scripts/package-release.sh" >/dev/null 2>&1; then
 echo 'Incomplete legacy bundle accepted' >&2; exit 1
fi
printf '../../escape\n' > "$fixture/dist/VERSION"
if bash "$fixture/scripts/package-release.sh" >/dev/null 2>&1; then
 echo 'Unsafe version accepted' >&2; exit 1
fi
printf 'PASS release packaging: 0.7 installer, scripts/docs/key included, private/stale AWG excluded, tamper rejected. No real build/install.\n'

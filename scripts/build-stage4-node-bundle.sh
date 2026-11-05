#!/usr/bin/env bash
# Sign a release for remote Linux/amd64 ArcVPN nodes on an offline trusted host.
# The private Ed25519 signing key MUST NOT be copied to the controller/VPS.
set -Eeuo pipefail
umask 077

if [[ $# -ne 5 ]]; then
  printf '%s\n' 'Usage: build-stage4-node-bundle.sh ARCVPN_LINUX_AMD64 XRAY_LINUX_AMD64 PRIVATE_ED25519_KEY VERSION OUTPUT_ZIP' >&2
  exit 64
fi

arcvpn_binary=$1
xray_binary=$2
private_key=$3
version=$4
output=$5

[[ -f "$arcvpn_binary" && -f "$xray_binary" && -f "$private_key" ]] || {
  printf '%s\n' 'The ArcVPN binary, Xray binary and signing key must all be regular files.' >&2
  exit 66
}
[[ ! -e "$output" ]] || {
  printf '%s\n' 'Refusing to overwrite an existing signed archive.' >&2
  exit 73
}
command -v file >/dev/null || { printf '%s\n' 'file(1) is required' >&2; exit 69; }
[[ "$version" =~ ^0\.6\.(0|[1-9][0-9]{0,5})(-[A-Za-z0-9_-]+)?$ ]] || { echo 'Signed node release must be within 0.6.x' >&2; exit 65; }
# Prevent a trusted signer from mislabeling an old agent as a newer release.
# The agent advertises its version via the same linker-stamped symbol used in
# the outbound heartbeat and therefore must match the signed manifest.
actual_version=$("$arcvpn_binary" version)
[[ "$actual_version" == "arcvpn $version" ]] || {
  printf 'Binary version mismatch: actual %s, manifest %s\n' "$actual_version" "$version" >&2
  exit 65
}
for binary in "$arcvpn_binary" "$xray_binary"; do
  description=$(file -b -- "$binary")
  [[ "$description" == *ELF* && "$description" == *"x86-64"* ]] || {
    printf '%s\n' 'Both binaries must be Linux x86-64 ELF executables.' >&2
    exit 65
  }
done

"$arcvpn_binary" update-sign \
  --binary "$arcvpn_binary" \
  --xray "$xray_binary" \
  --private-key "$private_key" \
  --version "$version" \
  --output "$output"

sha256sum -- "$output"
printf '%s\n' 'Copy only the signed ZIP and corresponding PUBLIC verification key to the controller.'

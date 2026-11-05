#!/usr/bin/env bash
# Build pinned upstream AmneziaWG 3.1 userspace components on the RELEASE runner.
# This script is not run by the unprivileged ArcVPN panel or on the VPS.
set -Eeuo pipefail
umask 022
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
AWG_TOOLS_TAG='v3.1.20260812'
AWG_GO_TAG='v3.1.20260828'

for tool in git go make cc tar; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Missing release tool: $tool" >&2; exit 1; }
done
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || {
  echo 'This release builder supports Linux x86_64 only' >&2; exit 1;
}
# AWG3.1 upstream currently requires a modern Go toolchain.
GOVERSION="$(go env GOVERSION)"
if [[ ! "$GOVERSION" =~ ^go([0-9]+)\.([0-9]+)(\.[0-9]+)?$ ]] ||
  ((10#${BASH_REMATCH[1]} < 1 || 10#${BASH_REMATCH[2]} < 25)); then
  echo "Go 1.25 or newer required for AmneziaWG 3.1; found $GOVERSION" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf -- "$WORK"' EXIT
mkdir -p "$ROOT/dist/sources"

# Both repositories and version tags are constants, not supplied by the UI or user input.
git clone --quiet --depth 1 --branch "$AWG_TOOLS_TAG" \
  https://github.com/amnezia-vpn/amneziawg-tools.git "$WORK/tools"
git clone --quiet --depth 1 --branch "$AWG_GO_TAG" \
  https://github.com/amnezia-vpn/amneziawg-go.git "$WORK/go"

[[ "$(git -C "$WORK/tools" describe --tags --exact-match)" == "$AWG_TOOLS_TAG" ]] || {
  echo 'Unexpected AmneziaWG tools git revision' >&2; exit 1;
}
[[ "$(git -C "$WORK/go" describe --tags --exact-match)" == "$AWG_GO_TAG" ]] || {
  echo 'Unexpected AmneziaWG Go git revision' >&2; exit 1;
}

# The upstream make target builds awg and supplies the matching awg-quick script.
make -C "$WORK/tools/src" -j2
[[ -s "$WORK/tools/src/wg" && -s "$WORK/tools/src/wg-quick/linux.bash" ]] || {
  echo 'Upstream AWG tools build did not produce required artifacts' >&2; exit 1;
}
install -m 0755 "$WORK/tools/src/wg" "$ROOT/dist/awg-linux-amd64"
install -m 0755 "$WORK/tools/src/wg-quick/linux.bash" "$ROOT/dist/awg-quick-linux-amd64"

# Use upstream Makefile to embed its exact git tag into the reported version.
make -C "$WORK/go" -j2
[[ -s "$WORK/go/amneziawg-go" ]] || { echo 'Missing amneziawg-go binary' >&2; exit 1; }
install -m 0755 "$WORK/go/amneziawg-go" "$ROOT/dist/amneziawg-go-linux-amd64"
"$ROOT/dist/amneziawg-go-linux-amd64" --version | grep -F "$AWG_GO_TAG" >/dev/null || {
  echo 'Built AmneziaWG-Go does not report the pinned 3.1 version' >&2; exit 1;
}

# Include corresponding source snapshots with each redistributed third-party binary.
# Source archives contain the tagged source, not an unknown third-party installer.
git -C "$WORK/tools" archive --format=tar.gz --prefix='amneziawg-tools/' HEAD \
  > "$ROOT/dist/sources/amneziawg-tools-$AWG_TOOLS_TAG.tar.gz"
git -C "$WORK/go" archive --format=tar.gz --prefix='amneziawg-go/' HEAD \
  > "$ROOT/dist/sources/amneziawg-go-$AWG_GO_TAG.tar.gz"
cat > "$ROOT/dist/AMNEZIAWG-SOURCES.txt" <<NOTICE
AmneziaWG 3.1 redistributable components:
  https://github.com/amnezia-vpn/amneziawg-tools  $AWG_TOOLS_TAG
  https://github.com/amnezia-vpn/amneziawg-go     $AWG_GO_TAG

Unmodified corresponding source archives are in dist/sources/.
Check upstream LICENSE files and apply the relevant license obligations
when distributing any derivatives or modifications of these binaries.
NOTICE
printf 'AmneziaWG userspace release components built from pinned upstream tags.\n'

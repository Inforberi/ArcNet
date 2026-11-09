#!/usr/bin/env bash
# Trusted build host only. No signing key enters Docker, Git, or the VPS.
set -Eeuo pipefail
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."
version=$(cat VERSION)
minimum=0.7.0-stage5-a10
publish=0
prebuilt=''
source=deploy/update-catalog-v2-source.json
notes="docs/releases/$version.md"
key=${ARCVPN_SIGNING_KEY:-${HOME}/.local/share/arcvpn-release-signing-2026-11-09/installer-private.hex}
while (($#)); do
 case "$1" in
  --publish) publish=1;shift;;
  --bridge) version=0.7.0-stage5-a10;minimum=0.7.0-stage5-a7;source=deploy/update-catalog-source.json;notes=docs/releases/bridge-a10.md;shift;;
  --key|--minimum-version|--notes|--from-build)
   (($#>=2)) || { echo "Missing value: $1" >&2;exit 64; }
   case "$1" in --key) key=$2;;--minimum-version) minimum=$2;;--notes) notes=$2;;--from-build) prebuilt=$2;;esac;shift 2;;
  --help) echo 'release-update.sh [--publish] [--bridge] [--key PATH] [--minimum-version ID] [--notes FILE] [--from-build DIRECTORY]';exit 0;;
  *) echo "Unknown option: $1" >&2;exit 64;;
 esac
done
for command in go git tar python3;do command -v "$command" >/dev/null || { echo "Build host requires $command" >&2;exit 69; };done
if ((publish));then command -v gh >/dev/null || { echo 'Publishing requires authenticated GitHub CLI (gh); nothing published' >&2;exit 69; };gh auth status >/dev/null;fi
[[ "$(git symbolic-ref --short HEAD)" == main ]] || { echo 'Publish releases from main' >&2;exit 65; }
[[ -z "$(git status --porcelain)" ]] || { echo 'Commit source changes first; release must have a clean source snapshot' >&2;exit 65; }
[[ -f "$notes" && ! -L "$notes" ]] || { echo 'Missing release notes' >&2;exit 66; }
out="dist/release-$version"
[[ ! -e "$out" ]] || { echo "Output already exists: $out; never overwrite a release" >&2;exit 73; }
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -o dist/arcvpn-release ./cmd/arcvpn-release
./dist/arcvpn-release version-check --version "$version"
./dist/arcvpn-release key-check --key "$key" --public-key deploy/update-public.hex
remote=$(git ls-remote --tags origin "refs/tags/v$version")
[[ -z "$remote" ]] || { echo "Release tag v$version already exists; choose a new version" >&2;exit 65; }
commit=$(git rev-parse HEAD)
if [[ -z "$prebuilt" ]];then
 command -v docker >/dev/null || { echo 'Build host requires docker' >&2;exit 69; }
 ARCVPN_BUILD_VERSION="$version" ./scripts/build-installer-docker.sh
 prebuilt="dist/installer-$version"
fi
# Signing jobs may consume an artifact only from this exact clean source snapshot.
python3 - "$prebuilt/BUILDINFO.json" "$version" "$commit" <<'PYRECEIPT'
import hashlib,json,pathlib,sys
receipt=json.loads(pathlib.Path(sys.argv[1]).read_text())
assert receipt.get('source_commit')==sys.argv[3] and receipt.get('source_dirty') is False,'Build source differs from clean release snapshot'
assert receipt.get('version')==sys.argv[2] and receipt.get('source_manifest_sha256')==hashlib.sha256(pathlib.Path('SOURCE_MANIFEST.sha256').read_bytes()).hexdigest(),'Build version/manifest mismatch'
PYRECEIPT
archive="$prebuilt/arcvpn-$version-linux-amd64.tar.gz"
core=$(mktemp "${TMPDIR:-/tmp}/arcnet-release-core.XXXXXX")
trap 'rm -f "$core"' EXIT
tar -xOf "$archive" dist/arcvpn-linux-amd64 > "$core"
./dist/arcvpn-release prepare-update --binary "$core" --build-info "$prebuilt/BUILDINFO.json" --version "$version" --minimum-version "$minimum" \
 --key "$key" --public-key deploy/update-public.hex --catalog-source "$source" --notes "$notes" --out-dir "$out"
if ((!publish));then echo "Prepared and verified: $out. Publication was not requested.";exit 0;fi
file="update-$version.zip";catalog=update-catalog-v2.json
if [[ "$version" == 0.7.0-stage5-a* ]];then file="update-${version##*-}.zip";catalog=update-catalog.json;fi
# No --clobber or force push. A partial publication stays explicit and recoverable.
git tag -a "v$version" "$commit" -m "ArcNet $version"
git push origin "refs/tags/v$version"
gh release create "v$version" "$out/$file" --repo Inforberi/ArcNet --verify-tag --draft --title "ArcNet $version" --notes-file "$notes"
verify_dir=$(mktemp -d "${TMPDIR:-/tmp}/arcnet-release-verify.XXXXXX")
# Compare bytes obtained from GitHub before publishing the draft/catalog.
gh release download "v$version" --repo Inforberi/ArcNet --pattern "$file" --dir "$verify_dir"
cmp "$out/$file" "$verify_dir/$file"
rm -f "$verify_dir/$file";rmdir "$verify_dir"
prerelease=false;if [[ "$version" == *-* ]];then prerelease=true;fi
gh release edit "v$version" --repo Inforberi/ArcNet --draft=false --prerelease="$prerelease"
install -m 0644 "$out/$catalog" "deploy/$catalog"
install -m 0644 "$out/catalog-source.json" "$source"
python3 - <<'PYMANIFEST'
from pathlib import Path
import hashlib,subprocess
files=sorted(set(subprocess.check_output(['git','ls-files','-c','-o','--exclude-standard','-z']).decode().split('\0'))-{'','SOURCE_MANIFEST.sha256'})
Path('SOURCE_MANIFEST.sha256').write_text(''.join(hashlib.sha256(Path(name).read_bytes()).hexdigest()+'  '+name+'\n' for name in files if Path(name).is_file()))
PYMANIFEST
git add -- "deploy/$catalog" "$source" SOURCE_MANIFEST.sha256
git commit -m "release: advertise $version in signed catalog"
git push origin HEAD
echo "Published v$version and its signed catalog. Compatible panels can now discover it."
echo 'If publication was interrupted, inspect the existing tag/draft; do not overwrite assets or generate a different package under the same ID.'

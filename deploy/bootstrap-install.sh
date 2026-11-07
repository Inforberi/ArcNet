#!/usr/bin/env bash
# Pinned preview installer. Authenticate this script by its published SHA256 first.
set -Eeuo pipefail
umask 077
RELEASE_VERSION=0.7.0-stage5-a7
FILE="arcvpn-$RELEASE_VERSION-linux-amd64.tar.gz"
BASE="https://github.com/Inforberi/ArcNet/releases/download/v$RELEASE_VERSION"
VERIFY=0
FROM=''
fail(){ printf 'ArcVPN: %s\n' "$*" >&2; exit 1; }
while (($#)); do
 case "$1" in
 --verify-only) VERIFY=1; shift;;
 --from-dir) (($# >= 2)) || fail 'Missing directory'; FROM="$2"; shift 2;;
 --help) echo 'Usage: bash bootstrap-install.sh [--verify-only [--from-dir DIRECTORY]]'; exit 0;;
 *) fail "Unknown argument: $1";;
 esac
done
[[ -z "$FROM" || "$VERIFY" == 1 ]] || fail '--from-dir requires --verify-only'
if (( ! VERIFY )); then
 [[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'Requires Linux x86_64'
 [[ $EUID == 0 ]] || fail 'Run as root'
 for path in /opt/arcvpn /var/lib/arcvpn /etc/arcvpn /etc/systemd/system/arcvpn-panel.service; do
  [[ ! -e "$path" && ! -L "$path" ]] || fail "Existing installation: $path; use migration guide"
 done
 [[ -d /run/systemd/system ]] || fail 'Requires running systemd'
 source /etc/os-release
 case "$ID" in
 debian) dpkg --compare-versions "$VERSION_ID" ge 12 || fail 'Requires Debian 12+';;
 ubuntu) dpkg --compare-versions "$VERSION_ID" ge 22.04 || fail 'Requires Ubuntu 22.04+';;
 *) fail 'Requires Debian/Ubuntu';;
 esac
 for tool in curl openssl python3; do
  if ! command -v "$tool" >/dev/null; then
   apt-get update -qq
   DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates openssl python3
   break
  fi
 done
fi
for tool in openssl python3 timeout; do command -v "$tool" >/dev/null || fail "Missing $tool"; done
if (( VERIFY )); then WORK="$(mktemp -d)"; else WORK="$(mktemp -d /var/tmp/arcvpn-bootstrap.XXXXXXXX)"; fi
chmod 700 "$WORK"
trap 'rm -rf -- "$WORK"' EXIT
fetch(){
 local name="$1" limit="$2" target="$WORK/$1"
 if [[ -n "$FROM" ]]; then
  [[ -f "$FROM/$name" ]] || fail "Missing $name"
  head -c "$((limit+1))" -- "$FROM/$name" > "$target"
 else
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fLsS --connect-timeout 15 --max-time 180 --max-filesize "$limit" "$BASE/$name" | head -c "$((limit+1))" > "$target"
 fi
 (( $(wc -c < "$target") <= limit )) || fail "File too large: $name"
}
fetch "$FILE.manifest.json" 4096
fetch "$FILE.manifest.sig" 64
[[ $(wc -c < "$WORK/$FILE.manifest.sig") == 64 ]] || fail 'Invalid signature size'
cat > "$WORK/public.pem" <<'PEM'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAAdJGvB77glcTCEMmNLJF+Yac5kc1fWzfilW48+EAQSc=
-----END PUBLIC KEY-----
PEM
{ printf 'ArcVPN installer manifest v1\n'; cat "$WORK/$FILE.manifest.json"; } > "$WORK/signed"
openssl pkeyutl -verify -rawin -pubin -inkey "$WORK/public.pem" -sigfile "$WORK/$FILE.manifest.sig" -in "$WORK/signed" >/dev/null || fail 'Manifest signature rejected'
fetch "$FILE" 134217728
timeout --kill-after=5s 60s python3 - "$WORK" "$RELEASE_VERSION" "$FILE" <<'PY'
import gzip, hashlib, json, os, pathlib, re, sys, tarfile
work, version, filename = sys.argv[1:]
def require(ok, message):
    if not ok: raise ValueError(message)
def pairs(items):
    result = {}
    for key, value in items:
        require(key not in result, 'Duplicate JSON key')
        result[key] = value
    return result
def digest(data): return hashlib.sha256(data).hexdigest()
def read(name): return pathlib.Path(work, name).read_bytes()
manifest = json.loads(read(filename + '.manifest.json'), object_pairs_hook=pairs)
require(set(manifest) == {'format','version','platform','file','size','sha256','source_manifest_sha256'}, 'Unexpected manifest fields')
require(manifest['format'] == 'arcvpn-installer-v1' and manifest['version'] == version and manifest['platform'] == 'linux/amd64' and manifest['file'] == filename, 'Wrong release target')
require(type(manifest['size']) is int and 0 < manifest['size'] <= 134217728, 'Invalid archive size')
for field in ('sha256', 'source_manifest_sha256'):
    require(isinstance(manifest[field], str) and re.fullmatch('[0-9a-f]{64}', manifest[field]), 'Invalid digest')
archive = pathlib.Path(work, filename)
require(archive.stat().st_size == manifest['size'], 'Archive size mismatch')
with archive.open('rb') as stream:
    hasher = hashlib.sha256()
    for chunk in iter(lambda: stream.read(1048576), b''): hasher.update(chunk)
require(hasher.hexdigest() == manifest['sha256'], 'Archive hash mismatch')
allowed = set('dist/arcvpn-linux-amd64 dist/VERSION dist/BUILDINFO.json SOURCE_MANIFEST.sha256 install.sh deploy/arcvpn-menu.sh deploy/arcvpn-tunnel-net.sh README.md LICENSE docs/ops/install-and-update.md docs/ops/local-admin-recovery.md docs/ops/local-service-control.md docs/planning/short-work-blocks.md dist/SHA256SUMS dist/update-public.hex'.split())
# Bound decompression including headers/padding before tar parsing.
raw = pathlib.Path(work, 'payload.tar')
with gzip.open(archive, 'rb') as src, raw.open('xb') as dst:
    total = 0
    for chunk in iter(lambda: src.read(1048576), b''):
        total += len(chunk)
        require(total <= 100663296, 'Expanded archive too large')
        dst.write(chunk)
root = pathlib.Path(work, 'payload')
root.mkdir(mode=0o700)
hashes = {}
with tarfile.open(raw, 'r:') as bundle:
    members = []
    for item in bundle:
        require(len(members) < 32, 'Too many archive entries')
        require(item.name in allowed and item.name not in hashes, 'Unexpected or duplicate archive path')
        require(item.isreg() and not item.sparse and not item.pax_headers, 'Links/special files/extensions forbidden')
        require(not item.mode & 0o7022 and 0 <= item.size <= 54525952, 'Unsafe archive permissions/size')
        hashes[item.name] = None
        members.append(item)
    require(set(hashes) == allowed, 'Incomplete installer payload')
    for item in members:
        target = root / item.name
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'wb') as dst, bundle.extractfile(item) as src:
            hasher = hashlib.sha256()
            for chunk in iter(lambda: src.read(1048576), b''):
                dst.write(chunk); hasher.update(chunk)
        hashes[item.name] = hasher.hexdigest()
checks = {}
for line in (root / 'dist/SHA256SUMS').read_text().splitlines():
    match = re.fullmatch(r'([0-9a-f]{64})  (.+)', line)
    require(match is not None, 'Invalid internal checksum entry')
    value, name = match.groups()
    require(name in allowed and name != 'dist/SHA256SUMS' and name not in checks, 'Unsafe checksum path')
    checks[name] = value
require(checks == {key:value for key,value in hashes.items() if key != 'dist/SHA256SUMS'}, 'Internal checksum mismatch')
require(hashes['SOURCE_MANIFEST.sha256'] == manifest['source_manifest_sha256'], 'Source manifest mismatch')
require((root / 'dist/VERSION').read_text().strip() == version, 'Payload version mismatch')
info = json.loads((root / 'dist/BUILDINFO.json').read_bytes(), object_pairs_hook=pairs)
require(info['version'] == version and info['target'] == 'linux/amd64' and info['source_manifest_sha256'] == manifest['source_manifest_sha256'], 'Build metadata mismatch')
(root / 'dist/arcvpn-linux-amd64').chmod(0o700)
print('PASS: signature, pinned version, archive and payload integrity; safe extraction')
PY
if (( VERIFY )); then exit 0; fi
if ! bash "$WORK/payload/install.sh"; then
 fail 'Installer failed. Inspect diagnostics and restore the VPS snapshot if needed; do not reinstall over partial data.'
fi

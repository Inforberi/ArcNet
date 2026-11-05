#!/usr/bin/env bash
# Deterministic stage-4 source verification. --full additionally requires a
# genuine npm lockfile and runs browser E2E against an embedded React binary.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
mode="${1:---source}"
if [[ "$mode" != '--source' && "$mode" != '--full' || $# -gt 1 ]]; then
  printf 'Usage: %s [--source|--full]\n' "$0" >&2
  exit 64
fi
for required in go bash python3 node timeout; do
  command -v "$required" >/dev/null || { printf 'BLOCKED: missing %s\n' "$required" >&2; exit 2; }
done
work="$(mktemp -d -t arcvpn-stage4-verify.XXXXXXXX)"
created_embed=0
finish() {
  if [[ "$created_embed" == 1 ]]; then rm -rf -- "$ROOT/web/react-dist"; fi
  rm -rf -- "$work"
}
trap finish EXIT
printf '[1/8] Go unit and integration tests\n'
go test -count=1 ./...
printf '[2/8] Go static analysis\n'
go vet ./...
printf '[3/8] Security and concurrency race tests\n'
timeout --kill-after=5s 110s go test -race -count=1 ./internal/nodeagent ./internal/nodebootstrap ./internal/nodeprotocol ./internal/noderuntime ./internal/updater ./internal/store ./internal/nodeprobe
timeout --kill-after=5s 80s go test -race -count=1 ./internal/httpapi \
  -run 'Test(Stage4APIPollRevisionAndPermissions|NodeDrainIsAuthorizedCSRFProtectedAndDoesNotStopXray|ExternalProbeGrantCSRFAndReportReplay|Stage4UpgradeEndpointsRejectAnonymousAndUnconfirmed|Stage4SSHInstallFailsClosedWithoutSignedRelease)' -timeout 55s
printf '[4/8] Compile Go CLI\n'
go build -o "$work/arcvpn" ./cmd/arcvpn
printf '[5/8] Shell and Python syntax\n'
for script in install.sh scripts/*.sh deploy/*.sh; do bash -n "$script"; done
PYTHONPYCACHEPREFIX="$work/python-cache" python3 -m py_compile tests/react_e2e.py
printf 'SOURCE_CHECKS_PASS\n'
if [[ "$mode" == '--source' ]]; then
  printf 'Not a LOCAL_PASS: React typecheck/build/E2E and VPS handshakes were not verified.\n'
  exit 0
fi
printf '[6/8] Real npm dependency installation and React build\n'
command -v npm >/dev/null || { echo 'BLOCKED: npm missing' >&2; exit 2; }
if [[ ! -s frontend/package-lock.json ]]; then
  echo 'BLOCKED: npm-generated frontend/package-lock.json must be committed first (do not fabricate it)' >&2
  exit 2
fi
(cd frontend && npm ci --no-audit --no-fund && npm run typecheck && npm run build)
[[ -s frontend/dist/index.html ]] || { echo 'BLOCKED: Vite produced no index.html' >&2; exit 2; }
printf '[7/8] Embed React in a dedicated Go binary\n'
if [[ -e web/react-dist || -L web/react-dist ]]; then
  echo 'BLOCKED: web/react-dist already exists; refusing to replace another build artifact' >&2
  exit 2
fi
cp -a frontend/dist web/react-dist
created_embed=1
CGO_ENABLED=1 go build -tags arcvpn_embed_react -trimpath -buildvcs=false \
  -ldflags='-X github.com/arcvpn/arcvpn/internal/nodeagent.Version=0.6.5-stage4' \
  -o "$work/arcvpn-embedded" ./cmd/arcvpn
"$work/arcvpn-embedded" frontend-check
printf '[8/8] Playwright Chromium E2E (five viewports)\n'
ARCVPN_BINARY="$work/arcvpn-embedded" python3 tests/react_e2e.py
printf 'LOCAL_BUILD_AND_BROWSER_CHECKS_PASS\n'
printf 'NOT STAGE4_ACCEPTED: external VPS/systemd/Xray/VLESS client checks remain mandatory.\n'

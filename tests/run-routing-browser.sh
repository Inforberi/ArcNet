#!/usr/bin/env bash
# UI/API checks on a disposable local instance; no Xray/VPN/SSH operations.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
root="$PWD"
binary="${ARCVPN_BINARY:-$root/dist/arcvpn-linux-amd64}"
[[ -x "$binary" ]] || { echo 'Build an embedded Linux binary and set ARCVPN_BINARY.' >&2; exit 1; }
"$binary" frontend-check
umask 077
fixture="$(mktemp -d /tmp/arcvpn-routing-browser.XXXXXX)"
server_pid=''
cleanup() {
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  rm -rf "$fixture"
}
trap cleanup EXIT
export ARCVPN_DATA_DIR="$fixture/state"
"$binary" admin-init > "$fixture/admin-init.log"
sed -n 's/^Password: //p' "$fixture/admin-init.log" > "$fixture/password"
port="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    print(sock.getsockname()[1])
PY
)"
"$binary" serve --listen "127.0.0.1:$port" --public-url https://198.51.100.25:8443 > "$fixture/server.log" 2>&1 &
server_pid="$!"
python3 - "$port" <<'PY'
import sys, time, urllib.request
for _ in range(100):
    try:
        with urllib.request.urlopen(f'http://127.0.0.1:{sys.argv[1]}/healthz', timeout=.5) as response:
            if response.status == 204:
                break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('Disposable backend did not start')
PY
ARCVPN_E2E_URL="http://127.0.0.1:$port" ARCVPN_E2E_PASSWORD_FILE="$fixture/password" node tests/routing_e2e.mjs

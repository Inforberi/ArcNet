#!/usr/bin/env bash
# Disposable Linux UI/API fixtures, without starting or applying VPN engines.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
root="$PWD"
binary="${ARCVPN_BINARY:-$root/dist/arcvpn-linux-amd64}"
[[ -x "$binary" && -f "$root/frontend/dist/index.html" ]] || { echo 'Build the Linux binary and frontend/dist first.' >&2; exit 1; }
umask 077
fixture="$(mktemp -d /tmp/arcvpn-monitoring-browser.XXXXXX)"
server_pid=''
stop_server() {
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; server_pid=''; fi
}
cleanup() { stop_server; rm -rf "$fixture"; }
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
start_server() {
  "$binary" serve --listen "127.0.0.1:$port" --web-root "$root/frontend/dist" --public-url https://198.51.100.25:8443 > "$fixture/server.log" 2>&1 &
  server_pid="$!"
  python3 - "$port" <<'PY'
import sys,time,urllib.request
for _ in range(100):
    try:
        with urllib.request.urlopen(f'http://127.0.0.1:{sys.argv[1]}/healthz',timeout=.5) as response:
            if response.status == 204: break
    except OSError: time.sleep(.1)
else: raise SystemExit('Disposable backend did not start')
PY
}
export ARCVPN_E2E_URL="http://127.0.0.1:$port"
export ARCVPN_E2E_PASSWORD_FILE="$fixture/password"
start_server
ARCVPN_E2E_CONFIGURED=0 node tests/monitoring_e2e.mjs
stop_server
# A synthetic stored config exercises the additional-protocol form. This is
# fixture seeding, not a successfully deployed Reality listener or valid keys.
python3 - "$fixture/state/panel.sqlite" <<'PY'
import sqlite3,sys
with sqlite3.connect(sys.argv[1]) as db:
    db.execute('INSERT INTO config(id,address,port,target,sni,priv,pub,short_id,enabled) VALUES(1,?,?,?,?,?,?,?,1)',
               ('198.51.100.25',443,'www.microsoft.com','www.microsoft.com','A'*43,'B'*43,'0123456789abcdef'))
PY
start_server
ARCVPN_E2E_CONFIGURED=1 node tests/monitoring_e2e.mjs

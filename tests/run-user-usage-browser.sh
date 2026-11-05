#!/usr/bin/env bash
# Disposable UI/API fixture with deliberately unavailable Xray. No VPN apply.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
root="$PWD"
binary="${ARCVPN_BINARY:-$root/dist/arcvpn-linux-amd64}"
[[ -x "$binary" && -f frontend/dist/index.html ]] || { echo 'Build a Linux binary and frontend/dist first.' >&2; exit 1; }
umask 077
fixture="$(mktemp -d /tmp/arcvpn-user-usage.XXXXXX)"
server_pid=''
cleanup() {
  if [[ -n "$server_pid" ]];then kill "$server_pid" 2>/dev/null || true;wait "$server_pid" 2>/dev/null || true;fi
  rm -rf "$fixture"
}
trap cleanup EXIT
export ARCVPN_DATA_DIR="$fixture/state"
"$binary" admin-init > "$fixture/init.log"
sed -n 's/^Password: //p' "$fixture/init.log" > "$fixture/password"
python3 - "$fixture/state/panel.sqlite" <<'PY'
import sqlite3,sys
with sqlite3.connect(sys.argv[1]) as db:
    db.execute('INSERT INTO config(id,address,port,target,sni,priv,pub,short_id,enabled) VALUES(1,?,?,?,?,?,?,?,1)',
               ('198.51.100.25',443,'www.microsoft.com','www.microsoft.com','A'*43,'B'*43,'0123456789abcdef'))
    db.execute("INSERT INTO users(name,uuid,token,enabled,created_at) VALUES(?,?,?,1,strftime('%s','now'))",
               ('Usage fixture','11111111-1111-4111-8111-111111111111','t'*43))
PY
port="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(('127.0.0.1',0))
    print(sock.getsockname()[1])
PY
)"
"$binary" serve --listen "127.0.0.1:$port" --web-root "$root/frontend/dist" --xray "$fixture/missing-xray" --public-url https://198.51.100.25:8443 > "$fixture/server.log" 2>&1 &
server_pid="$!"
python3 - "$port" <<'PY'
import sys,time,urllib.request
for _ in range(100):
    try:
        with urllib.request.urlopen(f'http://127.0.0.1:{sys.argv[1]}/healthz',timeout=.5) as response:
            if response.status==204:break
    except OSError:time.sleep(.1)
else:raise SystemExit('Disposable backend did not start')
PY
ARCVPN_E2E_URL="http://127.0.0.1:$port" ARCVPN_E2E_PASSWORD_FILE="$fixture/password" node tests/user_usage_e2e.mjs

#!/usr/bin/env bash
# Launch the Go API and Vite development frontend on one local machine.
# Requires Go, a C compiler, SQLite and libqrencode, Node.js and npm.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
DATA_DIR="${ARCVPN_DEV_DATA_DIR:-$ROOT/.local/data}"
mkdir -p "$DATA_DIR" "$ROOT/.local/bin"
chmod 0700 "$DATA_DIR"
export ARCVPN_DATA_DIR="$DATA_DIR"
command -v go >/dev/null || { echo 'Go is required' >&2;exit 1; }
command -v npm >/dev/null || { echo 'Node.js/npm are required' >&2;exit 1; }
if [[ ! -f "$DATA_DIR/panel.sqlite" ]];then
  echo '[local] Initializing developer administrator; save the generated password'
  go run ./cmd/arcvpn admin-init
fi
if [[ ! -d frontend/node_modules ]];then
  echo '[local] Installing React dependencies (requires npm registry)'
  (cd frontend && npm install --no-audit --no-fund)
fi
printf '[local] Building Go backend\n'
go build -o "$ROOT/.local/bin/arcvpn" ./cmd/arcvpn
"$ROOT/.local/bin/arcvpn" serve --listen 127.0.0.1:8181 --legacy-ui --xray "$ROOT/.local/bin/xray" --xray-config "$DATA_DIR/xray.json" &
API_PID=$!
cleanup(){ kill "$API_PID" 2>/dev/null || true;wait "$API_PID" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
printf '[local] React UI: http://127.0.0.1:5173\n'
(cd frontend && npm run dev)

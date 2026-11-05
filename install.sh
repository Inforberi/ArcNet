#!/usr/bin/env bash
# ArcVPN preview installer for a fresh Debian/Ubuntu x86_64 VPS.
# Installation changes system services and creates a narrowly-scoped root helper.
set -Eeuo pipefail
umask 077
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="/var/lib/arcvpn"
OPT="/opt/arcvpn"
CONFIG_DIR="/etc/arcvpn"
CERTBOT_DIR="/opt/arcvpn/certbot"
SYSTEMD="/etc/systemd/system"
PUBLIC_IP="${ARCVPN_PUBLIC_IP:-}"
PANEL_PORT="${ARCVPN_PANEL_PORT:-8443}"

err(){ printf '\033[31mERROR: %s\033[0m\n' "$*" >&2;exit 1; }
info(){ printf '\033[36m%s\033[0m\n' "$*"; }
trap 'printf "Installation interrupted at line %s; inspect systemctl/journalctl before retrying.\n" "$LINENO" >&2' ERR
[[ $EUID -eq 0 ]] || err "Run: sudo bash install.sh"
[[ -r /etc/os-release ]] || err "Unsupported OS"
# shellcheck disable=SC1091
source /etc/os-release
case "$ID" in debian|ubuntu) ;; *) err "Only Debian and Ubuntu are supported";;esac
if [[ "$ID" == "ubuntu" ]];then
 dpkg --compare-versions "$VERSION_ID" ge "22.04" || err "Ubuntu 22.04 or newer is required by the precompiled binary"
fi
if [[ "$ID" == "debian" ]];then
 dpkg --compare-versions "$VERSION_ID" ge "12" || err "Debian 12 or newer is required by the precompiled binary"
fi
[[ "$(uname -m)" == "x86_64" ]] || err "The packaged binary currently supports Linux x86_64 only"
[[ "$PANEL_PORT" =~ ^[0-9]+$ ]] && ((PANEL_PORT>=1024 && PANEL_PORT<=65535)) || err "Invalid panel port"
[[ -s "$ROOT/dist/SHA256SUMS" ]] || err "Missing release integrity manifest"
(cd "$ROOT" && sha256sum --status -c dist/SHA256SUMS) || err "Release files have failed integrity validation"
[[ -f "$ROOT/dist/arcvpn-linux-amd64" ]] || err "Missing packaged binary: dist/arcvpn-linux-amd64"
EXPECTED_BIN_SHA="$(awk '$2=="dist/arcvpn-linux-amd64"||$2=="arcvpn-linux-amd64"{print $1}' "$ROOT/dist/SHA256SUMS" | head -1)"
[[ "$EXPECTED_BIN_SHA" =~ ^[0-9a-f]{64}$ ]] || err "Missing valid binary SHA256"
ACTUAL_BIN_SHA="$(sha256sum "$ROOT/dist/arcvpn-linux-amd64" | awk '{print $1}')"
[[ "$EXPECTED_BIN_SHA" == "$ACTUAL_BIN_SHA" ]] || err "Packaged Go binary digest mismatch"
# Reject incomplete AWG3.1 releases before any package/service changes.
for name in awg awg-quick amneziawg-go; do
 file="dist/${name}-linux-amd64"
 [[ -s "$ROOT/$file" ]] || err "Missing AWG3.1 release artifact: $file"
 grep -qE "^[0-9a-f]{64}  $file$" "$ROOT/dist/SHA256SUMS" || err "Missing checksum entry: $file"
done
for name in amneziawg-tools-v3.1.20260812 amneziawg-go-v3.1.20260828; do
 file="dist/sources/$name.tar.gz"
 [[ -s "$ROOT/$file" ]] || err "Missing corresponding source: $file"
 grep -qE "^[0-9a-f]{64}  $file$" "$ROOT/dist/SHA256SUMS" || err "Missing checksum entry: $file"
done
[[ -s "$ROOT/dist/AMNEZIAWG-SOURCES.txt" ]] || err 'Missing AWG licensing/source notice'
[[ -d /run/systemd/system ]] || err "systemd must be running"
[[ ! -e "$SYSTEMD/arcvpn-panel.service" ]] || err "Existing ArcVPN installation detected; install.sh does not overwrite existing data."
for bin in curl python3 ss systemctl;do command -v "$bin" >/dev/null || { apt-get update -qq;apt-get install -y curl python3 iproute2 ca-certificates;break;};done
if [[ -z "$PUBLIC_IP" ]]; then
 PUBLIC_IP="$(curl -fLsS --connect-timeout 5 --max-time 12 https://api.ipify.org || true)"
fi
python3 - "$PUBLIC_IP" <<'PY'
import ipaddress,sys
try:
 ip=ipaddress.ip_address(sys.argv[1]); assert ip.is_global and isinstance(ip,ipaddress.IPv4Address)
except (ValueError,AssertionError):
 print('Public globally routable IP required. Set ARCVPN_PUBLIC_IP before installing.',file=sys.stderr)
 sys.exit(1)
PY
info "Public IP: $PUBLIC_IP"
# Fail before touching services if control ports are occupied.
if ss -Hltn "sport = :$PANEL_PORT" | grep -q .;then err "HTTPS port $PANEL_PORT already in use";fi
if ss -Hltn 'sport = :80' | grep -q .;then err "Port 80 is in use; standalone ACME IP validation requires it free";fi
if ss -Hltn 'sport = :443' | grep -q .;then
 info "WARNING: TCP/443 is already in use. Choose another VPN port in the UI. Existing service will not be changed."
fi
info "Installing runtime dependencies"
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ca-certificates curl unzip python3-venv libsqlite3-0 libqrencode4 ufw sqlite3 iproute2 iptables procps wireguard-tools
# Runtime shared libraries are needed even for the lightweight frontend-check command.
"$ROOT/dist/arcvpn-linux-amd64" frontend-check >/dev/null || err "The binary has no valid embedded React frontend. Build release with scripts/build-release.sh"
if ! getent group arcvpn >/dev/null; then groupadd --system arcvpn;fi
if ! getent passwd arcvpn >/dev/null; then useradd --system --gid arcvpn --home-dir "$RUN_DIR" --shell /usr/sbin/nologin arcvpn;fi
install -d -m 0755 "$OPT/bin"
install -d -m 0700 -o arcvpn -g arcvpn "$RUN_DIR"
install -d -m 0700 -o arcvpn -g arcvpn "$RUN_DIR/update"
install -d -m 0750 -o root -g arcvpn "$CONFIG_DIR"
install -d -m 0750 -o root -g arcvpn "$CONFIG_DIR/tls"
install -m 0755 "$ROOT/dist/arcvpn-linux-amd64" "$OPT/bin/arcvpn-core"
# Public verification key must be generated by the release owner and distributed
# through a separately authenticated channel. Never ship the signing private key.
if [[ -s "$ROOT/dist/update-public.hex" ]]; then
  [[ "$(wc -c < "$ROOT/dist/update-public.hex")" -le 256 ]] || err 'Invalid update signing public key'
  install -m 0644 "$ROOT/dist/update-public.hex" "$CONFIG_DIR/update-public.hex"
  info 'Signed offline updates enabled (trusted release public key installed)'
else
  info 'Signed updates disabled until a trusted public key is installed'
fi
install -m 0755 "$ROOT/deploy/arcvpn-tunnel-net.sh" "$OPT/bin/arcvpn-tunnel-net"
info "Verified embedded React/Vite frontend"
# AmneziaWG dependencies are built into the release on a trusted CI runner,
# so a 1 GB VPS does not need Git, Go or a compiler, and no PPA is added.
# All files must have matching SHA256 entries in the release manifest.
for name in awg awg-quick amneziawg-go; do
 src="dist/${name}-linux-amd64"
 [[ -s "$ROOT/$src" ]] || err "Missing bundled AmneziaWG 3.1 component: $src"
 grep -qE "^[0-9a-f]{64}  $src$" "$ROOT/dist/SHA256SUMS" || err "Missing SHA256 for $src"
 install -m 0755 "$ROOT/$src" "$OPT/bin/$name"
done
"$OPT/bin/amneziawg-go" --version | grep -F 'v3.1.20260828' >/dev/null || err 'AmneziaWG userspace version mismatch'
# Source distribution and license manifest accompany the release.
install -d -m 0755 "$OPT/sources"
for src in "$ROOT"/dist/sources/amneziawg-{tools-v3.1.20260812,go-v3.1.20260828}.tar.gz; do
 [[ -s "$src" ]] || err "Missing upstream AmneziaWG source: $src"
 install -m 0644 "$src" "$OPT/sources/$(basename "$src")"
done
install -m 0644 "$ROOT/dist/AMNEZIAWG-SOURCES.txt" "$OPT/AMNEZIAWG-SOURCES.txt"
info "Installed pinned AmneziaWG 3.1 userspace tools and daemon"
info "Installing verified Xray release"
XRAY_VERSION="${ARCVPN_XRAY_VERSION:-}"
if [[ -z "$XRAY_VERSION" ]];then
 XRAY_VERSION="$(curl -fLsS --connect-timeout 8 --max-time 30 https://api.github.com/repos/XTLS/Xray-core/releases/latest | python3 -c 'import json,sys;print(json.load(sys.stdin)["tag_name"])')"
fi
[[ "$XRAY_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || err "Unexpected Xray release version: $XRAY_VERSION"
TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT
URL="https://github.com/XTLS/Xray-core/releases/download/$XRAY_VERSION/Xray-linux-64.zip"
curl -fL --retry 2 --connect-timeout 10 --max-time 180 "$URL" -o "$TMP/xray.zip"
curl -fL --retry 2 --connect-timeout 10 --max-time 45 "$URL.dgst" -o "$TMP/xray.zip.dgst"
EXPECTED="$(awk -F '=' '/^[[:space:]]*SHA(2-)?256[[:space:]]*=/{gsub(/ /,"",$2);print tolower($2)}' "$TMP/xray.zip.dgst" | head -1)"
[[ "$EXPECTED" =~ ^[0-9a-f]{64}$ ]] || err "No SHA256 digest found for Xray release"
ACTUAL="$(sha256sum "$TMP/xray.zip" | awk '{print $1}')"
[[ "$EXPECTED" == "$ACTUAL" ]] || err "Xray release checksum does not match"
unzip -qq "$TMP/xray.zip" xray -d "$TMP"
install -m 0755 "$TMP/xray" "$OPT/bin/xray"
"$OPT/bin/xray" version | head -2
# Hysteria is optional at install time but is provisioned from a pinned,
# SHA256-verified release. Never execute an unverified remote installation script.
# A missing or unverifiable release MUST NOT silently install untrusted code.
if [[ "${ARCVPN_SKIP_HYSTERIA:-0}" != "1" ]]; then
 info "Installing SHA256-verified Hysteria2 release"
 HYSTERIA_META="$TMP/hysteria.json"
 curl -fLsS --retry 2 --connect-timeout 10 --max-time 40 \
   https://api.github.com/repos/HyNetworks/hysteria/releases/latest -o "$HYSTERIA_META"
 readarray -t HYSTERIA_RELEASE < <(python3 - "$HYSTERIA_META" <<'PYJSON'
import json,sys,re
release=json.load(open(sys.argv[1],encoding='utf-8'))
asset=next((a for a in release.get('assets',[]) if a.get('name')=='hysteria-linux-amd64'),None)
if not asset or not re.fullmatch(r'sha256:[0-9a-f]{64}',asset.get('digest') or ''):
 raise SystemExit('Missing validated SHA256 for Hysteria Linux amd64 asset')
url=asset.get('browser_download_url','')
if not url.startswith('https://github.com/HyNetworks/hysteria/releases/download/'):
 raise SystemExit('Unexpected Hysteria download source')
print(url)
print(asset['digest'].removeprefix('sha256:'))
PYJSON
)
 [[ "${#HYSTERIA_RELEASE[@]}" -eq 2 ]] || err "Cannot verify Hysteria release"
 curl -fL --retry 2 --connect-timeout 10 --max-time 180 "${HYSTERIA_RELEASE[0]}" -o "$TMP/hysteria"
 echo "${HYSTERIA_RELEASE[1]}  $TMP/hysteria" | sha256sum --status -c - || err "Hysteria SHA256 mismatch"
 install -m 0755 "$TMP/hysteria" "$OPT/bin/hysteria"
 "$OPT/bin/hysteria" version
fi
# Enable IPv4 forwarding for full-tunnel VPN clients, including after reboot.
# ArcVPN does not advertise IPv6 tunnel routes until IPv6 forwarding is supported.
install -m 0644 /dev/stdin /etc/sysctl.d/90-arcvpn-forward.conf <<'SYSCTL'
net.ipv4.ip_forward=1
SYSCTL
sysctl -w net.ipv4.ip_forward=1 >/dev/null
# AmneziaWG 3.1 uses the separately packaged userspace daemon and awg-quick.
# No DKMS build or external APT repository is needed by the VPS installer.
# Port 80 must be reachable for ACME; do not enable UFW if currently disabled.
if ufw status | grep -q '^Status: active';then
 ufw allow 80/tcp comment 'ArcVPN ACME validation'
fi
info "Requesting trusted Let's Encrypt IP certificate (port 80 must be reachable)"
python3 -m venv "$CERTBOT_DIR"
"$CERTBOT_DIR/bin/python" -m pip install --disable-pip-version-check --quiet 'certbot==5.9.0'
"$CERTBOT_DIR/bin/certbot" certonly --standalone --non-interactive --agree-tos --register-unsafely-without-email --preferred-profile shortlived --ip-address "$PUBLIC_IP"
FULLCHAIN="/etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem"
PRIVATEKEY="/etc/letsencrypt/live/$PUBLIC_IP/privkey.pem"
[[ -r "$FULLCHAIN" && -r "$PRIVATEKEY" ]] || err "Certbot did not create expected certificate files"
install -m 0640 -o root -g arcvpn "$FULLCHAIN" "$CONFIG_DIR/tls/fullchain.pem"
install -m 0640 -o root -g arcvpn "$PRIVATEKEY" "$CONFIG_DIR/tls/privkey.pem"
info "Configuring isolated ArcVPN journal namespace (other services unaffected)"
install -d -m 0755 /etc/systemd/journald@arcvpn.conf.d
cat > /etc/systemd/journald@arcvpn.conf.d/90-arcvpn-budget.conf <<'JOURNAL'
[Journal]
Storage=persistent
SystemMaxUse=200M
RuntimeMaxUse=200M
SystemMaxFileSize=16M
RuntimeMaxFileSize=16M
JOURNAL
chmod 0644 /etc/systemd/journald@arcvpn.conf.d/90-arcvpn-budget.conf
info "Creating systemd services"
cat > "$SYSTEMD/arcvpn-panel.service" <<UNIT
[Unit]
Description=ArcVPN controller and web interface
After=network-online.target
Wants=network-online.target
[Service]
LogNamespace=arcvpn
Type=simple
User=arcvpn
Group=arcvpn
WorkingDirectory=$RUN_DIR
ExecStart=$OPT/bin/arcvpn-core serve --listen 0.0.0.0:$PANEL_PORT --tls-cert $CONFIG_DIR/tls/fullchain.pem --tls-key $CONFIG_DIR/tls/privkey.pem --xray $OPT/bin/xray --xray-config $RUN_DIR/xray.json --xray-service arcvpn-xray.service --control-socket /run/arcvpn-control.sock --public-url https://$PUBLIC_IP:$PANEL_PORT
Restart=on-failure
RestartSec=3
Environment=ARCVPN_DATA_DIR=$RUN_DIR
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$RUN_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
RestrictSUIDSGID=true
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat > "$SYSTEMD/arcvpn-xray.service" <<UNIT
[Unit]
Description=ArcVPN managed Xray-core
After=network-online.target
Wants=network-online.target
ConditionPathExists=$RUN_DIR/xray.json
[Service]
LogNamespace=arcvpn
Type=simple
User=arcvpn
Group=arcvpn
ExecStart=$OPT/bin/xray run -config $RUN_DIR/xray.json
Restart=on-failure
RestartSec=2
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$RUN_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
RestrictSUIDSGID=true
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat > "$SYSTEMD/arcvpn-hysteria.service" <<UNIT
[Unit]
Description=ArcVPN Hysteria2 UDP server
After=network-online.target
Wants=network-online.target
ConditionPathExists=$RUN_DIR/hysteria.yaml
[Service]
LogNamespace=arcvpn
Type=simple
User=arcvpn
Group=arcvpn
ExecStart=$OPT/bin/hysteria server -c $RUN_DIR/hysteria.yaml
Restart=on-failure
RestartSec=2
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$RUN_DIR
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat > "$SYSTEMD/arcvpn-wireguard.service" <<UNIT
[Unit]
Description=ArcVPN WireGuard interface
After=network-online.target
Wants=network-online.target
ConditionPathExists=$RUN_DIR/arcwg.conf
[Service]
LogNamespace=arcvpn
Type=oneshot
RemainAfterExit=yes
ExecStart=$OPT/bin/arcvpn-core tunnel up wireguard
ExecStartPost=$OPT/bin/arcvpn-tunnel-net up arcwg 10.76.0.0/16
ExecStop=$OPT/bin/arcvpn-core tunnel down wireguard
ExecStopPost=$OPT/bin/arcvpn-tunnel-net down arcwg 10.76.0.0/16
TimeoutStartSec=20
TimeoutStopSec=20
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat > "$SYSTEMD/arcvpn-amneziawg.service" <<UNIT
[Unit]
Description=ArcVPN AmneziaWG interface
After=network-online.target
Wants=network-online.target
ConditionPathExists=$RUN_DIR/arcawg.conf
[Service]
LogNamespace=arcvpn
Type=oneshot
RemainAfterExit=yes
ExecStart=$OPT/bin/arcvpn-core tunnel up amneziawg
ExecStartPost=$OPT/bin/arcvpn-tunnel-net up arcawg 10.77.0.0/16
ExecStop=$OPT/bin/arcvpn-core tunnel down amneziawg
ExecStopPost=$OPT/bin/arcvpn-tunnel-net down arcawg 10.77.0.0/16
TimeoutStartSec=20
TimeoutStopSec=20
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat > "$OPT/bin/arcvpn-cert-deploy" <<DEPLOY
#!/usr/bin/env bash
set -Eeuo pipefail
install -m 0640 -o root -g arcvpn "$FULLCHAIN" "$CONFIG_DIR/tls/fullchain.pem"
install -m 0640 -o root -g arcvpn "$PRIVATEKEY" "$CONFIG_DIR/tls/privkey.pem"
/usr/bin/systemctl try-restart arcvpn-panel.service
DEPLOY
chmod 0755 "$OPT/bin/arcvpn-cert-deploy"
cat > "$SYSTEMD/arcvpn-control.socket" <<'UNIT'
[Unit]
Description=ArcVPN privileged one-operation control socket
[Socket]
ListenStream=/run/arcvpn-control.sock
SocketGroup=arcvpn
SocketMode=0660
RemoveOnStop=true
[Install]
WantedBy=sockets.target
UNIT
cat > "$SYSTEMD/arcvpn-control.service" <<UNIT
[Unit]
Description=ArcVPN restricted systemd control helper
Requires=arcvpn-control.socket
[Service]
Type=simple
ExecStart=$OPT/bin/arcvpn-core helper
User=root
Group=root
StandardInput=null
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
RestrictSUIDSGID=true
ReadWritePaths=/etc/systemd/journald@arcvpn.conf.d /etc/ufw
UMask=0077
UNIT
cat > "$SYSTEMD/arcvpn-update.service" <<UNIT
[Unit]
Description=ArcVPN verified application update with rollback
After=network-online.target
[Service]
Type=oneshot
User=root
Group=root
ExecStart=$OPT/bin/arcvpn-core update-execute
TimeoutStartSec=90
RuntimeDirectory=arcvpn-update
RuntimeDirectoryMode=0700
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$OPT/bin $RUN_DIR/update /run/arcvpn-update
UMask=0077
UNIT

cat > "$SYSTEMD/arcvpn-cert-renew.service" <<UNIT
[Unit]
Description=Renew Let's Encrypt IP certificate for ArcVPN
[Service]
LogNamespace=arcvpn
Type=oneshot
ExecStart=$CERTBOT_DIR/bin/certbot renew --quiet --deploy-hook $OPT/bin/arcvpn-cert-deploy
UNIT
cat > "$SYSTEMD/arcvpn-cert-renew.timer" <<'UNIT'
[Unit]
Description=Check ArcVPN short-lived TLS certificate regularly
[Timer]
OnCalendar=*-*-* 00/8:13:00
RandomizedDelaySec=30m
Persistent=true
[Install]
WantedBy=timers.target
UNIT
# Encrypted scheduled backups are opt-in. The owner must provide the password
# through the terminal menu, and must copy backups outside this VPS.
install -d -m 0700 -o arcvpn -g arcvpn /var/backups/arcvpn
cat > "$SYSTEMD/arcvpn-backup.service" <<UNIT
[Unit]
Description=Create an encrypted ArcVPN VPN-state snapshot
ConditionPathExists=$RUN_DIR/backup-password
[Service]
LogNamespace=arcvpn
Type=oneshot
User=arcvpn
Group=arcvpn
Environment=ARCVPN_DATA_DIR=$RUN_DIR
ExecStart=$OPT/bin/arcvpn-core backup-create --directory /var/backups/arcvpn --password-file $RUN_DIR/backup-password --keep 7
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=$RUN_DIR /var/backups/arcvpn
UMask=0077
UNIT
cat > "$SYSTEMD/arcvpn-backup.timer" <<'UNIT'
[Unit]
Description=Daily encrypted ArcVPN backups (opt-in)
[Timer]
OnCalendar=daily
RandomizedDelaySec=20m
Persistent=true
[Install]
WantedBy=timers.target
UNIT
install -m 0755 "$ROOT/deploy/arcvpn-menu.sh" /usr/local/bin/arcvpn
systemctl daemon-reload
info "Initializing administrator (password will be printed once)"
runuser -u arcvpn -- env ARCVPN_DATA_DIR="$RUN_DIR" "$OPT/bin/arcvpn-core" admin-init
info "Configuring firewall rules without changing SSH policy"
# When UFW is already active, apply only additive rules.
# When UFW is disabled, DO NOT enable it automatically and risk SSH lockout.
if ufw status | grep -q '^Status: active';then
 ufw allow 80/tcp comment 'ArcVPN ACME validation'
 ufw allow "$PANEL_PORT"/tcp comment 'ArcVPN HTTPS admin'
 info "UFW active: allowed ACME and HTTPS. VPN port opens after manual confirmation."
else
 info "UFW is inactive. Leaving it inactive to avoid locking out SSH."
fi
systemctl enable --now arcvpn-control.socket arcvpn-panel.service arcvpn-cert-renew.timer
systemctl is-active --quiet arcvpn-panel.service || err "Panel service failed; run journalctl -u arcvpn-panel"
info "Installation complete: https://$PUBLIC_IP:$PANEL_PORT"
info "To manage the service run: arcvpn"
info "VPN protocol ports can be opened in ArcVPN -> Security -> Firewall when UFW is active."

#!/usr/bin/env bash
# One-time recovery for A4 stopped at Certbot, before units or database creation.
set -Eeuo pipefail
umask 077
fail(){ printf 'Recovery stopped: %s\n' "$*" >&2; exit 1; }
[[ $EUID == 0 && $(uname -s) == Linux ]] || fail 'Requires Linux root'
PUBLIC_IP="${ARCVPN_PUBLIC_IP:?Set the IP used to obtain the certificate}"
PANEL_PORT=8443
RUN_DIR=/var/lib/arcvpn
OPT=/opt/arcvpn
CONFIG_DIR=/etc/arcvpn
CERTBOT_DIR=/opt/arcvpn/certbot
SYSTEMD=/etc/systemd/system
exec 9>/run/arcvpn-a4-recovery.lock
flock -n 9 || fail 'Recovery already running'
! compgen -G "$SYSTEMD/arcvpn-*" >/dev/null || fail 'ArcVPN units already exist; inspect them first'
[[ -d "$RUN_DIR" && ! -L "$RUN_DIR" ]] || fail 'Missing partial A4 state directory'
[[ -z $(find "$RUN_DIR" -mindepth 1 ! -type d -print -quit) ]] || fail 'State contains files; refusing to overwrite data'
[[ -x "$OPT/bin/arcvpn-core" && -x "$CERTBOT_DIR/bin/certbot" ]] || fail 'Missing partial installation'
! ss -Hltn 'sport = :8443' | grep -q . || fail 'Port 8443 occupied'
FULLCHAIN="/etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem"
PRIVATEKEY="/etc/letsencrypt/live/$PUBLIC_IP/privkey.pem"
openssl x509 -in "$FULLCHAIN" -noout -checkip "$PUBLIC_IP" >/dev/null || fail 'Certificate IP mismatch'
openssl x509 -in "$FULLCHAIN" -noout -checkend 86400 >/dev/null || fail 'Certificate expires too soon'
cert_key=$(openssl x509 -in "$FULLCHAIN" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum)
private_key=$(openssl pkey -in "$PRIVATEKEY" -pubout -outform DER | sha256sum)
[[ "$cert_key" == "$private_key" ]] || fail 'Certificate and private key mismatch'
WORK=$(mktemp -d /var/tmp/arcvpn-a4-recovery.XXXXXXXX)
ROOT="$WORK/payload"
mkdir "$ROOT"
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fLsS --connect-timeout 15 --max-time 180 --max-filesize 134217728 \
 https://github.com/Inforberi/ArcNet/releases/download/v0.7.0-stage5-a4/arcvpn-0.7.0-stage5-a4-linux-amd64.tar.gz -o "$WORK/a4.tar.gz"
echo "491a46ce4fbd944ede22e48e7cd67d74d551a547de0797dec8a21dcfbc2a0054  $WORK/a4.tar.gz" | sha256sum -c -
tar -xzf "$WORK/a4.tar.gz" -C "$ROOT"
cmp -s "$OPT/bin/arcvpn-core" "$ROOT/dist/arcvpn-linux-amd64" || fail 'Installed core differs from A4'
[[ $(sed -n '181p' "$ROOT/install.sh") == 'FULLCHAIN="/etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem"' ]] || fail 'Unexpected A4 continuation'
[[ ! -L "$OPT" && $(stat -c %u "$OPT") == 0 ]] || fail 'Unexpected installation directory owner/type'
chmod 0755 "$OPT"
runuser -u arcvpn -- "$OPT/bin/arcvpn-core" frontend-check >/dev/null
# Apply IPv4 only to this private Certbot process, retaining normal TLS checks.
install -d -m 0755 "$SYSTEMD/arcvpn-cert-renew.service.d"
cat > "$SYSTEMD/arcvpn-cert-renew.service.d/ipv4.conf" <<UNIT
[Service]
ExecStart=
ExecStart=/opt/arcvpn/certbot/bin/python -c "import runpy; import urllib3.util.connection as c; c.HAS_IPV6 = False; runpy.run_path('/opt/arcvpn/certbot/bin/certbot', run_name='__main__')" renew --cert-name $PUBLIC_IP --quiet --deploy-hook /opt/arcvpn/bin/arcvpn-cert-deploy
UNIT
info(){ printf '%s\n' "$*"; }
err(){ fail "$*"; }
sed -n '181,$p' "$ROOT/install.sh" > "$WORK/finish.sh"
printf 'Continuing verified A4; private installation log: %s/install.log\n' "$WORK"
# Includes admin-init: never echo this log, which contains the initial password.
# Calling source in an if condition disables errexit inside the entire installer.
set +e
( set -e; source "$WORK/finish.sh" ) > "$WORK/install.log" 2>&1
result=$?
set -e
if (( result != 0 )); then
 fail "Continuation failed; inspect private log $WORK/install.log. Do not rerun."
fi
printf 'Installation commands completed; verify panel stability and HTTPS separately.\nPrivate credentials: %s/install.log\n' "$WORK"

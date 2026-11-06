#!/usr/bin/env bash
# Disposable Linux container only; exercises refusals without network or services.
set -euo pipefail
[[ -f /.dockerenv ]] || { echo 'Run only in a disposable Docker container' >&2; exit 1; }
script=/work/deploy/recover-a4-certificate.sh
export ARCVPN_PUBLIC_IP=192.0.2.1
expect_failure(){
 local output
 if output=$(bash "$script" 2>&1); then echo 'Unexpected success' >&2; exit 1; fi
 [[ "$output" == *"$1"* ]] || { printf '%s\n' "$output" >&2; exit 1; }
}
mkdir -p /etc/systemd/system
expect_failure 'Missing partial A4 state directory'
mkdir -p /var/lib/arcvpn/update
expect_failure 'Missing partial installation'
touch /var/lib/arcvpn/state.db
expect_failure 'State contains files'
touch /etc/systemd/system/arcvpn-panel.service
expect_failure 'ArcVPN units already exist'
test ! -e /etc/systemd/system/arcvpn-cert-renew.service.d
echo 'PASS: missing state/runtime, existing database/units refused before mutation'
archive=/work/dist/installer-0.7.0-stage5-a4/arcvpn-0.7.0-stage5-a4-linux-amd64.tar.gz
echo "491a46ce4fbd944ede22e48e7cd67d74d551a547de0797dec8a21dcfbc2a0054  $archive" | sha256sum -c -
tar -xOf "$archive" install.sh > /tmp/a4-install.sh
[[ $(sed -n '181p' /tmp/a4-install.sh) == 'FULLCHAIN="/etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem"' ]]
sed -n '181,$p' /tmp/a4-install.sh > /tmp/a4-finish.sh
bash -n /tmp/a4-finish.sh
echo 'PASS: pinned A4 archive, continuation boundary and shell syntax'
OPT=/tmp/arcvpn-permissions
mkdir -m 0700 "$OPT"
permission_command=$(sed -n '/^install -d -m 0755 "\$OPT" "\$OPT\/bin"$/p' /work/install.sh)
[[ -n "$permission_command" ]]
eval "$permission_command"
[[ $(stat -c %a "$OPT") == 755 ]]
install -m 0755 /bin/true "$OPT/bin/probe"
runuser -u nobody -- "$OPT/bin/probe"
echo 'PASS: installer repairs parent traversal for an unprivileged service user'
WORK=/tmp/arcvpn-errexit
mkdir "$WORK"
printf 'false\ntouch /tmp/arcvpn-unexpected-continuation\n' > "$WORK/finish.sh"
sed -n '/^set +e$/,/^set -e$/p' "$script" > "$WORK/run.sh"
source "$WORK/run.sh"
[[ $result != 0 && ! -e /tmp/arcvpn-unexpected-continuation ]]
echo 'PASS: continuation stops at the first error, including inside sourced code'
python3 - <<'PY'
import pathlib, runpy, sys, types
text = pathlib.Path('/work/install.sh').read_text()
wrapper = text.split("<<'CERTBOT'\n", 1)[1].split('\nCERTBOT\n', 1)[0]
connection = types.ModuleType('urllib3.util.connection')
connection.HAS_IPV6 = True
urllib3 = types.ModuleType('urllib3')
urllib3.util = types.ModuleType('urllib3.util')
urllib3.util.connection = connection
sys.modules.update({'urllib3': urllib3, 'urllib3.util': urllib3.util, 'urllib3.util.connection': connection})
calls = []
def invoke(path, run_name):
    assert connection.HAS_IPV6 is False
    calls.append((path, run_name))
runpy.run_path = invoke
exec(compile(wrapper, 'arcvpn-certbot', 'exec'), {})
assert calls == [('/opt/arcvpn/certbot/bin/certbot', '__main__')]
del connection.HAS_IPV6
try:
    exec(compile(wrapper, 'arcvpn-certbot', 'exec'), {})
except RuntimeError:
    pass
else:
    raise AssertionError('Unknown urllib3 API must fail closed')
assert len(calls) == 1
print('PASS: Certbot wrapper selects IPv4 before dispatch and rejects unknown API')
PY

#!/usr/bin/env bash
# One-time core/UI transition from the A5/A6 helper with missing update dispatch.
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Run in the root Termius shell.' >&2; exit 77; }
core=/opt/arcvpn/bin/arcvpn-core
version="$($core version)"
case "$version" in
  'arcvpn 0.7.0-stage5-a5'|'arcvpn 0.7.0-stage5-a6') ;;
  *) echo 'This transition supports only installed A5/A6. Do not reinstall.' >&2; exit 65 ;;
esac
exec 9>/run/arcvpn-a7-transition.lock
flock -n 9 || { echo 'Another transition is running.' >&2; exit 75; }
case "$(systemctl show --value -p ActiveState arcvpn-update.service)" in
  inactive|failed) ;;
  *) echo 'Update unit is busy or unavailable.' >&2; exit 75 ;;
esac
stage=/var/lib/arcvpn/update/staged.zip
test -d /var/lib/arcvpn/update
test ! -L /var/lib/arcvpn/update
test ! -e "$stage" && test ! -L "$stage"
d=$(mktemp -d /var/tmp/arcvpn-a7-update.XXXXXXXX)
trap 'rm -rf -- "$d"' EXIT
printf '%s\n' 01d246bc1efb82571308432634b245f9869ce647357d6cdf8a55b8f3e1004127 > "$d/public.hex"
# Keep the independent, already installed verification identity.
test ! -L /etc/arcvpn/update-public.hex
cmp "$d/public.hex" /etc/arcvpn/update-public.hex
curl -4 -fLsS --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 120 \
  https://github.com/Inforberi/ArcNet/releases/download/v0.7.0-stage5-a7/update-a7.zip -o "$d/update.zip"
printf '%s  %s\n' 92bb14d969c5c4eacd900086c8a63f8d8c59e828e2a0e7c06219844ee036abfb "$d/update.zip" | sha256sum -c -
mv -T "$d/update.zip" "$stage"
# Existing root-owned unit verifies the package signature before replacement.
systemctl start arcvpn-update.service
test "$($core version)" = 'arcvpn 0.7.0-stage5-a7'
runuser -u arcvpn -- "$core" frontend-check
# The old helper otherwise remains alive while clients poll its socket.
systemctl restart arcvpn-control.service
systemctl is-active arcvpn-panel.service arcvpn-control.socket x-ui.service
"$core" version
printf '%s\n' 'Open Settings → Updates. Future compatible releases can be selected there.'

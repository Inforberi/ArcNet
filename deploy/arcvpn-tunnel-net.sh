#!/usr/bin/env bash
# Narrowly scoped NAT for exactly two ArcVPN-managed tunnel interfaces.
# Called only from trusted generated wg-quick/awg-quick configs as root.
set -Eeuo pipefail
[[ $# -eq 3 ]] || { echo 'usage: arcvpn-tunnel-net up|down arcwg|arcawg subnet' >&2; exit 2; }
action="$1"; iface="$2"; subnet="$3"
case "$action:$iface:$subnet" in
  up:arcwg:10.76.0.0/16|down:arcwg:10.76.0.0/16|up:arcawg:10.77.0.0/16|down:arcawg:10.77.0.0/16) ;;
  *) echo 'refusing unmanaged interface or subnet' >&2; exit 2 ;;
esac
command -v ip >/dev/null && command -v iptables >/dev/null || { echo 'iproute2 and iptables required' >&2; exit 1; }
# Parse a locally configured default route; never treat its contents as shell code.
state_dir=/run/arcvpn
wan_file="$state_dir/wan-$iface"
if [[ "$action" == up ]]; then
  wan="$(ip -4 route show default | awk 'NR==1 {for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}')"
  [[ "$wan" =~ ^[a-zA-Z0-9_.:-]{1,15}$ ]] || { echo 'no usable IPv4 default interface' >&2; exit 1; }
  install -d -m 0700 "$state_dir"
  printf '%s\n' "$wan" > "$wan_file"
  chmod 0600 "$wan_file"
else
  [[ -r "$wan_file" ]] || exit 0
  IFS= read -r wan < "$wan_file"
  [[ "$wan" =~ ^[a-zA-Z0-9_.:-]{1,15}$ ]] || exit 1
fi
comment="ArcVPN-$iface"
# Each rule is scoped to the managed tunnel network and return traffic only.
apply_rule() {
  local table="$1" chain="$2"; shift 2
  if [[ "$action" == 'up' ]]; then
    iptables -w 5 -t "$table" -C "$chain" "$@" 2>/dev/null || iptables -w 5 -t "$table" -I "$chain" 1 "$@"
  else
    iptables -w 5 -t "$table" -C "$chain" "$@" 2>/dev/null && iptables -w 5 -t "$table" -D "$chain" "$@" || :
  fi
}
apply_rule nat POSTROUTING -s "$subnet" -o "$wan" -m comment --comment "$comment" -j MASQUERADE
apply_rule filter FORWARD -i "$iface" -o "$wan" -s "$subnet" -m comment --comment "$comment" -j ACCEPT
apply_rule filter FORWARD -i "$wan" -o "$iface" -d "$subnet" -m conntrack --ctstate RELATED,ESTABLISHED -m comment --comment "$comment" -j ACCEPT

if [[ "$action" == down ]]; then rm -f -- "$wan_file"; fi

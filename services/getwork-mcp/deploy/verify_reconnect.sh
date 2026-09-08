#!/usr/bin/env bash
set -Eeuo pipefail

[[ "${EUID}" -eq 0 ]] || { echo "verify_reconnect: must run as root" >&2; exit 1; }
mode="${1:-}"
[[ "$mode" == stop || "$mode" == start ]] ||
  { echo "usage: verify_reconnect.sh stop|start <verify_node arguments...>" >&2; exit 1; }
shift
[[ $# -ge 1 ]] || { echo "verify_reconnect: verifier arguments are required" >&2; exit 1; }
deploy_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

if [[ "$mode" == stop ]]; then
  python3 "$deploy_dir/verify_node.py" "$@" >/dev/null
  systemctl stop henukit-getwork-tunnel.service
  [[ "$(systemctl is-active henukit-getwork-tunnel.service || true)" == inactive ]]
  [[ "$(systemctl show henukit-getwork-tunnel.service --property MainPID --value)" == 0 ]]
  echo "tunnel stopped; production must now record relay 503 and local-crawler absence"
  exit 0
fi

systemctl start henukit-getwork-tunnel.service
for _ in {1..30}; do
  [[ "$(systemctl is-active henukit-getwork-tunnel.service || true)" == active ]] && break
  sleep 1
done
old_pid="$(systemctl show henukit-getwork-tunnel.service --property MainPID --value)"
[[ "$old_pid" =~ ^[1-9][0-9]*$ ]]
kill -KILL "$old_pid"
for _ in {1..30}; do
  new_pid="$(systemctl show henukit-getwork-tunnel.service --property MainPID --value)"
  if [[ "$new_pid" =~ ^[1-9][0-9]*$ && "$new_pid" != "$old_pid" ]] &&
    [[ "$(systemctl is-active henukit-getwork-tunnel.service || true)" == active ]]; then
    break
  fi
  sleep 1
done
[[ "$new_pid" != "$old_pid" ]]
python3 "$deploy_dir/verify_node.py" "$@"

#!/usr/bin/env bash
set -Eeuo pipefail

backup_dir="${1:-}"
[[ "${EUID}" -eq 0 ]] || { echo "rollback_node: must run as root" >&2; exit 1; }
self_path="$(readlink -f "${BASH_SOURCE[0]}")"
current_parent="$(dirname "$self_path")"
[[ -f "$self_path" && ! -L "$self_path" && "$(stat -c %u "$self_path")" == 0 ]] ||
  { echo "rollback_node: helper is not root-trusted" >&2; exit 1; }
(( (8#$(stat -c %a "$self_path") & 8#022) == 0 )) ||
  { echo "rollback_node: helper is writable by group/other" >&2; exit 1; }
while [[ "$current_parent" != / ]]; do
  [[ ! -L "$current_parent" && "$(stat -c %u "$current_parent")" == 0 ]] ||
    { echo "rollback_node: helper ancestry is not root-trusted" >&2; exit 1; }
  (( (8#$(stat -c %a "$current_parent") & 8#022) == 0 )) ||
    { echo "rollback_node: helper ancestry is writable" >&2; exit 1; }
  current_parent="$(dirname "$current_parent")"
done
[[ "$backup_dir" == /var/lib/henukit-getwork-backups/* && -d "$backup_dir" && ! -L "$backup_dir" ]] ||
  { echo "rollback_node: pass an exact trusted backup directory" >&2; exit 1; }
[[ "$(stat -c %u "$backup_dir")" == 0 ]] ||
  { echo "rollback_node: backup must be root-owned" >&2; exit 1; }
state_file="$backup_dir/service-state"
[[ -f "$state_file" && ! -L "$state_file" && "$(stat -c %u "$state_file")" == 0 &&
   "$(stat -c %a "$state_file")" == 400 ]] ||
  { echo "rollback_node: service-state contract is missing or untrusted" >&2; exit 1; }

state_value() {
  local key="$1"
  [[ "$(grep -Ec "^${key}=" "$state_file" || true)" -eq 1 ]]
  grep -E "^${key}=" "$state_file" | cut -d= -f2-
}

for unit in henukit-getwork-tunnel.service henukit-getwork-mcp.service; do
  if systemctl cat "$unit" >/dev/null 2>&1; then
    systemctl stop "$unit"
    systemctl reset-failed "$unit" 2>/dev/null || true
    [[ "$(systemctl is-active "$unit" 2>/dev/null || true)" == inactive ]]
    [[ "$(systemctl show "$unit" --property MainPID --value)" == 0 ]]
  fi
done
if docker inspect henukit-getwork-mcp >/dev/null 2>&1; then
  echo "rollback_node: crawler container remained after service stop" >&2
  exit 1
fi
quarantine="/var/lib/henukit-getwork-backups/pre-rollback-$(date -u +%Y%m%dT%H%M%SZ).$$"
install -d -o root -g root -m 0700 "$quarantine"
for relative in \
  etc/henukit-getwork \
  etc/systemd/system/henukit-getwork-mcp.service \
  etc/systemd/system/henukit-getwork-tunnel.service \
  usr/local/libexec/henukit-getwork-egress \
  usr/local/libexec/henukit-getwork-deploy; do
  current="/$relative"
  if [[ -e "$current" || -L "$current" ]]; then
    install -d -m 0755 "$quarantine/$(dirname "$relative")"
    mv "$current" "$quarantine/$relative"
  fi
  if [[ -e "$backup_dir/$relative" || -L "$backup_dir/$relative" ]]; then
    install -d -m 0755 "/$(dirname "$relative")"
    cp -a "$backup_dir/$relative" "/$(dirname "$relative")/"
  fi
done
systemctl daemon-reload
for unit in henukit-getwork-mcp.service henukit-getwork-tunnel.service; do
  key="${unit//[-.]/_}"
  if [[ "$(state_value "unit_${key}_enabled")" == 1 &&
        -f "/etc/systemd/system/$unit" ]]; then
    systemctl enable "$unit" >/dev/null
  else
    systemctl disable "$unit" >/dev/null
  fi
  if [[ -f "/etc/systemd/system/$unit" ]]; then
    if [[ "$(state_value "unit_${key}_active")" == 1 ]]; then
      systemctl start "$unit"
    else
      systemctl stop "$unit"
    fi
  fi
done

if [[ "$(state_value network_present)" == 0 ]] &&
   docker network inspect henukit-getwork-egress >/dev/null 2>&1; then
  iptables_bin="$(command -v iptables)"
  while "$iptables_bin" -C DOCKER-USER -s 172.30.250.0/24 -j HENUKIT-GETWORK-EGRESS 2>/dev/null; do
    "$iptables_bin" -D DOCKER-USER -s 172.30.250.0/24 -j HENUKIT-GETWORK-EGRESS
  done
  "$iptables_bin" -F HENUKIT-GETWORK-EGRESS 2>/dev/null || true
  "$iptables_bin" -X HENUKIT-GETWORK-EGRESS 2>/dev/null || true
  docker network rm henukit-getwork-egress >/dev/null
fi

if [[ "$(state_value account_present)" == 1 ]]; then
  account_groups="$(state_value account_groups)"
  usermod --home "$(state_value account_home)" --shell "$(state_value account_shell)" \
    --gid "$(state_value account_gid)" --groups "${account_groups// /,}" henukit-getwork-tunnel
  printf '%s:%s\n' henukit-getwork-tunnel "$(state_value account_password)" |
    chpasswd --encrypted
  home_metadata="$(state_value account_home_metadata)"
  if [[ -n "$home_metadata" && -d "$(state_value account_home)" ]]; then
    IFS=: read -r home_uid home_gid home_mode <<<"$home_metadata"
    chown "$home_uid:$home_gid" "$(state_value account_home)"
    chmod "$home_mode" "$(state_value account_home)"
  fi
  if [[ "$(state_value account_home)" != /var/lib/henukit-getwork-tunnel &&
        -d /var/lib/henukit-getwork-tunnel ]]; then
    mv /var/lib/henukit-getwork-tunnel "$quarantine/created-account-home"
  fi
else
  if getent passwd henukit-getwork-tunnel >/dev/null; then
    account_uid="$(id -u henukit-getwork-tunnel)"
    ! pgrep -u "$account_uid" >/dev/null
    if [[ -d /var/lib/henukit-getwork-tunnel ]]; then
      mv /var/lib/henukit-getwork-tunnel "$quarantine/created-account-home"
    fi
    userdel henukit-getwork-tunnel
  fi
  if [[ "$(state_value group_present)" == 0 ]] &&
     getent group henukit-getwork-tunnel >/dev/null; then
    groupdel henukit-getwork-tunnel
  fi
fi
echo "restored metadata-preserving backup ${backup_dir}; replaced state retained at ${quarantine}"

#!/usr/bin/env bash
set -Eeuo pipefail

program=install_production_tunnel_account
public_key_file="${1:-}"
trust_file="${2:-}"
account=henukit-getwork-tunnel
home="/var/lib/$account"
dropin=/etc/ssh/sshd_config.d/60-henukit-getwork-tunnel.conf
backup_dir=""
authorized_tmp=""
authorized_candidate=""
dropin_tmp=""
committed=0
configuration_changed=0
account_existed=0
account_state_captured=0
group_existed=0
home_existed=0
ssh_dir_existed=0
home_metadata=""
ssh_dir_metadata=""
previous_home=""
previous_shell=""
previous_gid=""
previous_password=""
previous_groups=""

die() {
  echo "${program}: $*" >&2
  exit 1
}

trusted_root_chain() {
  local current="$1"
  [[ "$current" == /* ]] || return 1
  while [[ "$current" != / ]]; do
    [[ ! -L "$current" && "$(stat -c %u "$current")" == 0 ]] || return 1
    (( (8#$(stat -c %a "$current") & 8#022) == 0 )) || return 1
    current="$(dirname "$current")"
  done
}

cleanup() {
  local status=$?
  [[ -z "$authorized_tmp" ]] || rm -f "$authorized_tmp"
  [[ -z "$authorized_candidate" ]] || rm -f "$authorized_candidate"
  [[ -z "$dropin_tmp" ]] || rm -f "$dropin_tmp"
  if [[ "$status" -ne 0 && "$committed" -eq 0 && -n "$backup_dir" ]]; then
    rejected="$backup_dir/rejected"
    install -d -o root -g root -m 0700 "$rejected"
    for current in "$home/.ssh/authorized_keys" "$dropin"; do
      relative="${current#/}"
      if [[ -e "$current" || -L "$current" ]]; then
        install -d -m 0700 "$rejected/$(dirname "$relative")"
        mv "$current" "$rejected/$relative"
      fi
      if [[ -e "$backup_dir/$relative" || -L "$backup_dir/$relative" ]]; then
        install -d -m 0755 "/$(dirname "$relative")"
        cp -a "$backup_dir/$relative" "/$(dirname "$relative")/"
      fi
    done
    if [[ -e "$backup_dir/retired-authorized_keys2" ]]; then
      mv "$backup_dir/retired-authorized_keys2" "$home/.ssh/authorized_keys2"
    fi
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 &&
        "$account_state_captured" -eq 1 && "$account_existed" -eq 1 ]]; then
    previous_group_csv="${previous_groups// /,}"
    usermod --home "$previous_home" --shell "$previous_shell" \
      --gid "$previous_gid" --groups "$previous_group_csv" "$account" || true
    printf '%s:%s\n' "$account" "$previous_password" | chpasswd --encrypted || true
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 &&
        "$account_state_captured" -eq 1 && "$account_existed" -eq 0 ]]; then
    if getent passwd "$account" >/dev/null; then
      userdel "$account" || echo "${program}: failed to remove newly created account" >&2
    fi
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 &&
        "$account_state_captured" -eq 1 && "$group_existed" -eq 0 ]] &&
     getent group "$account" >/dev/null; then
    groupdel "$account" || echo "${program}: failed to remove newly created group" >&2
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 && "$account_state_captured" -eq 1 ]]; then
    if [[ "$home_existed" -eq 0 && ( -e "$home" || -L "$home" ) ]]; then
      rejected="${backup_dir:-/var/lib/henukit-getwork-production-backups}/rejected"
      install -d -o root -g root -m 0700 "$rejected"
      mv "$home" "$rejected/candidate-account-home" ||
        echo "${program}: failed to quarantine candidate account home" >&2
    elif [[ "$home_existed" -eq 1 ]]; then
      if [[ "$ssh_dir_existed" -eq 0 ]]; then
        rmdir "$home/.ssh" 2>/dev/null || true
      elif [[ -n "$ssh_dir_metadata" && -d "$home/.ssh" ]]; then
        IFS=: read -r ssh_mode ssh_uid ssh_gid <<<"$ssh_dir_metadata"
        chown "$ssh_uid:$ssh_gid" "$home/.ssh" || true
        chmod "$ssh_mode" "$home/.ssh" || true
      fi
      if [[ -n "$home_metadata" && -d "$home" ]]; then
        IFS=: read -r home_mode home_uid home_gid <<<"$home_metadata"
        chown "$home_uid:$home_gid" "$home" || true
        chmod "$home_mode" "$home" || true
      fi
    fi
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 && "$configuration_changed" -eq 1 ]]; then
    if sshd -t; then
      systemctl reload sshd 2>/dev/null || systemctl reload ssh ||
        echo "${program}: failed to reload restored SSH configuration" >&2
    else
      echo "${program}: restored SSH configuration did not validate" >&2
    fi
  fi
  exit "$status"
}
trap cleanup EXIT

[[ "${EUID}" -eq 0 ]] || die "must run as root"
for command in pgrep pkill; do
  command -v "$command" >/dev/null || die "required command is missing: $command"
done
self_path="$(readlink -f "${BASH_SOURCE[0]}")"
[[ -f "$self_path" && ! -L "$self_path" && "$(stat -c %u "$self_path")" == 0 ]] ||
  die "installer must execute from a root-owned signature-verified runtime"
(( (8#$(stat -c %a "$self_path") & 8#022) == 0 )) ||
  die "installer must not be group/world writable"
trusted_root_chain "$(dirname "$self_path")" ||
  die "installer ancestry is not root-trusted"
[[ "$public_key_file" == /* && -f "$public_key_file" && ! -L "$public_key_file" &&
   "$(stat -c %u "$public_key_file")" == 0 ]] ||
  die "usage: $program <dedicated-ed25519-public-key-file> <approved-trust.env>"
(( (8#$(stat -c %a "$public_key_file") & 8#022) == 0 )) ||
  die "public key file must not be group/world writable"
trusted_root_chain "$(dirname "$public_key_file")" ||
  die "public key file ancestry is not root-trusted"
[[ "$trust_file" == /* && -f "$trust_file" && ! -L "$trust_file" &&
   "$(stat -c %u "$trust_file")" == 0 ]] ||
  die "usage: $program <dedicated-ed25519-public-key-file> <approved-trust.env>"
(( (8#$(stat -c %a "$trust_file") & 8#022) == 0 )) ||
  die "trust file must not be group/world writable"
trusted_root_chain "$(dirname "$trust_file")" ||
  die "trust file ancestry is not root-trusted"
key_type="$(awk 'NR == 1 {print $1}' "$public_key_file")"
key_body="$(awk 'NR == 1 {print $2}' "$public_key_file")"
[[ "$key_type" == ssh-ed25519 && -n "$key_body" ]] || die "public key must be Ed25519"
ssh-keygen -l -f "$public_key_file" >/dev/null || die "public key is invalid"
key_fingerprint="$(ssh-keygen -lf "$public_key_file" -E sha256 | awk '{print $2}')"
expected_fingerprint="$(awk -F= '$1 == "HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT" { if (seen++) exit 2; print $2 }' "$trust_file")"
[[ -n "$expected_fingerprint" && "$key_fingerprint" == "$expected_fingerprint" ]] ||
  die "dedicated public-key fingerprint is not approved"

if getent passwd "$account" >/dev/null; then
  account_existed=1
  IFS=: read -r _ _ _ previous_gid _ previous_home previous_shell \
    <<<"$(getent passwd "$account")"
  previous_password="$(getent shadow "$account" | cut -d: -f2)"
  previous_groups="$(id -Gn "$account" | awk '{$1=""; sub(/^ /, ""); print}')"
fi
if getent group "$account" >/dev/null; then
  group_existed=1
fi
[[ ! -L "$home" ]] || die "account home must not be a symbolic link"
if [[ -d "$home" ]]; then
  home_existed=1
  home_metadata="$(stat -c '%a:%u:%g' "$home")"
fi
if [[ -d "$home/.ssh" && ! -L "$home/.ssh" ]]; then
  ssh_dir_existed=1
  ssh_dir_metadata="$(stat -c '%a:%u:%g' "$home/.ssh")"
elif [[ -e "$home/.ssh" || -L "$home/.ssh" ]]; then
  die "account SSH directory must be a real directory"
fi
account_state_captured=1

backup_root=/var/lib/henukit-getwork-production-backups
install -d -o root -g root -m 0700 "$backup_root"
backup_dir="$(mktemp -d "$backup_root/$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")"
chmod 0700 "$backup_dir"
for current in "$home/.ssh/authorized_keys" "$home/.ssh/authorized_keys2" "$dropin" \
  "$previous_home/.ssh/authorized_keys" "$previous_home/.ssh/authorized_keys2"; do
  [[ "$current" == "/.ssh/authorized_keys" || "$current" == "/.ssh/authorized_keys2" ]] && continue
  if [[ -e "$current" || -L "$current" ]]; then
    cp -a --parents "$current" "$backup_dir"
  fi
done

if [[ "$group_existed" -eq 0 ]]; then
  groupadd --system "$account"
fi
if [[ "$account_existed" -eq 0 ]]; then
  useradd --system --home-dir "/var/lib/$account" --create-home \
    --gid "$account" --shell /usr/sbin/nologin "$account"
fi
prior_effective="$(sshd -T -C user=$account,host=localhost,addr=127.0.0.1)" ||
  die "could not resolve the prior effective SSH policy"
prior_login_grace="$(awk '$1 == "logingracetime" { if (seen++) exit 2; print $2 }' \
  <<<"$prior_effective")"
[[ "$prior_login_grace" =~ ^[0-9]+$ && "$prior_login_grace" -ge 1 &&
   "$prior_login_grace" -le 300 ]] ||
  die "prior LoginGraceTime must be between 1 and 300 seconds"
install -d -o root -g "$account" -m 0750 "$home"
install -d -o root -g "$account" -m 0750 "$home/.ssh"
configuration_changed=1
# If a pre-existing account used another home, keep its current key available at
# the absolute Match path before installing the drop-in. A hard interruption or
# reboot then retains a usable old key under the restrictive policy; the new key
# is promoted atomically only after reload.
if [[ "$account_existed" -eq 1 && "$previous_home" != "$home" ]]; then
  if [[ -e "$home/.ssh/authorized_keys" || -L "$home/.ssh/authorized_keys" ]]; then
    mv "$home/.ssh/authorized_keys" "$backup_dir/retired-future-authorized_keys"
  fi
  if [[ -f "$previous_home/.ssh/authorized_keys" &&
        ! -L "$previous_home/.ssh/authorized_keys" ]]; then
    install -o root -g "$account" -m 0640 \
      "$previous_home/.ssh/authorized_keys" "$home/.ssh/authorized_keys"
  fi
fi
if [[ -e "$home/.ssh/authorized_keys2" || -L "$home/.ssh/authorized_keys2" ]]; then
  mv "$home/.ssh/authorized_keys2" "$backup_dir/retired-authorized_keys2"
fi

authorized_tmp="$(mktemp)"
dropin_tmp="$(mktemp)"
printf '%s\n' \
  "restrict,port-forwarding,permitopen=\"127.0.0.1:18100\",permitlisten=\"127.0.0.1:18100\",command=\"/bin/false\" $key_type $key_body henukit-getwork-tunnel" \
  > "$authorized_tmp"
authorized_candidate="$(mktemp "$home/.ssh/.authorized_keys.XXXXXX")"
install -o root -g "$account" -m 0640 "$authorized_tmp" "$authorized_candidate"

cat > "$dropin_tmp" <<'EOF'
Match User henukit-getwork-tunnel
    AuthorizedKeysFile /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys
    AuthenticationMethods publickey
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    PermitTTY no
    X11Forwarding no
    AllowAgentForwarding no
    AllowTcpForwarding remote
    AllowStreamLocalForwarding no
    GatewayPorts no
    PermitOpen none
    PermitListen 127.0.0.1:18100
    ForceCommand /bin/false
EOF
install -o root -g root -m 0644 "$dropin_tmp" "$dropin"
sshd -t || die "sshd configuration validation failed; restore ${backup_dir}"
effective="$(sshd -T -C user=$account,host=localhost,addr=127.0.0.1)"
grep -Fqx 'authenticationmethods publickey' <<<"$effective"
grep -Fqx 'authorizedkeysfile /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys' <<<"$effective"
grep -Fqx 'passwordauthentication no' <<<"$effective"
grep -Fqx 'kbdinteractiveauthentication no' <<<"$effective"
grep -Fqx 'permittty no' <<<"$effective"
grep -Fqx 'x11forwarding no' <<<"$effective"
grep -Fqx 'allowagentforwarding no' <<<"$effective"
grep -Fqx 'allowtcpforwarding remote' <<<"$effective"
grep -Fqx 'allowstreamlocalforwarding no' <<<"$effective"
grep -Fqx 'gatewayports no' <<<"$effective"
grep -Fqx 'permitopen none' <<<"$effective"
grep -Fqx 'permitlisten 127.0.0.1:18100' <<<"$effective"
grep -Fqx 'forcecommand /bin/false' <<<"$effective"
systemctl reload sshd 2>/dev/null || systemctl reload ssh
sleep "$((prior_login_grace + 1))"
pkill -KILL -u "$account" 2>/dev/null || true
sleep 1
if pgrep -u "$account" >/dev/null; then
  die "dedicated account still has an authenticated process after session drain"
fi
mv -f -- "$authorized_candidate" "$home/.ssh/authorized_keys"
authorized_candidate=""
usermod --home "/var/lib/$account" --shell /usr/sbin/nologin --gid "$account" \
  --groups '' "$account"
# A leading "!" marks a Linux account locked and OpenSSH rejects it before
# public-key authentication. "NP" is the OpenSSH-documented non-password value;
# the Match block also disables both password authentication methods.
usermod --password NP "$account"
[[ "$(getent shadow "$account" | cut -d: -f2)" == NP ]] ||
  die "account password field is not the reviewed non-password value"
[[ "$(getent passwd "$account" | cut -d: -f6)" == "$home" ]]
[[ "$(getent passwd "$account" | cut -d: -f7)" == /usr/sbin/nologin ]]
[[ "$(getent passwd "$account" | cut -d: -f4)" == "$(getent group "$account" | cut -d: -f3)" ]]
[[ "$(id -G "$account")" == "$(getent group "$account" | cut -d: -f3)" ]]
[[ -z "$(getent group "$account" | cut -d: -f4)" ]]
[[ "$(wc -l < "$home/.ssh/authorized_keys" | tr -d '[:space:]')" -eq 1 ]]
[[ "$(cat "$home/.ssh/authorized_keys")" == "restrict,port-forwarding,permitopen=\"127.0.0.1:18100\",permitlisten=\"127.0.0.1:18100\",command=\"/bin/false\" $key_type $key_body henukit-getwork-tunnel" ]]
committed=1
echo "installed remote-forward-only account; backup: ${backup_dir}"

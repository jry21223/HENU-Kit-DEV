#!/usr/bin/env bash
set -Eeuo pipefail

program=install_node
release_sha=""
stage_dir=""
allowed_signers=""
trust_file=""
backup_dir=""
trusted_work=""
node_env_tmp=""
rollback_helper=""
committed=0
account_was_present=0
group_was_present=0
previous_account_gid=""
previous_account_home=""
previous_account_shell=""
previous_account_password=""
previous_account_groups=""
previous_home_metadata=""
account_state_captured=0

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

trusted_root_file() {
  local file="$1"
  [[ -f "$file" && ! -L "$file" && "$(stat -c %u "$file")" == 0 ]] || return 1
  (( (8#$(stat -c %a "$file") & 8#022) == 0 )) || return 1
  trusted_root_chain "$(dirname "$file")"
}

cleanup() {
  local status=$?
  [[ -z "$node_env_tmp" ]] || rm -f -- "$node_env_tmp"
  if [[ "$status" -ne 0 && "$committed" -eq 0 && -n "$backup_dir" && -x "$rollback_helper" ]]; then
    "$rollback_helper" "$backup_dir" >&2 || true
  fi
  if [[ "$status" -ne 0 && "$committed" -eq 0 && -z "$backup_dir" &&
        "$account_state_captured" -eq 1 ]]; then
    if [[ "$account_was_present" -eq 1 ]]; then
      previous_group_csv="${previous_account_groups// /,}"
      usermod --home "$previous_account_home" --shell "$previous_account_shell" \
        --gid "$previous_account_gid" --groups "$previous_group_csv" henukit-getwork-tunnel || true
      printf '%s:%s\n' henukit-getwork-tunnel "$previous_account_password" |
        chpasswd --encrypted || true
      if [[ -n "$previous_home_metadata" && -d "$previous_account_home" ]]; then
        IFS=: read -r previous_uid previous_gid previous_mode <<<"$previous_home_metadata"
        chown "$previous_uid:$previous_gid" "$previous_account_home" || true
        chmod "$previous_mode" "$previous_account_home" || true
      fi
    else
      if getent passwd henukit-getwork-tunnel >/dev/null; then
        account_uid="$(id -u henukit-getwork-tunnel)"
        if ! pgrep -u "$account_uid" >/dev/null; then
          userdel henukit-getwork-tunnel || true
        fi
      fi
      if [[ "$group_was_present" -eq 0 ]] && getent group henukit-getwork-tunnel >/dev/null; then
        groupdel henukit-getwork-tunnel || true
      fi
    fi
  fi
  if [[ -n "$trusted_work" && "$trusted_work" == /var/lib/henukit-getwork-install/* ]]; then
    rm -rf -- "$trusted_work"
  fi
  exit "$status"
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha) release_sha="${2:-}"; shift 2 ;;
    --stage-dir) stage_dir="${2:-}"; shift 2 ;;
    --allowed-signers) allowed_signers="${2:-}"; shift 2 ;;
    --trust-file) trust_file="${2:-}"; shift 2 ;;
    *)
      die "usage: $program --sha <40-hex-main-sha> --stage-dir <root-stage> --allowed-signers <root-trust> --trust-file <approved-fingerprints>"
      ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || die "must run as root"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid release SHA"
self_path="$(readlink -f "${BASH_SOURCE[0]}")"
trusted_root_file "$self_path" ||
  die "installer must execute from a root-owned signature-verified runtime"
for command in docker iptables jq ssh-keygen systemctl tar; do
  command -v "$command" >/dev/null || die "required command is missing: $command"
done
[[ "$stage_dir" == /* && -d "$stage_dir" && ! -L "$stage_dir" ]] ||
  die "invalid stage directory"
trusted_root_chain "$stage_dir" || die "stage directory ancestry is not root-trusted"
trusted_root_file "$allowed_signers" || die "allowed-signers file is not root-trusted"
trusted_root_file "$trust_file" || die "fingerprint trust file is not root-trusted"

image_archive="henukit-getwork-mcp-${release_sha}.docker.tar.gz"
runtime_archive="henukit-runtime-${release_sha}.tar.gz"
manifest="henukit-release-${release_sha}.manifest"
signature="${manifest}.sig"
input_names=(
  "$image_archive" "${image_archive}.sha256"
  "$runtime_archive" "${runtime_archive}.sha256"
  "$manifest" "$signature"
  node.env mcp.env id_ed25519 known_hosts
)
for name in "${input_names[@]}"; do
  trusted_root_file "$stage_dir/$name" || die "staged input is not root-trusted: $name"
done

install -d -o root -g root -m 0700 /var/lib/henukit-getwork-install
trusted_work="$(mktemp -d /var/lib/henukit-getwork-install/${release_sha}.XXXXXX)"
chmod 0700 "$trusted_work"
install -d -o root -g root -m 0700 "$trusted_work/input" "$trusted_work/runtime"
for name in "${input_names[@]}"; do
  install -o root -g root -m 0400 "$stage_dir/$name" "$trusted_work/input/$name"
done
stage_dir="$trusted_work/input"

signer="${HENUKIT_RELEASE_SIGNER:-henukit-release}"
namespace="${HENUKIT_RELEASE_SIGNATURE_NAMESPACE:-henukit-release}"
[[ "$signer" =~ ^[A-Za-z0-9_.@-]+$ && "$namespace" =~ ^[A-Za-z0-9_.@-]+$ ]] ||
  die "release signature identity is invalid"
ssh-keygen -Y verify -f "$allowed_signers" -I "$signer" -n "$namespace" \
  -s "$stage_dir/$signature" < "$stage_dir/$manifest" >/dev/null ||
  die "signed release manifest verification failed"
[[ "$(grep -Fxc 'format=henukit-local-release-v1' "$stage_dir/$manifest")" -eq 1 ]]
[[ "$(grep -Fxc "release_sha=${release_sha}" "$stage_dir/$manifest")" -eq 1 ]]
[[ "$(grep -Fxc 'source_ref=refs/heads/main' "$stage_dir/$manifest")" -eq 1 ]]
[[ "$(grep -Fxc 'builder_platform=linux/amd64' "$stage_dir/$manifest")" -eq 1 ]]
[[ "$(grep -Fxc "signer=${signer}" "$stage_dir/$manifest")" -eq 1 ]]
[[ "$(grep -Fxc "signature_namespace=${namespace}" "$stage_dir/$manifest")" -eq 1 ]]

verify_signed_artifact() {
  local name="$1"
  local digest recorded_digest recorded_name
  digest="$(sha256sum "$stage_dir/$name" | awk '{print $1}')"
  [[ "$(wc -l < "$stage_dir/${name}.sha256" | tr -d '[:space:]')" == 1 ]] ||
    die "checksum file must contain exactly one record: $name"
  read -r recorded_digest recorded_name < "$stage_dir/${name}.sha256"
  recorded_name="${recorded_name#\*}"
  [[ "$recorded_digest" == "$digest" && "$recorded_name" == "$name" ]] ||
    die "checksum does not name the exact artifact: $name"
  [[ "$(grep -Fxc "artifact_sha256=${digest}  ${name}" "$stage_dir/$manifest")" -eq 1 ]] ||
    die "artifact is absent from the signed release manifest: $name"
  printf '%s' "$digest"
}
archive_sha="$(verify_signed_artifact "$image_archive")"
runtime_sha="$(verify_signed_artifact "$runtime_archive")"
[[ -n "$runtime_sha" ]]

if tar -tzf "$stage_dir/$runtime_archive" |
  awk '/^\// || /(^|\/)\.\.($|\/)/ { exit 1 }'; then
  :
else
  die "runtime archive contains an unsafe path"
fi
tar --no-same-owner -xzf "$stage_dir/$runtime_archive" -C "$trusted_work/runtime"
[[ "$(tr -d '[:space:]' < "$trusted_work/runtime/RELEASE_SHA")" == "$release_sha" ]] ||
  die "runtime archive release marker does not match"
asset_dir="$trusted_work/runtime/getwork-node-deploy"
for asset in install_node.sh rollback_node.sh verify_node.py verify_reconnect.sh \
  henukit-getwork-egress systemd/henukit-getwork-mcp.service \
  systemd/henukit-getwork-tunnel.service; do
  trusted_root_file "$asset_dir/$asset" || die "runtime deployment asset is untrusted: $asset"
done
rollback_helper="$asset_dir/rollback_node.sh"

read_exact_env() {
  local file="$1"
  local key="$2"
  local count
  count="$(grep -Ec "^${key}=" "$file" || true)"
  [[ "$count" -eq 1 ]] || die "$key must occur exactly once in $(basename "$file")"
  grep -E "^${key}=" "$file" | cut -d= -f2-
}
memory_limit="$(read_exact_env "$stage_dir/node.env" HENUKIT_GETWORK_MEMORY_LIMIT)"
tunnel_target="$(read_exact_env "$stage_dir/node.env" HENUKIT_GETWORK_TUNNEL_TARGET)"
tunnel_port="$(read_exact_env "$stage_dir/node.env" HENUKIT_GETWORK_TUNNEL_PORT)"
[[ "$(grep -Evc '^(HENUKIT_GETWORK_MEMORY_LIMIT|HENUKIT_GETWORK_TUNNEL_TARGET|HENUKIT_GETWORK_TUNNEL_PORT)=' "$stage_dir/node.env")" -eq 0 ]] ||
  die "node.env contains unreviewed keys"
[[ "$memory_limit" == 4g ]] || die "memory limit must be 4g"
[[ "$tunnel_target" =~ ^henukit-getwork-tunnel@[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$ ]] ||
  die "tunnel target is invalid"
[[ "$tunnel_port" =~ ^[0-9]+$ && "$tunnel_port" -ge 1 && "$tunnel_port" -le 65535 ]] ||
  die "tunnel port is invalid"
token="$(read_exact_env "$stage_dir/mcp.env" GETWORK_MCP_ACCESS_TOKEN)"
[[ "$(grep -Evc '^GETWORK_MCP_ACCESS_TOKEN=' "$stage_dir/mcp.env")" -eq 0 ]] ||
  die "mcp.env contains unreviewed keys"
normalized_token="$(printf '%s' "$token" | tr '[:upper:]' '[:lower:]')"
[[ ${#token} -ge 32 && "$token" != *[[:space:]]* &&
   ! "$normalized_token" =~ (replace|example|change-me|test-only) ]] ||
  die "MCP bearer is missing or a placeholder"
unset token normalized_token

approved_key_fingerprint="$(read_exact_env "$trust_file" HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT)"
approved_host_fingerprint="$(read_exact_env "$trust_file" HENUKIT_GETWORK_HOST_KEY_FINGERPRINT)"
[[ "$(grep -Evc '^(HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT|HENUKIT_GETWORK_HOST_KEY_FINGERPRINT)=' "$trust_file")" -eq 0 ]] ||
  die "fingerprint trust file contains unreviewed keys"
ssh-keygen -y -f "$stage_dir/id_ed25519" > "$trusted_work/tunnel.pub"
private_fingerprint="$(ssh-keygen -lf "$trusted_work/tunnel.pub" -E sha256 | awk '{print $2}')"
[[ "$private_fingerprint" == "$approved_key_fingerprint" ]] ||
  die "tunnel private key fingerprint is not approved"
tunnel_host="${tunnel_target#*@}"
ssh-keygen -F "[${tunnel_host}]:${tunnel_port}" -f "$stage_dir/known_hosts" >/dev/null ||
  die "known_hosts does not contain the approved tunnel target"
host_fingerprints="$(ssh-keygen -lf "$stage_dir/known_hosts" -E sha256 | awk '{print $2}' | sort -u)"
[[ "$(wc -l <<<"$host_fingerprints" | tr -d '[:space:]')" -eq 1 &&
   "$host_fingerprints" == "$approved_host_fingerprint" ]] ||
  die "production host key fingerprint is not approved"

archive_config="$(tar -xOzf "$stage_dir/$image_archive" manifest.json |
  jq -er 'if length == 1 and (.[0].Config | test("^[0-9a-f]{64}\\.json$")) then .[0].Config else error("invalid image manifest") end')"
archive_image_id="sha256:${archive_config%.json}"
docker load --input "$stage_dir/$image_archive" >/dev/null
image="henukit-getwork-mcp:${release_sha}"
[[ "$(docker image inspect "$image" --format '{{.Os}}/{{.Architecture}}')" == linux/amd64 ]] ||
  die "loaded image is not linux/amd64"
image_id="$(docker image inspect "$image" --format '{{.Id}}')"
[[ "$image_id" == "$archive_image_id" ]] ||
  die "loaded image ID does not match the signed image archive"

if getent passwd henukit-getwork-tunnel >/dev/null; then
  account_was_present=1
  IFS=: read -r _ _ _ previous_account_gid _ previous_account_home previous_account_shell \
    <<<"$(getent passwd henukit-getwork-tunnel)"
  previous_account_password="$(getent shadow henukit-getwork-tunnel | cut -d: -f2)"
  previous_account_groups="$(id -Gn henukit-getwork-tunnel | awk '{$1=""; sub(/^ /, ""); print}')"
  if [[ -d "$previous_account_home" && ! -L "$previous_account_home" ]]; then
    previous_home_metadata="$(stat -c '%u:%g:%a' "$previous_account_home")"
  fi
fi
if getent group henukit-getwork-tunnel >/dev/null; then
  group_was_present=1
else
  groupadd --system henukit-getwork-tunnel
fi
account_state_captured=1
if ! getent passwd henukit-getwork-tunnel >/dev/null; then
  useradd --system --gid henukit-getwork-tunnel \
    --home-dir /var/lib/henukit-getwork-tunnel --create-home \
    --shell /usr/sbin/nologin henukit-getwork-tunnel
fi
usermod --home /var/lib/henukit-getwork-tunnel --shell /usr/sbin/nologin \
  --gid henukit-getwork-tunnel --groups '' --password NP henukit-getwork-tunnel
install -d -o root -g root -m 0750 /var/lib/henukit-getwork-tunnel
IFS=: read -r _ _ account_uid account_gid _ account_home account_shell \
  <<<"$(getent passwd henukit-getwork-tunnel)"
expected_gid="$(getent group henukit-getwork-tunnel | cut -d: -f3)"
[[ "$account_uid" -lt 1000 && "$account_gid" -lt 1000 &&
   "$account_gid" == "$expected_gid" &&
   "$account_home" == /var/lib/henukit-getwork-tunnel &&
   "$account_shell" == /usr/sbin/nologin &&
   "$(getent shadow henukit-getwork-tunnel | cut -d: -f2)" == NP &&
   "$(id -G henukit-getwork-tunnel)" == "$account_gid" &&
   -z "$(getent group henukit-getwork-tunnel | cut -d: -f4)" ]] ||
  die "WSL tunnel account does not match the reviewed no-login contract"

backup_root=/var/lib/henukit-getwork-backups
install -d -o root -g root -m 0700 "$backup_root"
backup_dir="$(mktemp -d "$backup_root/$(date -u +%Y%m%dT%H%M%SZ)-${release_sha}.XXXXXX")"
chmod 0700 "$backup_dir"
for current in /etc/henukit-getwork \
  /etc/systemd/system/henukit-getwork-mcp.service \
  /etc/systemd/system/henukit-getwork-tunnel.service \
  /usr/local/libexec/henukit-getwork-egress \
  /usr/local/libexec/henukit-getwork-deploy; do
  if [[ -e "$current" || -L "$current" ]]; then
    cp -a --parents "$current" "$backup_dir"
  else
    printf '%s\n' "$current" >> "$backup_dir/absent"
  fi
done
chmod 0600 "$backup_dir/absent" 2>/dev/null || true
{
  for unit in henukit-getwork-mcp.service henukit-getwork-tunnel.service; do
    if systemctl is-enabled "$unit" >/dev/null 2>&1; then
      printf 'unit_%s_enabled=1\n' "${unit//[-.]/_}"
    else
      printf 'unit_%s_enabled=0\n' "${unit//[-.]/_}"
    fi
    if [[ "$(systemctl is-active "$unit" 2>/dev/null || true)" == active ]]; then
      printf 'unit_%s_active=1\n' "${unit//[-.]/_}"
    else
      printf 'unit_%s_active=0\n' "${unit//[-.]/_}"
    fi
  done
  if docker network inspect henukit-getwork-egress >/dev/null 2>&1; then
    printf 'network_present=1\n'
  else
    printf 'network_present=0\n'
  fi
  printf 'account_present=%s\n' "$account_was_present"
  printf 'group_present=%s\n' "$group_was_present"
  printf 'account_gid=%s\n' "$previous_account_gid"
  printf 'account_home=%s\n' "$previous_account_home"
  printf 'account_shell=%s\n' "$previous_account_shell"
  printf 'account_password=%s\n' "$previous_account_password"
  printf 'account_groups=%s\n' "$previous_account_groups"
  printf 'account_home_metadata=%s\n' "$previous_home_metadata"
} > "$backup_dir/service-state"
chmod 0400 "$backup_dir/service-state"
printf '%s\n' "$backup_dir" > "$backup_root/latest"
chmod 0600 "$backup_root/latest"

install -d -o root -g henukit-getwork-tunnel -m 0750 \
  /etc/henukit-getwork /etc/henukit-getwork/tunnel
install -d -o root -g root -m 0700 /var/lib/henukit-getwork-artifacts
install -o root -g root -m 0644 "$allowed_signers" /etc/henukit-getwork/release-signers
install -o root -g root -m 0600 "$trust_file" /etc/henukit-getwork/trust.env
install -o root -g root -m 0600 "$stage_dir/mcp.env" /etc/henukit-getwork/mcp.env
install -o root -g root -m 0600 "$stage_dir/id_ed25519" /etc/henukit-getwork/tunnel/id_ed25519
install -o root -g henukit-getwork-tunnel -m 0640 \
  "$stage_dir/known_hosts" /etc/henukit-getwork/tunnel/known_hosts
for artifact in "$image_archive" "$manifest" "$signature"; do
  install -o root -g root -m 0400 "$stage_dir/$artifact" \
    "/var/lib/henukit-getwork-artifacts/$artifact"
done

deploy_candidate="$(mktemp -d /usr/local/libexec/.henukit-getwork-deploy.XXXXXX)"
cp -a "$asset_dir/." "$deploy_candidate/"
chown -R root:root "$deploy_candidate"
if [[ -e /usr/local/libexec/henukit-getwork-deploy ]]; then
  mv /usr/local/libexec/henukit-getwork-deploy "$backup_dir/replaced-deploy-bundle"
fi
mv "$deploy_candidate" /usr/local/libexec/henukit-getwork-deploy
installed_deploy=/usr/local/libexec/henukit-getwork-deploy
rollback_helper="$installed_deploy/rollback_node.sh"

node_env_tmp="$(mktemp)"
{
  printf 'HENUKIT_GETWORK_RELEASE_SHA=%s\n' "$release_sha"
  printf 'HENUKIT_GETWORK_IMAGE_ID=%s\n' "$image_id"
  printf 'HENUKIT_GETWORK_ARCHIVE_SHA256=%s\n' "$archive_sha"
  printf 'HENUKIT_GETWORK_MCP_UNIT_SHA256=%s\n' \
    "$(sha256sum "$installed_deploy/systemd/henukit-getwork-mcp.service" | awk '{print $1}')"
  printf 'HENUKIT_GETWORK_TUNNEL_UNIT_SHA256=%s\n' \
    "$(sha256sum "$installed_deploy/systemd/henukit-getwork-tunnel.service" | awk '{print $1}')"
  printf 'HENUKIT_GETWORK_EGRESS_SHA256=%s\n' \
    "$(sha256sum "$installed_deploy/henukit-getwork-egress" | awk '{print $1}')"
  printf 'HENUKIT_GETWORK_MEMORY_LIMIT=%s\n' "$memory_limit"
  printf 'HENUKIT_GETWORK_TUNNEL_TARGET=%s\n' "$tunnel_target"
  printf 'HENUKIT_GETWORK_TUNNEL_PORT=%s\n' "$tunnel_port"
} > "$node_env_tmp"
install -o root -g root -m 0644 "$node_env_tmp" /etc/henukit-getwork/node.env
install -o root -g root -m 0755 "$installed_deploy/henukit-getwork-egress" \
  /usr/local/libexec/henukit-getwork-egress
install -o root -g root -m 0644 "$installed_deploy/systemd/henukit-getwork-mcp.service" \
  /etc/systemd/system/henukit-getwork-mcp.service
install -o root -g root -m 0644 "$installed_deploy/systemd/henukit-getwork-tunnel.service" \
  /etc/systemd/system/henukit-getwork-tunnel.service

systemctl daemon-reload
systemctl enable henukit-getwork-mcp.service henukit-getwork-tunnel.service >/dev/null
systemctl restart henukit-getwork-mcp.service
systemctl restart henukit-getwork-tunnel.service
committed=1
echo "installed ${image} from signed manifest; rollback backup: ${backup_dir}"

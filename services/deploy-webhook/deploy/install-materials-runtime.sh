#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

program="install-materials-runtime"

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
usage: install-materials-runtime.sh --release-sha <full-git-sha>

Installs only the prebuilt, signed materials runtime carried beside this
script. It removes the retired converter configuration recoverably and keeps
the materials runner disabled until the exact-SHA webhook path is approved.
EOF
}

release_sha=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --release-sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$EUID" -eq 0 ]] || die "must run as root"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--release-sha must be a full lowercase Git SHA"
[[ "$(uname -s)" == "Linux" && "$(uname -m)" == "x86_64" ]] ||
  die "materials runtime requires Linux/amd64"

payload_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
release_marker="$payload_dir/../RELEASE_SHA"
[[ -f "$release_marker" && ! -L "$release_marker" ]] || die "signed runtime RELEASE_SHA is missing"
[[ "$(tr -d '[:space:]' < "$release_marker")" == "$release_sha" ]] ||
  die "payload release SHA does not match --release-sha"

for command in awk cp find grep install mktemp mv sha256sum sort stat systemctl; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

checksum_manifest="$payload_dir/SHA256SUMS"
[[ -f "$checksum_manifest" && ! -L "$checksum_manifest" ]] || die "materials SHA256SUMS is missing"
if find "$payload_dir" -type l -print -quit | grep -q .; then
  die "materials payload contains a symlink"
fi
if find "$payload_dir" ! -type d ! -type f -print -quit | grep -q .; then
  die "materials payload contains an unsupported file type"
fi
actual_files="$(cd "$payload_dir" && find . -type f ! -name SHA256SUMS -printf '%P\n' | LC_ALL=C sort)"
manifest_files="$(awk 'NF == 2 && $1 ~ /^[0-9a-f]{64}$/ { print $2 }' "$checksum_manifest" | LC_ALL=C sort)"
[[ -n "$actual_files" && "$actual_files" == "$manifest_files" ]] ||
  die "materials payload does not exactly match SHA256SUMS"
(cd "$payload_dir" && sha256sum -c SHA256SUMS) >/dev/null || die "materials payload checksum verification failed"

required_files=(
  bin/henukit-deploy-webhook
  bin/materials-oss-canary
  bin/materials-oss-release
  bin/library-activate-public-release
  libexec/henukit-materials-orchestrate
  libexec/henukit-materials-prepare
  libexec/henukit-materials-seal
  libexec/henukit-materials-activate
  libexec/henukit-materials-publish-oss
  libexec/henukit-materials-publish-release-oss
  libexec/prepare-henukit-materials.mjs
  libexec/seal-henukit-materials.mjs
  libexec/activate-henukit-materials.mjs
  libexec/build-henukit-library-activation-bundle.mjs
  systemd/henukit-materials-webhook.service
  systemd/henukit-materials-webhook.path
  systemd/henukit-materials-runner.service
)
for relative_path in "${required_files[@]}"; do
  [[ -f "$payload_dir/$relative_path" && ! -L "$payload_dir/$relative_path" ]] ||
    die "materials payload is missing $relative_path"
done

# Close the only automatic publication/activation trigger before any database
# or configuration mutation. Missing units are acceptable on a first install;
# an existing unit that remains active or enabled is not.
systemctl disable --now henukit-materials-webhook.path >/dev/null 2>&1 || true
systemctl stop henukit-materials-runner.service >/dev/null 2>&1 || true
! systemctl is-active --quiet henukit-materials-runner.service || die "materials runner did not stop"
! systemctl is-enabled --quiet henukit-materials-webhook.path || die "materials runner path did not disable"

file_mode() {
  stat -c '%a' "$1"
}

file_owner() {
  stat -c '%u:%g' "$1"
}

require_root_file() {
  local path="$1"
  local label="$2"
  local exact_mode="${3:-}"
  [[ -f "$path" && ! -L "$path" ]] || die "$label must be a regular non-symlink file"
  [[ "$(file_owner "$path")" == "0:0" ]] || die "$label must be owned by root:root"
  if [[ -n "$exact_mode" ]]; then
    [[ "$(file_mode "$path")" == "$exact_mode" ]] || die "$label must have mode $exact_mode"
  elif (( 8#$(file_mode "$path") & 8#022 )); then
    die "$label must not be writable by group or other"
  fi
}

incoming_paths=()
cleanup() {
  local path
  for path in "${incoming_paths[@]-}"; do
    case "$path" in
      /etc/henukit-deploy/.materials-*|/opt/henukit-backups/materials-study/.materials-*|/usr/local/bin/.henukit-*|/usr/local/libexec/henukit/.henukit-*|/etc/systemd/system/.henukit-*)
        [[ ! -e "$path" || -f "$path" ]] && rm -f -- "$path"
        ;;
    esac
  done
}
trap cleanup EXIT

atomic_install() {
  local source="$1"
  local target="$2"
  local mode="$3"
  local target_directory temporary
  target_directory="$(dirname "$target")"
  temporary="$(mktemp "$target_directory/.henukit-$(basename "$target").XXXXXX")"
  incoming_paths+=("$temporary")
  install -o root -g root -m "$mode" "$source" "$temporary"
  mv -T "$temporary" "$target"
}

runtime_backup_root="/opt/henukit-materials/runtime-backups/$release_sha"
install -d -o root -g root -m 0700 "$runtime_backup_root"
backup_existing_target() {
  local target="$1"
  local backup="$runtime_backup_root$target"
  [[ -e "$target" ]] || return 0
  require_root_file "$target" "existing runtime target $target"
  if [[ ! -e "$backup" ]]; then
    install -d -o root -g root -m 0700 "$(dirname "$backup")"
    cp -a -- "$target" "$backup"
  fi
  require_root_file "$backup" "runtime backup $backup"
}

activate_config="/etc/henukit-deploy/materials-activate.env"
require_root_file "$activate_config" "materials activation configuration" "600"
retired_key_pattern='^HENUKIT_MATERIALS_(CONVERTER|IMPORTER|PSQL|PG_SERVICE_FILE|PG_SERVICE|LEGACY_INVENTORY)='
retired_keys=(
  HENUKIT_MATERIALS_CONVERTER
  HENUKIT_MATERIALS_IMPORTER
  HENUKIT_MATERIALS_PSQL
  HENUKIT_MATERIALS_PG_SERVICE_FILE
  HENUKIT_MATERIALS_PG_SERVICE
  HENUKIT_MATERIALS_LEGACY_INVENTORY
)
retired_key_count=0
for retired_key in "${retired_keys[@]}"; do
  retired_key_occurrences="$(grep -c "^${retired_key}=" "$activate_config" || true)"
  [[ "$retired_key_occurrences" =~ ^[0-9]+$ && "$retired_key_occurrences" -le 1 ]] ||
    die "materials activation configuration contains duplicate retired key: $retired_key"
  retired_key_count="$((retired_key_count + retired_key_occurrences))"
done
activate_backup=""
if [[ "$retired_key_count" -gt 0 ]]; then
  config_backup_dir="/etc/henukit-deploy/backups"
  activate_backup="$config_backup_dir/materials-activate.env.pre-oss-only-$release_sha"
  install -d -o root -g root -m 0700 "$config_backup_dir"
  if [[ ! -e "$activate_backup" ]]; then
    install -o root -g root -m 0400 "$activate_config" "$activate_backup"
  fi
  require_root_file "$activate_backup" "materials activation configuration backup" "400"
  config_incoming="$(mktemp "/etc/henukit-deploy/.materials-activate-${release_sha}.XXXXXX")"
  incoming_paths+=("$config_incoming")
  awk '$0 !~ /^HENUKIT_MATERIALS_(CONVERTER|IMPORTER|PSQL|PG_SERVICE_FILE|PG_SERVICE|LEGACY_INVENTORY)=/' "$activate_config" > "$config_incoming"
  [[ "$(grep -Ec "$retired_key_pattern" "$config_incoming" || true)" == "0" ]] ||
    die "retired materials key remains in migrated configuration"
  chown root:root "$config_incoming"
  chmod 0600 "$config_incoming"
  mv -T "$config_incoming" "$activate_config"
fi

retired_dir="/opt/henukit-materials/retired/$release_sha"
retire_root_file() {
  local source="$1"
  local label="$2"
  local exact_mode="${3:-}"
  [[ -e "$source" ]] || return 0
  require_root_file "$source" "$label" "$exact_mode"
  local retired_target="$retired_dir/$(basename "$source")"
  install -d -o root -g root -m 0700 "$retired_dir"
  [[ ! -e "$retired_target" ]] || die "$label backup already exists while source is still active"
  mv -T "$source" "$retired_target"
  require_root_file "$retired_target" "retired $label" "$exact_mode"
}

retire_root_file "/usr/local/libexec/henukit/convert-henukit-slides.py" "converter"
retire_root_file "/usr/local/libexec/henukit/import-henukit-materials.mjs" "Study importer"
retire_root_file "/etc/henukit-deploy/materials-postgresql.conf" "Study PostgreSQL credential" "600"
retire_root_file "/etc/henukit-deploy/materials-legacy-inventory.json" "Study legacy inventory" "600"

install -d -o root -g root -m 0755 /usr/local/bin /usr/local/libexec/henukit /etc/systemd/system

declare -a install_records=(
  "bin/henukit-deploy-webhook|/usr/local/bin/henukit-deploy-webhook|0755"
  "bin/materials-oss-canary|/usr/local/libexec/henukit/materials-oss-canary|0700"
  "bin/materials-oss-release|/usr/local/libexec/henukit/materials-oss-release|0700"
  "bin/library-activate-public-release|/usr/local/libexec/henukit/library-activate-public-release|0700"
  "libexec/henukit-materials-orchestrate|/usr/local/libexec/henukit/henukit-materials-orchestrate|0700"
  "libexec/henukit-materials-prepare|/usr/local/libexec/henukit/henukit-materials-prepare|0755"
  "libexec/henukit-materials-seal|/usr/local/libexec/henukit/henukit-materials-seal|0700"
  "libexec/henukit-materials-activate|/usr/local/libexec/henukit/henukit-materials-activate|0700"
  "libexec/henukit-materials-publish-oss|/usr/local/libexec/henukit/henukit-materials-publish-oss|0700"
  "libexec/henukit-materials-publish-release-oss|/usr/local/libexec/henukit/henukit-materials-publish-release-oss|0700"
  "libexec/prepare-henukit-materials.mjs|/usr/local/libexec/henukit/prepare-henukit-materials.mjs|0644"
  "libexec/seal-henukit-materials.mjs|/usr/local/libexec/henukit/seal-henukit-materials.mjs|0600"
  "libexec/activate-henukit-materials.mjs|/usr/local/libexec/henukit/activate-henukit-materials.mjs|0600"
  "libexec/build-henukit-library-activation-bundle.mjs|/usr/local/libexec/henukit/build-henukit-library-activation-bundle.mjs|0600"
  "systemd/henukit-materials-webhook.service|/etc/systemd/system/henukit-materials-webhook.service|0644"
  "systemd/henukit-materials-webhook.path|/etc/systemd/system/henukit-materials-webhook.path|0644"
  "systemd/henukit-materials-runner.service|/etc/systemd/system/henukit-materials-runner.service|0644"
)
for record in "${install_records[@]}"; do
  IFS='|' read -r source target mode <<< "$record"
  backup_existing_target "$target"
  atomic_install "$payload_dir/$source" "$target" "$mode"
done

systemctl daemon-reload
systemctl disable --now henukit-materials-webhook.path >/dev/null
systemctl stop henukit-materials-runner.service >/dev/null
systemctl restart henukit-materials-webhook.service
systemctl is-active --quiet henukit-materials-webhook.service || die "materials receiver is not active"
if systemctl cat henukit-deploy-webhook.service >/dev/null 2>&1; then
  systemctl restart henukit-deploy-webhook.service
  systemctl is-active --quiet henukit-deploy-webhook.service || die "main deploy receiver is not active"
fi
! systemctl is-active --quiet henukit-materials-runner.service || die "materials runner unexpectedly became active"
! systemctl is-enabled --quiet henukit-materials-webhook.path || die "materials runner path unexpectedly became enabled"

printf 'release_sha=%s\n' "$release_sha"
printf 'activation_config_backup=%s\n' "${activate_backup:-unchanged-no-legacy-key}"
printf 'runtime_backup=%s\n' "$runtime_backup_root"
printf 'materials_receiver=active\nmaterials_runner=inactive\nmaterials_runner_path=disabled\n'

#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

if [[ "${EUID}" -ne 0 ]]; then
  echo "run this installer as root" >&2
  exit 77
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_dir="$(cd -- "$script_dir/.." && pwd)"
source_dir="$service_dir"
repository="jry21223/HENU-Kit-DEV"
branch="main"
listen_addr="127.0.0.1:10087"
clone_url=""
enable_command_hook=0
enable_study_hook=0
enable_materials_sync=0

usage() {
  cat <<'USAGE'
usage: sudo install.sh [options]

  --source-dir PATH          deploy-webhook service source directory
  --repository OWNER/NAME    expected GitHub repository
  --branch NAME              deploy branch (default: main)
  --listen-addr HOST:PORT    loopback listener
  --clone-url URL            clone the private repository after the read-only key is registered
  --enable-command-hook      enable the generic root-owned command hook
  --enable-study-hook        enable the existing Study artifact deployment hook
  --enable-materials-sync    install the HENU-Final-Review materials sync webhook
USAGE
}

set_env_value() {
  local file="${1:?}"
  local key="${2:?}"
  local value="${3-}"
  local found=0
  local line
  local temp
  [[ "$key" =~ ^[A-Z0-9_]+$ ]] || { echo "invalid environment key: $key" >&2; exit 64; }
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || {
    echo "environment value for $key contains a newline" >&2
    exit 64
  }

  temp="$(mktemp "${file}.tmp.XXXXXX")"
  if ! {
    while IFS= read -r line || [[ -n "$line" ]]; do
      if [[ "$line" == "${key}="* ]]; then
        printf '%s=%s\n' "$key" "$value"
        found=1
      else
        printf '%s\n' "$line"
      fi
    done < "$file"
    if (( ! found )); then
      printf '%s=%s\n' "$key" "$value"
    fi
  } > "$temp"; then
    rm -f -- "$temp"
    return 1
  fi

  chmod --reference="$file" "$temp"
  chown --reference="$file" "$temp"
  mv -f -- "$temp" "$file"
}

copy_env_value_if_present() {
  local source="${1:?}"
  local destination="${2:?}"
  local key="${3:?}"
  if grep -q "^${key}=" "$source"; then
    local value
    value="$(grep "^${key}=" "$source" | tail -1 | cut -d= -f2-)"
    set_env_value "$destination" "$key" "$value"
  fi
}

unset_env_value() {
  local file="${1:?}"
  local key="${2:?}"
  [[ "$key" =~ ^[A-Z0-9_]+$ ]] || { echo "invalid environment key: $key" >&2; exit 64; }
  sed -i "/^${key}=/d" "$file"
}

validate_materials_runner_env() {
  local file="${1:?}"
  local line
  local key
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || { echo "invalid materials runner config line" >&2; return 1; }
    key="${line%%=*}"
    case "$key" in
      HENUKIT_MATERIALS_ROOT|HENUKIT_MATERIALS_REPOSITORY|HENUKIT_MATERIALS_REPO_URL|HENUKIT_MATERIALS_REPO_REF|HENUKIT_MATERIALS_DATABASE_URL|HENUKIT_MATERIALS_RELEASE_DIR|HENUKIT_MATERIALS_ENV_FILE|HENUKIT_MATERIALS_ACTIVE_SHA_FILE|POSTGRES_USER) ;;
      *) echo "unsupported materials runner config key: $key" >&2; return 1 ;;
    esac
  done < "$file"
  for key in HENUKIT_MATERIALS_ROOT HENUKIT_MATERIALS_REPOSITORY HENUKIT_MATERIALS_REPO_URL HENUKIT_MATERIALS_REPO_REF HENUKIT_MATERIALS_ACTIVE_SHA_FILE; do
    [[ "$(grep -c "^${key}=" "$file")" == 1 ]] || {
      echo "materials runner config requires exactly one $key" >&2
      return 1
    }
  done
}

validate_materials_receiver_env() {
  local file="${1:?}"
  local line
  local key
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || { echo "invalid materials receiver config line" >&2; return 1; }
    key="${line%%=*}"
    case "$key" in
      HENUKIT_WEBHOOK_LISTEN_ADDR|HENUKIT_WEBHOOK_PATH|HENUKIT_WEBHOOK_REPOSITORY|HENUKIT_WEBHOOK_BRANCH|HENUKIT_WEBHOOK_STATE_DIR|HENUKIT_WEBHOOK_MAX_BODY_BYTES|HENUKIT_WEBHOOK_MAX_QUEUE|HENUKIT_WEBHOOK_QUEUE_MODE|HENUKIT_WEBHOOK_PROCESSED_RETENTION) ;;
      *) echo "unsupported materials receiver config key: $key" >&2; return 1 ;;
    esac
  done < "$file"
}

quiesce_materials_unit() {
  local unit="${1:?}"
  local action="${2:?}"
  local load_state
  local active_state
  if ! load_state="$(systemctl show --property=LoadState --value "$unit" 2>/dev/null)"; then
    echo "cannot inspect materials unit: $unit" >&2
    return 1
  fi
  if [[ "$load_state" == not-found ]]; then
    return 0
  fi
  [[ -n "$load_state" ]] || {
    echo "materials unit returned an empty load state: $unit" >&2
    return 1
  }
  case "$action" in
    disable)
      if ! systemctl disable --now "$unit" >/dev/null; then
        echo "cannot disable materials unit: $unit" >&2
        return 1
      fi
      ;;
    stop)
      if ! systemctl stop "$unit" >/dev/null; then
        echo "cannot stop materials unit: $unit" >&2
        return 1
      fi
      ;;
    *)
      echo "unsupported materials quiesce action: $action" >&2
      return 64
      ;;
  esac
  if active_state="$(systemctl is-active "$unit" 2>/dev/null)"; then
    :
  else
    : # `inactive` normally uses status 3; the exact state is checked below.
  fi
  [[ "$active_state" == inactive ]] || {
    echo "materials unit did not reach inactive state: $unit ($active_state)" >&2
    return 1
  }
}

while (( "$#" )); do
  case "$1" in
    --source-dir) source_dir="${2:?}"; shift 2 ;;
    --repository) repository="${2:?}"; shift 2 ;;
    --branch) branch="${2:?}"; shift 2 ;;
    --listen-addr) listen_addr="${2:?}"; shift 2 ;;
    --clone-url) clone_url="${2:?}"; shift 2 ;;
    --enable-command-hook) enable_command_hook=1; shift ;;
    --enable-study-hook) enable_study_hook=1; shift ;;
    --enable-materials-sync) enable_materials_sync=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 64 ;;
  esac
done

[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "invalid repository" >&2; exit 64; }
[[ "$branch" =~ ^[A-Za-z0-9._/-]+$ ]] || { echo "invalid branch" >&2; exit 64; }
[[ "$listen_addr" =~ ^(127\.0\.0\.1|\[::1\]|localhost):[0-9]{2,5}$ ]] || {
  echo "listen address must remain loopback-only" >&2
  exit 64
}
[[ -f "$source_dir/go.mod" && -f "$source_dir/cmd/server/main.go" ]] || {
  echo "invalid source directory: $source_dir" >&2
  exit 66
}

for command in go install systemctl useradd openssl git ssh-keygen runuser sha256sum flock sudo visudo; do
  command -v "$command" >/dev/null || { echo "missing required command: $command" >&2; exit 69; }
done

if ! id henukit-deploy >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/henukit-deploy --shell /usr/sbin/nologin henukit-deploy
fi
install -d -o root -g henukit-deploy -m 0750 /etc/henukit-deploy
install -d -o root -g henukit-deploy -m 0750 /etc/henukit-deploy/hooks.d
install -d -o root -g henukit-deploy -m 0750 /etc/henukit-deploy/approved-shas
install -d -o henukit-deploy -g henukit-deploy -m 0750 /opt/henukit
install -d -o henukit-deploy -g henukit-deploy -m 0750 /opt/henukit/releases
install -d -o henukit-deploy -g henukit-deploy -m 0750 /var/lib/henukit-deploy-webhook
install -d -o root -g root -m 0755 /usr/local/libexec/henukit
install -d -o root -g root -m 0755 /usr/local/share/henukit/migrations/study
install -d -o root -g root -m 0755 /usr/share/doc/henukit-deploy-webhook/hooks
install -d -o root -g root -m 0755 /etc/sudoers.d

if (( enable_materials_sync )); then
  # Freeze the complete old materials pipeline before replacing the shared Go
  # binary, helpers, environment or units. A failed upgrade stays stopped and
  # cannot run a mixed old/new transaction.
  quiesce_materials_unit henukit-materials-webhook.path disable
  quiesce_materials_unit henukit-materials-runner.service stop
  quiesce_materials_unit henukit-materials-webhook.service stop
fi

temp_binary="$(mktemp)"
trap 'rm -f "$temp_binary" "${materials_receiver_stage:-}" "${materials_runner_stage:-}" "${materials_sudoers_stage:-}"' EXIT
(
  cd "$source_dir"
  CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$temp_binary" ./cmd/server
)
install -o root -g root -m 0755 "$temp_binary" /usr/local/bin/henukit-deploy-webhook
install -o root -g root -m 0755 "$source_dir/deploy/henukit-deploy" /usr/local/libexec/henukit/henukit-deploy
install -o root -g root -m 0755 "$source_dir/deploy/henukit-approve-release" /usr/local/sbin/henukit-approve-release
install -o root -g root -m 0644 "$source_dir/deploy/hooks/README.md" /usr/share/doc/henukit-deploy-webhook/hooks/README.md
install -o root -g root -m 0644 "$source_dir/deploy/hooks/10-command.example" /usr/share/doc/henukit-deploy-webhook/hooks/10-command.example
install -o root -g root -m 0644 "$source_dir/deploy/hooks/20-study.example" /usr/share/doc/henukit-deploy-webhook/hooks/20-study.example

if (( enable_command_hook )); then
  install -o root -g root -m 0755 "$source_dir/deploy/hooks/10-command.example" /etc/henukit-deploy/hooks.d/10-command
fi
if (( enable_study_hook )); then
  install -o root -g root -m 0755 "$source_dir/deploy/hooks/20-study.example" /etc/henukit-deploy/hooks.d/20-study
fi

if [[ ! -f /etc/henukit-deploy/webhook.env ]]; then
  sed \
    -e "s|^HENUKIT_WEBHOOK_LISTEN_ADDR=.*|HENUKIT_WEBHOOK_LISTEN_ADDR=$listen_addr|" \
    -e "s|^HENUKIT_WEBHOOK_REPOSITORY=.*|HENUKIT_WEBHOOK_REPOSITORY=$repository|" \
    -e "s|^HENUKIT_WEBHOOK_BRANCH=.*|HENUKIT_WEBHOOK_BRANCH=$branch|" \
    "$source_dir/deploy/webhook.env.example" > /etc/henukit-deploy/webhook.env
fi
chown root:henukit-deploy /etc/henukit-deploy/webhook.env
chmod 0640 /etc/henukit-deploy/webhook.env

if [[ ! -f /etc/henukit-deploy/deploy.env ]]; then
  expected_remote_url="${clone_url:-git@github.com:$repository.git}"
  sed \
    -e "s|^HENUKIT_ALLOWED_REPOSITORY=.*|HENUKIT_ALLOWED_REPOSITORY=$repository|" \
    -e "s|^HENUKIT_GIT_BRANCH=.*|HENUKIT_GIT_BRANCH=$branch|" \
    -e "s|^HENUKIT_EXPECTED_REMOTE_URL=.*|HENUKIT_EXPECTED_REMOTE_URL=$expected_remote_url|" \
    "$source_dir/deploy/deploy.env.example" > /etc/henukit-deploy/deploy.env
fi
chown root:henukit-deploy /etc/henukit-deploy/deploy.env
chmod 0640 /etc/henukit-deploy/deploy.env

if [[ ! -f /etc/henukit-deploy/webhook-secret ]]; then
  openssl rand -hex 32 > /etc/henukit-deploy/webhook-secret
fi
chown root:root /etc/henukit-deploy/webhook-secret
chmod 0400 /etc/henukit-deploy/webhook-secret

ssh_dir=/var/lib/henukit-deploy/.ssh
install -d -o henukit-deploy -g henukit-deploy -m 0700 "$ssh_dir"
if [[ ! -f "$ssh_dir/id_ed25519" ]]; then
  runuser -u henukit-deploy -- ssh-keygen -q -t ed25519 -N '' -C "henukit-production-readonly" -f "$ssh_dir/id_ed25519"
fi
cat > "$ssh_dir/config" <<'SSHCONFIG'
Host github.com
  HostName github.com
  User git
  IdentityFile ~/.ssh/id_ed25519
  IdentitiesOnly yes
  StrictHostKeyChecking yes
SSHCONFIG
chown henukit-deploy:henukit-deploy "$ssh_dir/config"
chmod 0600 "$ssh_dir/config"

if [[ -n "$clone_url" && ! -d /opt/henukit/repository/.git ]]; then
  if [[ ! -f "$ssh_dir/known_hosts" ]]; then
    echo "GitHub host key is not installed. Add a verified github.com host key to $ssh_dir/known_hosts first." >&2
    echo "The generated read-only Deploy Key is:" >&2
    cat "$ssh_dir/id_ed25519.pub" >&2
    exit 78
  fi
  runuser -u henukit-deploy -- git clone --branch "$branch" --single-branch "$clone_url" /opt/henukit/repository
fi

install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-deploy-webhook.service" /etc/systemd/system/henukit-deploy-webhook.service
install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-deploy-webhook.path" /etc/systemd/system/henukit-deploy-webhook.path
install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-deploy-runner.service" /etc/systemd/system/henukit-deploy-runner.service
systemctl daemon-reload
# Start only the receiver during bootstrap. The queue watcher remains disabled
# until the read-only clone, root-owned hooks, Nginx, and controlled release
# verification are complete.
systemctl enable --now henukit-deploy-webhook.service
systemctl disable --now henukit-deploy-webhook.path >/dev/null 2>&1 || true

if (( enable_materials_sync )); then
  # Materials sync webhook: one unprivileged receiver/queue runner. Only the
  # fixed root helper crosses the sudo boundary for the canonical transaction.
  repo_root="$(cd -- "$source_dir/../.." && pwd)"
  for command in node python3 docker; do
    command -v "$command" >/dev/null || { echo "missing materials sync command: $command" >&2; exit 69; }
  done
  python3 -c 'import pptx' >/dev/null 2>&1 || {
    echo "python3-pptx is required for materials sync" >&2
    exit 69
  }
  if ! command -v soffice >/dev/null && ! command -v libreoffice >/dev/null; then
    echo "LibreOffice is required to convert legacy .ppt materials" >&2
    exit 69
  fi
  install -o root -g root -m 0755 "$repo_root/scripts/ops/sync-henukit-materials.sh" /usr/local/libexec/henukit/sync-henukit-materials.sh
  install -o root -g root -m 0755 "$repo_root/scripts/ops/convert-henukit-slides.py" /usr/local/libexec/henukit/convert-henukit-slides.py
  install -o root -g root -m 0755 "$repo_root/scripts/ops/import-henukit-materials.mjs" /usr/local/libexec/henukit/import-henukit-materials.mjs
  install -o root -g root -m 0755 "$repo_root/scripts/ops/henukit-materials-sync.sh" /usr/local/libexec/henukit/henukit-materials-sync
  install -o root -g root -m 0755 "$source_dir/deploy/henukit-materials-root" /usr/local/libexec/henukit/henukit-materials-root
  install -o root -g root -m 0755 "$source_dir/deploy/henukit-materials-sudo" /usr/local/libexec/henukit/henukit-materials-sudo
  install -o root -g root -m 0444 "$repo_root/services/api/migrations/0002_henukit_materials_sync_expand.up.sql" /usr/local/share/henukit/migrations/study/0002_henukit_materials_sync_expand.up.sql
  install -o root -g root -m 0444 "$repo_root/services/api/migrations/0002_henukit_materials_sync_expand.down.sql" /usr/local/share/henukit/migrations/study/0002_henukit_materials_sync_expand.down.sql
  install -o root -g root -m 0444 "$repo_root/services/api/migrations/0002_henukit_materials_sync_expand.md" /usr/local/share/henukit/migrations/study/0002_henukit_materials_sync_expand.md
  # Retire the unsafe historical entrypoint while preserving a fail-closed
  # compatibility name for operators with an old absolute path.
  ln -sfn henukit-materials-sync /usr/local/libexec/henukit/henukit-materials-sync.sh

  materials_sudoers_stage="$(mktemp /etc/sudoers.d/.henukit-materials.XXXXXX)"
  {
    printf '%s\n' 'Defaults!/usr/local/libexec/henukit/henukit-materials-root secure_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin, env_reset'
    printf '%s\n' 'henukit-deploy ALL=(root) NOPASSWD: /usr/local/libexec/henukit/henukit-materials-root *'
  } > "$materials_sudoers_stage"
  chown root:root "$materials_sudoers_stage"
  chmod 0440 "$materials_sudoers_stage"
  visudo -cf "$materials_sudoers_stage"
  mv -f -- "$materials_sudoers_stage" /etc/sudoers.d/henukit-materials
  materials_sudoers_stage=""

  receiver_env=/etc/henukit-deploy/materials-webhook.env
  runner_env=/etc/henukit-deploy/materials-runner.env
  materials_receiver_stage="$(mktemp /etc/henukit-deploy/.materials-webhook.env.XXXXXX)"
  materials_runner_stage="$(mktemp /etc/henukit-deploy/.materials-runner.env.XXXXXX)"
  if [[ -f "$receiver_env" ]]; then
    cp -- "$receiver_env" "$materials_receiver_stage"
  else
    cp -- "$source_dir/deploy/materials.env.example" "$materials_receiver_stage"
  fi
  runner_env_was_present=0
  if [[ -f "$runner_env" ]]; then
    runner_env_was_present=1
    cp -- "$runner_env" "$materials_runner_stage"
  else
    cp -- "$source_dir/deploy/materials-runner.env.example" "$materials_runner_stage"
  fi

  # Copy every legacy privileged value into the complete staged root file
  # before removing anything from the staged receiver file. The root file is
  # atomically installed first, so interruption cannot lose credentials.
  for key in \
    HENUKIT_MATERIALS_ROOT \
    HENUKIT_MATERIALS_REPOSITORY \
    HENUKIT_MATERIALS_REPO_URL \
    HENUKIT_MATERIALS_REPO_REF \
    HENUKIT_MATERIALS_DATABASE_URL \
    HENUKIT_MATERIALS_RELEASE_DIR \
    HENUKIT_MATERIALS_ENV_FILE \
    HENUKIT_MATERIALS_ACTIVE_SHA_FILE \
    POSTGRES_USER; do
    if (( ! runner_env_was_present )) || ! grep -q "^${key}=" "$materials_runner_stage"; then
      copy_env_value_if_present "$materials_receiver_stage" "$materials_runner_stage" "$key"
    fi
  done

  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_LISTEN_ADDR 127.0.0.1:10088
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_PATH /webhooks/materials
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_REPOSITORY jry21223/HENU-Final-Review
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_BRANCH main
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_STATE_DIR /var/lib/henukit-materials-webhook
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_MAX_BODY_BYTES 1048576
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_MAX_QUEUE 1
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_QUEUE_MODE latest
  set_env_value "$materials_receiver_stage" HENUKIT_WEBHOOK_PROCESSED_RETENTION 720h
  for key in \
    HENUKIT_DEPLOY_COMMAND \
    HENUKIT_DEPLOY_TIMEOUT \
    HENUKIT_MATERIALS_ROOT \
    HENUKIT_MATERIALS_REPOSITORY \
    HENUKIT_MATERIALS_REPO_URL \
    HENUKIT_MATERIALS_REPO_REF \
    HENUKIT_MATERIALS_DATABASE_URL \
    HENUKIT_MATERIALS_RELEASE_DIR \
    HENUKIT_MATERIALS_ENV_FILE \
    HENUKIT_MATERIALS_ACTIVE_SHA_FILE \
    POSTGRES_USER; do
    unset_env_value "$materials_receiver_stage" "$key"
  done

  for key in \
    HENUKIT_WEBHOOK_STATE_DIR \
    HENUKIT_WEBHOOK_MAX_QUEUE \
    HENUKIT_WEBHOOK_QUEUE_MODE \
    HENUKIT_WEBHOOK_PROCESSED_RETENTION \
    HENUKIT_DEPLOY_COMMAND \
    HENUKIT_DEPLOY_TIMEOUT; do
    unset_env_value "$materials_runner_stage" "$key"
  done
  if ! grep -q '^HENUKIT_MATERIALS_ROOT=' "$materials_runner_stage"; then
    set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_ROOT /opt/henukit-materials
  fi
  set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_REPOSITORY jry21223/HENU-Final-Review
  set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_REPO_URL https://github.com/jry21223/HENU-Final-Review.git
  set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_REPO_REF main
  if ! grep -q '^HENUKIT_MATERIALS_ACTIVE_SHA_FILE=' "$materials_runner_stage"; then
    set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_ACTIVE_SHA_FILE /var/lib/henukit-actions-watch/last-activated-sha
  fi
  if grep -q '^HENUKIT_MATERIALS_ENV_FILE=' "$materials_runner_stage"; then
    current_materials_env="$(grep '^HENUKIT_MATERIALS_ENV_FILE=' "$materials_runner_stage" | tail -1 | cut -d= -f2-)"
    if [[ "$current_materials_env" == /opt/henukit/.env.henukit ]]; then
      # The historical default sits below a henukit-deploy-owned directory and
      # cannot be consumed safely by path after validation. Point upgrades at
      # the root-controlled copy; the watcher remains disabled until operators
      # populate and verify it using the runbook.
      set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_ENV_FILE /etc/henukit-deploy/materials-production.env
    fi
  else
    set_env_value "$materials_runner_stage" HENUKIT_MATERIALS_ENV_FILE /etc/henukit-deploy/materials-production.env
  fi

  validate_materials_runner_env "$materials_runner_stage"
  validate_materials_receiver_env "$materials_receiver_stage"

  chown root:root "$materials_runner_stage"
  chmod 0600 "$materials_runner_stage"
  chown root:henukit-deploy "$materials_receiver_stage"
  chmod 0640 "$materials_receiver_stage"
  mv -f -- "$materials_runner_stage" "$runner_env"
  materials_runner_stage=""
  mv -f -- "$materials_receiver_stage" "$receiver_env"
  materials_receiver_stage=""
  if [[ ! -f /etc/henukit-deploy/materials-webhook-secret ]]; then
    openssl rand -hex 32 > /etc/henukit-deploy/materials-webhook-secret
  fi
  chown root:root /etc/henukit-deploy/materials-webhook-secret
  chmod 0400 /etc/henukit-deploy/materials-webhook-secret

  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-webhook.service" /etc/systemd/system/henukit-materials-webhook.service
  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-webhook.path" /etc/systemd/system/henukit-materials-webhook.path
  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-runner.service" /etc/systemd/system/henukit-materials-runner.service
  systemctl daemon-reload
  quiesce_materials_unit henukit-materials-webhook.path disable
  quiesce_materials_unit henukit-materials-runner.service stop
  # Start a fresh receiver process so the binary, queue mode and secret split
  # are live. The watcher remains disabled until controlled validation.
  systemctl enable henukit-materials-webhook.service
  systemctl restart henukit-materials-webhook.service
fi

fingerprint="$(sha256sum /etc/henukit-deploy/webhook-secret | awk '{print $1}')"
echo
printf 'Webhook receiver installed. Secret SHA-256 fingerprint: %s\n' "$fingerprint"
if (( enable_materials_sync )); then
  materials_fingerprint="$(sha256sum /etc/henukit-deploy/materials-webhook-secret | awk '{print $1}')"
  echo
  printf 'Materials sync webhook installed. Secret SHA-256 fingerprint: %s\n' "$materials_fingerprint"
fi
echo "Read-only Deploy Key (add it to the repository, without write access):"
cat "$ssh_dir/id_ed25519.pub"
echo
echo "Next mandatory steps:"
echo "  1. Add a verified github.com host key to $ssh_dir/known_hosts."
echo "  2. Clone the private repository as henukit-deploy into /opt/henukit/repository."
echo "  3. Configure at least one root-owned hook in /etc/henukit-deploy/hooks.d."
echo "  4. Install the Nginx location from deploy/nginx.conf.example behind HTTPS."
echo "  5. Add a GitHub repository webhook for push events only, using the secret file."
echo "  6. Send Ping, then enable the queue watcher only after hooks and rollback are verified:"
echo "       systemctl enable --now henukit-deploy-webhook.path"
echo "  7. Execute a controlled push and verify /statusz plus the live release SHA."

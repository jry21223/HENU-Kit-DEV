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

for command in go install systemctl useradd openssl git ssh-keygen runuser sha256sum flock; do
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
install -d -o root -g root -m 0755 /usr/share/doc/henukit-deploy-webhook/hooks

temp_binary="$(mktemp)"
trap 'rm -f "$temp_binary"' EXIT
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
  # Materials sync webhook: 第二个接收器实例 + runner,共享同一二进制。
  # runner 以 root 运行,需要 docker compose exec、git 与镜像目录写权限。
  repo_root="$(cd -- "$source_dir/../.." && pwd)"
  install -o root -g root -m 0755 "$repo_root/scripts/ops/sync-henukit-materials.sh" /usr/local/libexec/henukit/sync-henukit-materials.sh
  install -o root -g root -m 0755 "$repo_root/scripts/ops/convert-henukit-slides.py" /usr/local/libexec/henukit/convert-henukit-slides.py
  install -o root -g root -m 0755 "$repo_root/scripts/ops/import-henukit-materials.mjs" /usr/local/libexec/henukit/import-henukit-materials.mjs
  install -o root -g root -m 0755 "$repo_root/scripts/ops/henukit-materials-sync.sh" /usr/local/libexec/henukit/henukit-materials-sync.sh

  if [[ ! -f /etc/henukit-deploy/materials-webhook.env ]]; then
    install -o root -g henukit-deploy -m 0640 "$source_dir/deploy/materials.env.example" /etc/henukit-deploy/materials-webhook.env
  fi
  if [[ ! -f /etc/henukit-deploy/materials-webhook-secret ]]; then
    openssl rand -hex 32 > /etc/henukit-deploy/materials-webhook-secret
  fi
  chown root:root /etc/henukit-deploy/materials-webhook-secret
  chmod 0400 /etc/henukit-deploy/materials-webhook-secret

  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-webhook.service" /etc/systemd/system/henukit-materials-webhook.service
  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-webhook.path" /etc/systemd/system/henukit-materials-webhook.path
  install -o root -g root -m 0644 "$source_dir/deploy/systemd/henukit-materials-runner.service" /etc/systemd/system/henukit-materials-runner.service
  systemctl daemon-reload
  # 与主接收器同规则:先只启用接收器,验证 Nginx/GitHub webhook 后再启用队列。
  systemctl enable --now henukit-materials-webhook.service
  systemctl disable --now henukit-materials-webhook.path >/dev/null 2>&1 || true
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

# HENU Kit 一键本地构建部署（Actions 额度耗尽时）

> 本文记录 2026-08-16 首次成功跑通的"GitHub Actions 额度耗尽 → WSL2 本地签名构建 → 生产部署"完整实战流程。所有坑和修复都在这里，按序执行即可复现。正式发布仍以 `henukit-artifact-deployment.md` 为准；本文是它的**单人运维可执行简化版**。

## 适用范围

- GitHub Actions 额度耗尽，`deploy-henukit.yml` 无法在 CI 跑
- 需要从 WSL2 本地构建签名发布包并部署到生产 `8.146.200.82`
- 生产当前已激活 `0c268509`（16 镜像，含 career-opportunities + food-mcp）

## 架构回顾

```
Mac (开发) ──ssh──> WSL2 (jerry-wsl) ──docker容器模拟builder身份──> 签名构建(18镜像)
                                                        │
WSL ──ssh henu-prod(修正KEX)──> 生产 8.146.200.82:22222 ──rsync bundle──> activate-henukit-release
```

**关键事实（本次踩坑总结）：**

1. **镜像清单现为 18 个**：`henukit-release-images.sh` 含 `food-mcp`（ADR-0033）、`career-opportunities`（#392）、`career-mcp`（ADR-0034）和 `quizcraft`（方案 2 容器化 Go core）。`docker-compose.henukit.yml` 的每个服务都必须同时在 inventory 和 `docker-compose.henukit.prebuilt.yml` 中有固定镜像。
2. **career-opportunities 是 conditional 角色**（不是 baseline），否则旧 release（14 镜像时代）无法通过回滚验证。
3. **直连 SSH 到生产会被中间设备干扰 KEX**（卡在 `expecting SSH2_MSG_KEX_ECDH_REPLY`），必须用 `KexAlgorithms diffie-hellman-group14-sha256`。
4. **生产 `.env.henukit` 必须包含全部 compose 必填变量**，新增服务（career/food-mcp）需要手动补 env + 建数据库。
5. **WSL git checkout 权限是 775**，会触发 verify 的 `trusted file must not be group- or world-writable`；构建前需 `chmod 755`（且 `henukit-materials-sync.sh` 在 git 里是 644，不能 chmod）。
6. **生产磁盘紧张**：部署前检查 `df -h /`，必要时清理旧 bundle/release。

---

## 一、前置检查（每次部署前）

### 1.1 生产磁盘

```bash
ssh quizcraft-prod 'df -h / | tail -1'
# 需要 ≥1.5G 可用；不足时清理（见第五节）
```

### 1.2 生产 inventory 版本

```bash
ssh quizcraft-prod '/usr/local/sbin/henukit-release-images.sh --records | wc -l'
# 必须 = 18。若 <18，从本地提取新 inventory 覆盖（见第六节）
```

### 1.3 生产 env 必填变量

```bash
ssh quizcraft-prod "grep -E '^CAREER_DATABASE_URL|^CAREER_CLIENT_SECRET|^FOOD_POST_CREATE_SECRET|^FOOD_POST_READ_SECRET|^FOOD_MCP_ACCESS_TOKEN|^CAREER_MCP_ACCESS_TOKEN|^PORTAL_DEPLOYED_AT|^PORTAL_VERSION' /opt/henukit/.env.henukit | sed 's/=.*/=[set]/'"
# 全都要有。缺失时见第七节补齐
```

### 1.4 career 数据库存在

```bash
ssh quizcraft-prod "docker exec henukit-postgres-1 psql -U henukit -d postgres -t -c \"SELECT datname FROM pg_database WHERE datname='career'\""
# 必须返回 career
```

---

## 二、构建签名发布包（WSL2）

### 2.1 更新 WSL 仓库到最新 main

```bash
ssh jerry-wsl
cd ~/HENU-Kit-DEV-career-radar-364
export https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890
git fetch origin main && git reset --hard origin/main
git log --oneline -1
```

### 2.2 修正 WSL 脚本权限（关键！）

```bash
docker run --rm -v /home/jerry/HENU-Kit-DEV-career-radar-364:/repo \
  alpine sh -c 'apk add --no-cache git >/dev/null 2>&1; git config --global --add safe.directory /repo; cd /repo; \
  find scripts/ops -maxdepth 1 -name "*.sh" -type f -perm -111 -exec chmod 755 {} \; ; \
  chmod 644 scripts/ops/henukit-materials-sync.sh; \
  git status --porcelain --untracked-files=all | wc -l'
# 输出必须是 0（干净）。否则 verify 会报 trusted file group-writable
```

### 2.3 容器内签名构建（18 镜像）

```bash
cat > /tmp/henukit-build.sh <<'SCRIPT'
#!/bin/sh
apk add --no-cache bash git openssh docker-cli gzip coreutils shadow nodejs npm findutils 2>&1 | tail -1
chmod 755 /repo/scripts/ops/*.sh 2>/dev/null
addgroup -S henukit-release-deployers 2>/dev/null
usermod -aG henukit-release-deployers root 2>/dev/null
export HOME=/root
git config --global --add safe.directory /repo
git config --global credential.https://github.com.helper "!/usr/local/bin/gh auth git-credential"
cd /repo
export https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890
export DOCKER_BUILDKIT=1
su root -c "bash scripts/ops/build-henukit-release-local.sh --sha \$(git rev-parse HEAD) --output-dir /home/jerry/henukit-signed --signing-key /keys/ed25519 --handoff-group henukit-release-deployers"
echo "BUILD_EXIT:$?"
SCRIPT
chmod +x /tmp/henukit-build.sh

docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /home/jerry/HENU-Kit-DEV-career-radar-364:/home/jerry/HENU-Kit-DEV-career-radar-364 \
  -v /home/jerry/HENU-Kit-DEV-career-radar-364:/repo \
  -v /etc/henukit-release-builder:/keys:ro \
  -v /home/jerry/henukit-signed:/home/jerry/henukit-signed \
  -v /home/jerry/.local/bin/gh:/usr/local/bin/gh:ro \
  -v /home/jerry/.config/gh:/root/.config/gh:ro \
  -v /usr/libexec/docker/cli-plugins/docker-buildx:/usr/local/libexec/docker/cli-plugins/docker-buildx:ro \
  -v /home/jerry/.docker/cli-plugins:/root/.docker/cli-plugins:ro \
  --network host \
  -e http_proxy=http://127.0.0.1:7890 -e https_proxy=http://127.0.0.1:7890 \
  -v /tmp/henukit-build.sh:/build.sh:ro \
  alpine sh /build.sh 2>&1 | tail -5
```

**要点：**
- 容器以 root 模拟 builder，`/repo` 挂载**可写**（否则 chmod 失败）
- `DOCKER_BUILDKIT=1` + 挂载宿主 buildx 插件（否则 `--mount=type=cache` 报错）
- 容器内装 `nodejs npm findutils`（runtime 打包要 node + GNU find `-printf`）
- 签名 key 从 `/etc/henukit-release-builder/ed25519` 挂载（只读）
- 产物在 `/home/jerry/henukit-signed/henukit-release-<sha>/`（41 个文件：18 镜像×2 + runtime×2 + RELEASE_SHA + manifest + sig）

### 2.4 验证产物

```bash
docker run --rm -v /home/jerry/henukit-signed:/s:ro alpine sh -c \
  "ls /s/henukit-release-\$(ls /s | grep -v build-inner | head -1 | sed 's/henukit-release-//')/ | wc -l"
# 必须 = 39
```

---

## 三、部署到生产

### 3.1 准备修正版 SSH config（KEX 修复）

```bash
docker run --rm -v /home/henukit-deployer/.ssh:/hs:ro alpine sh -c \
  "sed 's/KexAlgorithms curve25519-sha256/KexAlgorithms diffie-hellman-group14-sha256/' /hs/config" \
  > /tmp/henu-prod-config-fixed.txt
grep KexAlgorithms /tmp/henu-prod-config-fixed.txt   # 必须显示 diffie-hellman-group14-sha256
```

### 3.2 部署（带重试，直连偶发超时）

```bash
SHA=$(git rev-parse HEAD)
cat > /tmp/henukit-deploy.sh <<'SCRIPT'
#!/bin/sh
apk add --no-cache bash git rsync openssh 2>&1 | tail -1
export HOME=/root
cd /repo
export https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890
git config --global --add safe.directory /repo
git config --global credential.https://github.com.helper "!/usr/local/bin/gh auth git-credential"
for attempt in 1 2 3 4 5 6 7 8; do
  echo "=== deploy attempt $attempt ==="
  if bash scripts/ops/deploy-henukit-release-from-wsl.sh \
    --sha $SHA \
    --artifact-dir /srv/artifacts/henukit-release-$SHA \
    --allowed-signers /etc/henukit-release-deployer/release-signers \
    --remote-env-file /opt/henukit/.env.henukit \
    --account-operator-role operations-operator \
    --execute; then
    echo "=== DEPLOY SUCCESS on attempt $attempt ==="
    exit 0
  fi
  echo "=== attempt $attempt failed, waiting 40s ==="
  sleep 40
done
echo "=== ALL ATTEMPTS FAILED ==="
exit 1
SCRIPT

docker run --rm \
  -v /home/jerry/HENU-Kit-DEV-career-radar-364:/repo:ro \
  -v /home/jerry/henukit-signed:/srv/artifacts:ro \
  -v /tmp/henu-prod-config-fixed.txt:/root/.ssh/config \
  -v /home/henukit-deployer/.ssh/id_ed25519_henu_prod:/home/henukit-deployer/.ssh/id_ed25519_henu_prod \
  -v /home/henukit-deployer/.ssh/known_hosts:/home/henukit-deployer/.ssh/known_hosts \
  -v /etc/henukit-release-deployer/release-signers:/etc/henukit-release-deployer/release-signers:ro \
  -v /home/jerry/.local/bin/gh:/usr/local/bin/gh:ro \
  -v /home/jerry/.config/gh:/root/.config/gh:ro \
  --network host \
  -e http_proxy=http://127.0.0.1:7890 -e https_proxy=http://127.0.0.1:7890 \
  -v /tmp/henukit-deploy.sh:/dep.sh:ro \
  alpine sh -c "chown root:root /root/.ssh/config /home/henukit-deployer/.ssh/id_ed25519_henu_prod /home/henukit-deployer/.ssh/known_hosts 2>/dev/null; chmod 600 /root/.ssh/config /home/henukit-deployer/.ssh/id_ed25519_henu_prod 2>/dev/null; sh /dep.sh" 2>&1 | tail -10
```

**要点：**
- `/root/.ssh/config` 用修正版（KEX），`known_hosts` 挂到 config 引用的路径 `/home/henukit-deployer/.ssh/known_hosts`
- 部署脚本幂等：bundle 已在生产时会复用不重传
- 直连 SSH 偶发超时（网络波动），重试 8 次每次间隔 40s

### 3.3 激活失败的常见修复

| 报错 | 修复 |
|---|---|
| `required variable CAREER_DATABASE_URL is missing` | 生产 env 补变量（第七节） |
| `required variable PORTAL_DEPLOYED_AT/PORTAL_VERSION is missing` | 同上 |
| `no healthy fixed-SHA rollback release is ready` | career 角色必须 conditional；生产 inventory 更新到 18 镜像 |
| `unexpected artifact file henukit-career-opportunities-...` | 生产 inventory 旧（14 镜像），更新（第六节） |
| `an approval already exists for release ...` | `ssh quizcraft-prod "rm -f /var/lib/henukit-actions-watch/approvals/<sha>"` |
| 磁盘满 `No space left on device` | 清理（第五节） |

---

## 四、验证部署

```bash
ssh quizcraft-prod '
  echo "=== last-activated ==="; cat /var/lib/henukit-actions-watch/last-activated-sha
  echo; echo "=== 容器镜像 ==="; docker ps --format "{{.Names}} {{.Image}}" | grep henukit | grep -vE "nginx|postgres|redis" | awk "{print \$2}" | sort | uniq -c
  echo "=== 健康 ==="; docker ps --format "{{.Names}} {{.Status}}" | grep -c Up
  echo "=== 路由 ==="
  for p in / /practice /library /career /api/v1/healthz; do printf "%s -> " $p; curl -s -o /dev/null -w "%{http_code}\n" https://henukit.cn$p; done
'
```

**成功标准：** last-activated = 目标 SHA；18 个容器镜像全为目标 SHA；21 容器 Up；全部路由 200。

---

## 五、生产磁盘清理（磁盘满时）

```bash
ssh quizcraft-prod '
  df -h / | tail -1
  # 1) 旧 incoming bundle（保留当前目标）
  ls -la /opt/henukit-incoming/ | grep henukit-release-
  rm -rf /opt/henukit-incoming/henukit-release-<旧sha1> ...
  # 2) 7月旧 releases
  rm -rf /opt/henukit-releases/0b302280* /opt/henukit-releases/0408e8cc* ...
  # 3) 恢复 current 链接到当前激活基线
  ln -sfn /opt/henukit/releases/<当前sha> /opt/henukit/current
'
```

---

## 六、生产 inventory 更新（新增服务时）

```bash
# 本地提取新 inventory（从 runtime 包或仓库）
ssh jerry-wsl
cd ~/HENU-Kit-DEV-career-radar-364
cp scripts/ops/henukit-release-images.sh /tmp/inventory-latest.sh

# 传到生产（走代理 + KEX 修复）
scp -o ProxyCommand="nc -x 127.0.0.1:7890 %h %p" \
  -o KexAlgorithms=diffie-hellman-group14-sha256 \
  -P 22222 -i ~/.ssh/meta-deploy-key \
  /tmp/inventory-latest.sh root@8.146.200.82:/tmp/inventory-latest.sh

# 安装
ssh quizcraft-prod '
  cp /tmp/inventory-latest.sh /usr/local/sbin/henukit-release-images.sh
  chmod 555 /usr/local/sbin/henukit-release-images.sh
  /usr/local/sbin/henukit-release-images.sh --records | wc -l   # 必须 = 18
'
```

---

## 七、生产 env 补齐（新增服务时）

```bash
ssh quizcraft-prod '
  # career 数据库（首次）
  docker exec henukit-postgres-1 psql -U henukit -d postgres -c "CREATE DATABASE career OWNER henukit;"

  # 追加变量（值用 openssl rand 生成）
  cat >> /opt/henukit/.env.henukit <<EOF

CAREER_DATABASE_URL=postgres://henukit:henukit_dev_change_me@postgres:5432/career?sslmode=disable
CAREER_REDIS_URL=redis://redis:6379/6
CAREER_CLIENT_ID=portal-gateway-career
CAREER_CLIENT_SECRET=career-service-$(openssl rand -hex 16)
CAREER_KEY_ID=career-key-1
CAREER_URL=http://career-opportunities:8097
FOOD_POSTS_URL=http://food:8096
FOOD_POST_CREATE_CLIENT_ID=portal-gateway-food-post-create
FOOD_POST_CREATE_SECRET=food-post-create-$(openssl rand -hex 16)
FOOD_POST_CREATE_KEY_ID=food-post-create-key
FOOD_POST_READ_CLIENT_ID=portal-gateway-food-post-read
FOOD_POST_READ_SECRET=food-post-read-$(openssl rand -hex 16)
FOOD_POST_READ_KEY_ID=food-post-read-key
FOOD_MCP_ACCESS_TOKEN=mcp-$(openssl rand -hex 16)
CAREER_MCP_ACCESS_TOKEN=mcp-$(openssl rand -hex 16)
PORTAL_DEPLOYED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
PORTAL_VERSION=<sha前12位>
EOF
'
```

**验证 env 完整**（对比 compose 必填）：

```bash
ssh quizcraft-prod '
  grep -oE "[A-Z_]+ is required" /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml 2>/dev/null | awk "{print \$1}" | sort -u > /tmp/required.txt
  while read v; do grep -q "^${v}=" /opt/henukit/.env.henukit || echo "MISSING: $v"; done < /tmp/required.txt
  # 无输出 = 全部齐备
  docker compose --env-file /opt/henukit/.env.henukit \
    -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml config --quiet
  # EXIT:0 = 插值通过
'
```

---

## 八、新增服务时修改清单（三步缺一不可）

1. **`scripts/ops/henukit-release-images.sh`**：`release_names/images/services/roles/contexts/dockerfiles/build_args` 六个数组各加一项（新服务 role 用 conditional 除非必需）
2. **`docker-compose.henukit.prebuilt.yml`**：加 `image: henukit-<name>:${RELEASE_SHA:?}` + `build: !reset null` + env（必填用 `:?`）
3. **测试**：`scripts/ops/tests/henukit-release-images.test.mjs`（expected 数组）+ `deploy-henukit-workflow.test.mjs`（两处 expectedImages + requiredEnvironment）

验证：

```bash
bash scripts/ops/henukit-release-images.sh --check
node --test scripts/ops/tests/henukit-release-images.test.mjs
node --test scripts/ops/tests/deploy-henukit-workflow.test.mjs
node scripts/ops/check-account-production-boundary.mjs
```

---

## 九、回滚

生产激活失败会自动回滚到上一激活 SHA（`last-activated-sha` 之前的固定镜像）。手动回滚：

```bash
ssh quizcraft-prod '
  # 用保留的旧 release 目录 + 旧 env
  sudo GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
    HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
    HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operations-operator \
    /usr/local/sbin/activate-henukit-release <旧sha> --local-artifacts /opt/henukit-incoming/henukit-release-<旧sha> --execute
'
```

---

## 附：本次部署（2026-08-16）实战时间线

| 时间 | 事件 |
|---|---|
| 13:30 | 发现 inventory 缺 food-mcp/career → 修复到 16 镜像 + prebuilt + 测试 |
| 14:30 | 容器签名构建 16 镜像成功 |
| 15:10 | 首次部署失败：生产 inventory 旧（14）→ 更新 |
| 15:30 | 部署失败：career baseline 导致旧基线验证失败 → 改 conditional |
| 15:50 | 部署失败：生产 env 缺 CAREER/FOOD_MCP/PORTAL 变量 → 补齐 |
| 16:00 | **激活成功：16 容器全跑 0c268509，网站 200** |

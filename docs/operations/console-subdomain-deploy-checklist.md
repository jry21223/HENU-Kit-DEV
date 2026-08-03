# console.henukit.cn 子域切换服务器侧部署清单

> 状态：进行中（DNS 已切，服务器侧未完成）
> 更新日期：2026-08-03
> 适用场景：Console 管理后台子域 `console.henukit.cn` 切换的服务器侧 Go/No-Go 执行与验收
> 依据：[`henukit-domain-cutover.md`](./henukit-domain-cutover.md)（Console subdomain cutover 章节）、`infra/nginx/henukit-host.conf.example`、`infra/nginx/henukit.conf.example`、`docker-compose.henukit.yml` / `docker-compose.henukit.prod-host.yml`

**背景**：Console 部署到独立子域，是为了让同源 `/api/*` 请求到达 Console Gateway 而不是 Portal Gateway（共享 edge 的 `/api/` 属于 Portal）。DNS 已切换，但服务器侧尚未落地：当前 `console.henukit.cn:443` 返回的是 `superhuazai.me` 旧证书（请求落入 Nginx 默认 server），而 `henukit.cn` apex HTTPS 正常（证书 `/etc/letsencrypt/live/henukit.cn/`）。

**核心事实（已核实）**：

| 项目 | 值 |
|---|---|
| 源服务器 | 阿里云 ECS cn-beijing，`8.146.200.82`，Ubuntu Nginx 1.24 |
| SSH | `root@8.146.200.82:22222` |
| 权威 DNS | Cloudflare（`brynne/leo.ns.cloudflare.com`），`console.henukit.cn` A 记录已生效，**Proxy status = DNS only（灰云）** |
| 生产 vhost | `/etc/nginx/sites-enabled/henukit.cn`（legacy `/etc/nginx/sites-enabled/superhuazai.me` 保持启用） |
| 现有证书 | `/etc/letsencrypt/live/henukit.cn/`（覆盖 `henukit.cn` + `www.henukit.cn`，有效期至 2026-10-28） |
| Compose edge | `127.0.0.1:8088`；console-gateway host 暴露 `127.0.0.1:8082`（生产 overlay） |
| 环境文件 | `/opt/henukit/.env.henukit`（root-owned） |
| 生产 Compose | 固定 SHA release 目录 `/opt/henukit-releases/<sha>/docker-compose.henukit.release.yml`（生产不执行 `docker build`） |

**执行顺序说明**：证书签发（第 2 节）依赖第 3 节的 80 端口 vhost（acme-challenge location）先行落地；实际执行顺序为 1 → 3a → 2 → 3b → 4 → 5 → 6 → 7。

---

## 1. 前置条件核对

- [ ] **DNS 记录生效**：从多解析器验证 `console.henukit.cn` A 记录指向 `8.146.200.82`。

  ```bash
  # 系统解析器
  dig +short console.henukit.cn A
  # 显式 Cloudflare 解析器
  dig @1.1.1.1 +short console.henukit.cn A
  # DoH（Cloudflare JSON API）
  curl -s 'https://cloudflare-dns.com/dns-query?name=console.henukit.cn&type=A' \
    -H 'accept: application/dns-json'
  ```

  预期：三条路径均返回 `8.146.200.82`。

- [ ] **Cloudflare 灰云状态**：在 Cloudflare 面板核对 `console.henukit.cn` 的 Proxy status 为 **DNS only（灰云）**。按项目文档仅 apex/www 开启 Cloudflare 代理；灰云状态下流量直达源服务器，ACME 挑战与 TLS 均由源站 Nginx 直接处理，无需考虑 Cloudflare SSL/TLS 模式。

- [ ] **服务器 SSH 可达**：

  ```bash
  ssh -p 22222 root@8.146.200.82 'nginx -v && ls /etc/nginx/sites-enabled/'
  ```

  预期：Nginx 1.24.x，`sites-enabled` 含 `henukit.cn` 与 `superhuazai.me`（legacy vhost 必须仍在，回滚依赖它）。

- [ ] **现状记录（已知问题复现）**：确认 `console.henukit.cn:443` 当前返回旧证书，作为切换前基线：

  ```bash
  openssl s_client -connect console.henukit.cn:443 -servername console.henukit.cn \
    </dev/null 2>/dev/null | openssl x509 -noout -subject -ext subjectAltName
  ```

  预期（切换前）：subject 为 `superhuazai.me`（SAN 含 `superhuazai.me, deploy.superhuazai.me`），即请求落入默认 server。

- [ ] **源服务器 80/443 公网可达**：apex 的 HTTP 308 与 HTTPS 均正常，默认视为已放行；签发前可再探活：

  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' http://console.henukit.cn/.well-known/acme-challenge/probe
  ```

  预期：404 或 308（证明 80 端口可达即可；此时尚无 challenge 文件）。

- [ ] **旧回调保留**：核对 `oauth_clients` 中 `console-gateway` 当前 `redirect_uris` 仍含 legacy 回调（`superhuazai.me/console-api/*`、`henukit.cn/console-api/*`）。查询方法见第 6 节；切换前的数组值需完整备份，回滚恢复用。

- [ ] **环境文件备份**：切流前对 `/opt/henukit/.env.henukit` 做 root-only、带校验和的备份：

  ```bash
  sudo cp -a /opt/henukit/.env.henukit /opt/henukit-backups/env.henukit.console-cutover.20260803
  sudo chown root:root /opt/henukit-backups/env.henukit.console-cutover.20260803
  sha256sum /opt/henukit-backups/env.henukit.console-cutover.20260803
  ```

---

## 2. 证书签发（HTTP-01 webroot）

**证书路径取舍**：现有 `henukit.cn` 证书是一张多域证书（SAN 覆盖 `henukit.cn` + `www.henukit.cn`），而 `henukit-host.conf.example` 的 console 443 vhost 引用的是**独立证书路径** `/etc/letsencrypt/live/console.henukit.cn/`。两种路径：

- **路径 A（推荐，与 example 一致）**：为 `console.henukit.cn` 单独签发一张证书。优点：不动现有证书，续期与回滚互不影响，回滚时直接停用该证书即可；缺点：多一张证书需要纳入现有续期流程。
- **路径 B**：把 `console.henukit.cn` 并入现有 `henukit.cn` 证书（`--expand` 扩展 SAN）。优点：证书数量不变；缺点：修改现有生产证书、续期命令需同步带上新域名、console vhost 的证书路径需改为引用 `henukit.cn` 路径，与 example 不符，回滚需再次收缩 SAN，风险更高。

**签发方式**：HTTP-01 + webroot（`henukit-host.conf.example` 中 console 80 端口块已有 `location ^~ /.well-known/acme-challenge/ { root /var/www/certbot; }`，webroot 为 `/var/www/certbot`）。**仓库未记录服务器现有的 certbot 具体签发命令与续期机制（cron/timer 或手动脚本），以下为参考命令，实际执行前需按服务器现有证书流程确认**：

```bash
# 参考命令（需确认与服务器现有签发方式一致）
certbot certonly --webroot -w /var/www/certbot -d console.henukit.cn
# 非交互方式
certbot certonly --webroot -w /var/www/certbot -d console.henukit.cn --non-interactive
```

前置依赖：第 3a 步（console 80 端口 vhost 含 acme-challenge location）必须先落地并 reload，否则 ACME 挑战会落入默认 server。灰云状态下挑战直达源站 80 端口，无需在 Cloudflare 侧做任何操作。

**验证**：

- [ ] 签发成功，证书文件存在：

  ```bash
  sudo ls -l /etc/letsencrypt/live/console.henukit.cn/
  ```

  预期：`fullchain.pem`、`privkey.pem`、`chain.pem`、`cert.pem` 存在。

- [ ] SAN 正确（对源站 IP 显式 SNI 验证，绕开解析缓存）：

  ```bash
  openssl s_client -connect 8.146.200.82:443 -servername console.henukit.cn \
    </dev/null 2>/dev/null | openssl x509 -noout -subject -ext subjectAltName
  ```

  预期：subject 含 `console.henukit.cn`，SAN 含 `DNS:console.henukit.cn`（此时 443 尚未服务该证书属正常，此步仅核对证书文件）。

- [ ] **续期纳入现有流程**：确认服务器现有的续期机制（certbot timer/`renew` 或既有脚本）覆盖新证书，运行续期演练：`certbot renew --dry-run`（需按服务器现有流程确认命令形式）。若走路径 B，则需把 `console.henukit.cn` 加入现有证书的续期命令。

---

## 3. Nginx vhost 部署

配置来源：`infra/nginx/henukit-host.conf.example`（console 80/443 server 块）。落地到生产 vhost（建议并入 `/etc/nginx/sites-enabled/henukit.cn`，或按现有 sites-enabled 组织方式单独建文件——**需与服务器现有 vhost 文件组织方式一致**）。

### 3a. 先落地 80 端口块（签发证书的前置）

先只加入 console 80 端口 server 块（acme-challenge + 308），此时**不要**加入 443 块——`ssl_certificate` 文件尚不存在，`nginx -t` 会失败。

```bash
# 编辑生产 vhost，加入 example 中的 console 80 块：
#   server {
#       listen 80;
#       listen [::]:80;
#       server_name console.henukit.cn;
#       location ^~ /.well-known/acme-challenge/ {
#           root /var/www/certbot;
#           default_type text/plain;
#       }
#       location / { return 308 https://console.henukit.cn$request_uri; }
#   }
sudo nginx -t
sudo systemctl reload nginx
```

验证：

- [ ] `http://console.henukit.cn/` 返回 308：

  ```bash
  curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' http://console.henukit.cn/
  ```

  预期：`308 https://console.henukit.cn/`。

### 3b. 证书签发后落地完整 vhost（80 + 443）

加入 443 块（内容照抄 example：`ssl_certificate /etc/letsencrypt/live/console.henukit.cn/fullchain.pem`、`ssl_certificate_key`、`include /etc/letsencrypt/options-ssl-nginx.conf`、`ssl_dhparam`、`client_max_body_size 32m`、HSTS 等安全头、`location /` 反代 `http://127.0.0.1:8088`）。

```bash
sudo nginx -t
sudo systemctl reload nginx
```

验证（SNI 证书正确）：

- [ ] 显式 SNI 返回 console 证书：

  ```bash
  openssl s_client -connect 8.146.200.82:443 -servername console.henukit.cn \
    </dev/null 2>/dev/null | openssl x509 -noout -subject -ext subjectAltName
  ```

  预期：subject / SAN 含 `console.henukit.cn`（不再是 `superhuazai.me`）。

- [ ] HTTPS 请求到达 compose edge（此时 edge 内尚无 console vhost，会走默认 server；本步只验证 TLS 与连通）：

  ```bash
  curl -sk -o /dev/null -w '%{http_code}\n' https://console.henukit.cn/
  ```

  预期：非连接错误（TLS 已握手成功）。compose edge 更新见第 4 节。

> 注意：console vhost 启用 HSTS（`max-age=31536000`），浏览器首次访问后会强制 HTTPS；回滚时仅删除 DNS 记录即可隔离，无需撤销 HSTS（该子域无 preload）。

---

## 4. Compose edge 更新

配置来源：`infra/nginx/henukit.conf.example`，已包含本次所需全部变更：

1. 主站 `location /console/` → `return 302 https://console.henukit.cn/;`（**302 而非 308**：浏览器不缓存 302，观察窗口期回滚保持干净；观察期结束后再转 308 或移除该 location）。
2. legacy `location /console-api/`（rewrite 到 console-gateway）——**观察窗口期间保留**，与 `oauth_clients` 中 `henukit.cn/console-api/*` 回调配套。
3. 新增 `console.henukit.cn` server 块：`/` → `console:80`（Console UI），`/api/` → `console-gateway:8082`（同源 API 直达 Console Gateway）。

部署：把最新 `henukit.conf.example` 同步到活动 release 目录的挂载路径（`docker-compose.henukit.yml` 中 nginx 服务以只读 volume 挂载 `./infra/nginx/henukit.conf.example:/etc/nginx/conf.d/default.conf:ro`），然后重建 nginx 容器（配置经 volume 挂载，`restart` 不会重新读取新文件；重建后 nginx 才加载新配置）：

```bash
# 同步配置文件到活动 release 目录后
docker compose --env-file /opt/henukit/.env.henukit \
  -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
  up -d --force-recreate nginx
```

> 若 release 目录中的 compose 文件与仓库 `docker-compose.henukit.yml` 挂载路径不同，以服务器实际挂载路径为准（需按现有发布流程确认）。

验证：

- [ ] 主站 `/console/` 302 跳转：

  ```bash
  curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' -I https://henukit.cn/console/
  ```

  预期：`302 https://console.henukit.cn/`。

- [ ] 子域 UI 与 API 路由已由 edge 接管（服务重建后做完整验收，见第 7 节）：

  ```bash
  curl -sk -o /dev/null -w '%{http_code}\n' https://console.henukit.cn/
  curl -sk -o /dev/null -w '%{http_code}\n' https://console.henukit.cn/api/v1/session
  ```

  预期：UI 返回 200（Console 容器）；`/api/v1/session` 未认证返回 401（console-gateway 行为，而非 Portal Gateway 的响应）。

---

## 5. 服务重建（--force-recreate）

**为什么 `restart` 不够**：容器环境变量在容器**创建**时从 `--env-file` 注入并固化；`docker compose restart` 只停止/启动同一容器实例，不会重新读取环境文件，修改后的变量不生效（见 `henukit-domain-cutover.md`：*A plain container restart does not update environment variables*）。因此环境变量变更必须 `--force-recreate`。

**必须设置的环境变量（生产 `.env.henukit`）**——本次切换新增/必须覆盖的值：

| 变量 | 值 | 消费方（来自 compose） |
|---|---|---|
| `CONSOLE_REDIRECT_URI` | `https://console.henukit.cn/api/v1/auth/callback` | `console-gateway`（compose 默认 `http://localhost:8088/console-api/v1/auth/callback`，必须被覆盖） |
| `CONSOLE_ORIGIN` | `https://console.henukit.cn` | 生产环境要求（Application origins 清单），供 Console OAuth Smoke / 验证脚本消费 |

**不得改动的相关已有值**（前次 cutover 已设置，recreate 会一并刷新，核对即可）：

- `PLATFORM_ACCOUNT_ORIGIN=https://henukit.cn/account-auth`（`console-gateway` 浏览器面向 Account Center 地址）
- `PUBLIC_ORIGIN` / `PORTAL_ORIGIN` / `PORTAL_REDIRECT_URI` 等其余 Application origins 值保持不变

**Console 镜像构建参数**：`apps/console/Dockerfile` 的 `ARG VITE_BASE_PATH=/`（compose 中 `VITE_CONSOLE_BASE_PATH` 默认 `/`）。该值在**镜像构建时**烘焙进镜像（Vite `base`），生产服务器不执行 `docker build`，因此部署侧只需确认发布 artifact 的 Console 镜像以 `VITE_BASE_PATH=/` 构建（compose 默认即 `/`，CI 构建不要覆盖），且不要在 `.env.henukit` 中设置 `VITE_CONSOLE_BASE_PATH`。该镜像只服务 `/`，不再服务 `henukit.cn/console`。

**执行重建**（仅重建消费了变更环境变量的服务，不触碰其他服务）：

```bash
docker compose --env-file /opt/henukit/.env.henukit \
  -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
  up -d --force-recreate console console-gateway
```

验证：

- [ ] 容器重建且健康：

  ```bash
  docker compose --env-file /opt/henukit/.env.henukit \
    -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
    ps
  ```

  预期：`console`、`console-gateway` 状态为 running（Up）。

- [ ] 环境变量已生效（容器内核对，不打印敏感值）：

  ```bash
  docker compose --env-file /opt/henukit/.env.henukit \
    -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
    exec console-gateway sh -c 'echo "REDIRECT_URI=$CONSOLE_REDIRECT_URI"'
  ```

  预期：`REDIRECT_URI=https://console.henukit.cn/api/v1/auth/callback`。

- [ ] console-gateway 诊断端口（host loopback 暴露）可直连：

  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8082/api/v1/session
  ```

  预期：401（未认证，服务存活）。

---

## 6. OAuth callback 注册

`CONSOLE_REDIRECT_URI` 环境变量本身不足够：Platform Core 对 `oauth_clients.redirect_uris` 做**精确匹配**。在重建网关前必须完成注册。

**步骤**：

1. 备份 `console-gateway`（与 `portal-gateway`）行，确认当前仅含预期 legacy 回调。
2. 单事务内添加 `console.henukit.cn` 回调，**保留 legacy 回调**（`superhuazai.me/console-api/*` 与 `henukit.cn/console-api/*`，观察窗口期回滚依赖）。
3. 提交后查询验证，运行 Console OAuth Smoke。
4. 观察窗口结束后，在**单独、经审计的变更**中移除已退役域名的回调（见第 7 节）。

**事务 SQL**（原文来自 `henukit-domain-cutover.md`；通过生产 PostgreSQL 容器执行，凭据来自环境文件且不打印）：

```sql
BEGIN;
SELECT id, redirect_uris
FROM oauth_clients
WHERE id IN ('portal-gateway', 'console-gateway')
FOR UPDATE;

UPDATE oauth_clients
SET redirect_uris = ARRAY[
  'https://superhuazai.me/api/v1/auth/callback',
  'https://henukit.cn/api/v1/auth/callback'
]
WHERE id = 'portal-gateway';

UPDATE oauth_clients
SET redirect_uris = ARRAY[
  'https://superhuazai.me/console-api/v1/auth/callback',
  'https://henukit.cn/console-api/v1/auth/callback',
  'https://console.henukit.cn/api/v1/auth/callback'
]
WHERE id = 'console-gateway';
COMMIT;
```

> 若查询发现 `console.henukit.cn` 回调在先前 cutover 中已注册，则跳过 UPDATE 事务，直接做查询验证与 Smoke。

**验证**：

- [ ] 行内容与预期一致：

  ```sql
  SELECT id, redirect_uris
  FROM oauth_clients
  WHERE id IN ('portal-gateway', 'console-gateway');
  ```

  预期：`console-gateway` 的 `redirect_uris` 恰为上述三条数组。

- [ ] **Console OAuth Smoke**：手动流程或复用验证脚本（`products/quizcraft/go-service/scripts/verify-cutover.sh` 的 `CONSOLE_ORIGIN` 验证路径）：
  - 访问 `https://console.henukit.cn/` 触发登录 → 重定向到 Account Center（`https://henukit.cn/account-auth`）→ 登录后回调回到 `https://console.henukit.cn/api/v1/auth/callback`（精确匹配）→ 进入 Console。
  - `GET https://console.henukit.cn/api/v1/session` 返回 200 且 `expires_at` 约为 8 小时（Console Session TTL）。
  - 登出后 `GET /api/v1/session` 返回 401。
  - Smoke 失败时：先事务恢复备份的 `redirect_uris` 数组，再恢复网关环境变量（见第 8 节）。

---

## 7. 验收清单（Go/No-Go）

全部通过才可判定 **GO**；任一失败按第 8 节回滚。

- [ ] **子域 HTTPS 可达**：

  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' -I https://console.henukit.cn/
  ```

  预期：`200`，且证书为 `console.henukit.cn`（`curl -vI` 或 openssl 复核 SAN）。

- [ ] **登录流程**：真实 `@henu.edu.cn` 邮箱走完登录 → OAuth 回调回到 `console.henukit.cn/api/v1/auth/callback` → Console 正常使用；`/api/v1/session` 登录后 200、登出后 401（同第 6 节 Smoke，记录浏览器/脚本证据）。

- [ ] **`/api/` 转发到 console-gateway**（而非 Portal Gateway）：

  ```bash
  # 未认证时应是 console-gateway 的 401 行为
  curl -s -o /dev/null -w '%{http_code}\n' https://console.henukit.cn/api/v1/session
  # 请求确实命中 console-gateway 容器（排除被 Portal Gateway 响应误导）
  docker compose --env-file /opt/henukit/.env.henukit \
    -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml \
    logs --tail=20 console-gateway
  ```

  预期：公网返回 `401`（未认证）；`console-gateway` 日志出现对应请求。

- [ ] **Console UI 资源路径**：页面 HTML 与静态资源均以 `/` 为 base（`VITE_BASE_PATH=/` 构建产物），无 `/console/` 前缀资源请求。

- [ ] **主站回归不受影响**：`https://henukit.cn/` 200；`/console/` 302 → `https://console.henukit.cn/`；`/console-api/` 仍工作（观察窗口期 legacy 回调配套）；Portal 登录正常。

- [ ] **观察窗口期清理（窗口结束后、单独审计变更中执行，不在本次 Go/No-Go 内）**：
  - [ ] 从 `oauth_clients` 移除 `console-gateway` 的 `superhuazai.me/console-api/*` 与 `henukit.cn/console-api/*` 回调（事务内执行，保留 `console.henukit.cn/api/v1/auth/callback`）。
  - [ ] 从 compose edge 移除 `/console/` 302 location（或转 308）与 `/console-api/` location。
  - [ ] 观察窗口结束前：**不删除**新证书、旧 vhost、旧 DNS 记录。

---

## 8. 回滚

参考 `henukit-domain-cutover.md` Rollback 章节；任一验收项失败即触发。观察窗口内保留新证书、旧 vhost、旧 DNS 记录与旧回调。

- [ ] **移除 DNS**：删除 Cloudflare 中 `console.henukit.cn` A 记录（当前灰云、无代理可禁用；若后续对子域开启了 Cloudflare 代理，则先禁用代理再删记录）。`dig console.henukit.cn A` 应不再返回 `8.146.200.82`。
- [ ] **保留/恢复 legacy 入口**：`superhuazai.me` DNS 与 vhost 保持启用（本次全程不动）。
- [ ] **恢复 OAuth 回调**：事务内将 `oauth_clients.redirect_uris` 恢复为切换前备份的数组（`console-gateway` 移除 `console.henukit.cn/api/v1/auth/callback`，恢复 `superhuazai.me/console-api/*` + `henukit.cn/console-api/*`）。

  ```sql
  BEGIN;
  UPDATE oauth_clients
  SET redirect_uris = ARRAY[
    'https://superhuazai.me/console-api/v1/auth/callback',
    'https://henukit.cn/console-api/v1/auth/callback'
  ]
  WHERE id = 'console-gateway';
  COMMIT;
  ```

  > 以第 1 节备份的实际数组为准；若切换前数组与上式不同，恢复备份值。
- [ ] **恢复环境备份**：以第 1 节备份文件恢复 `/opt/henukit/.env.henukit`（root-owned、校验 sha256 一致）。
- [ ] **重建回旧状态**：移除 console 80/443 vhost 块并 `nginx -t && systemctl reload nginx`；恢复 compose edge 旧配置并 `--force-recreate nginx`；以旧固定 SHA 镜像 `--force-recreate console console-gateway`。
- [ ] **验证**：legacy HTTPS（`https://henukit.cn/` 200、`superhuazai.me` 证书正常）、Portal/Console 旧回调 OAuth 登录、API readiness 均恢复；`console.henukit.cn:443` 不再被本清单负责（DNS 已删）。

---

## 9. 审批记录

| 检查项 | 负责人 | 证据位置 | 结论/时间 |
|---|---|---|---|
| 前置条件核对（第 1 节） |  |  |  |
| 证书签发与 SAN 验证（第 2 节） |  |  |  |
| Nginx vhost 落地与 SNI 验证（第 3 节） |  |  |  |
| Compose edge 更新与 302 验证（第 4 节） |  |  |  |
| 服务重建与环境变量核对（第 5 节） |  |  |  |
| OAuth 注册与 Smoke（第 6 节） |  |  |  |
| 验收清单（第 7 节） |  |  |  |
| 最终结论 |  |  | GO / NO-GO |

# 生产服务器核验 Runbook（PRODUCTION VERIFY RUNBOOK）

> 2026-08-19 更新（服务器实测）：**本 Runbook 已执行一轮**，结果见 `CURRENT_PRODUCTION_STATE.md`（2026-08-19 服务器实测版）。执行后预期已更新：quizcraft **Go 契约表在宿主机 postgres 的 `quizcraft_v2` 库**（非 docker postgres）；`/api/v1/practice/banks` 预期 **200-空列表**（portal-api 直读幽灵路径）；`/study-api/healthz` 预期 **404**；study/quiz/account 子域预期**默认证书**（无专用 vhost）。命令主体未改。
> 2026-08-20 更新（方案 2 容器化，仓库配置层）：Go core 纳入主栈 compose 服务 `quizcraft`（镜像 `henukit-quizcraft`，替换 systemd）；宿主机 10089 不再监听（容器内 expose）；§9.3 相应更新。

> 目标：把 `docs/operations/CURRENT_PRODUCTION_STATE.md` §6「待服务器核验清单」与 `docs/migrations/ALL_NEW_STACK_CUTOVER.md` M0 的服务器核验项，变成一份**可直接复制粘贴逐条执行**的清单。
> 适用服务器：阿里云 ECS cn-beijing `8.146.200.82`（Ubuntu + Nginx），SSH `root@8.146.200.82:22222`（KEX 干扰时加 `-o KexAlgorithms=diffie-hellman-group14-sha256`）。
> **预计耗时：40 分钟**。**需要：root 权限 + PostgreSQL 只读访问**（默认用 compose 内 `henukit` 超级用户，但只允许执行只读事务，见 §0 安全规则）。
> 执行方式：登录服务器后按 §0 初始化，随后逐节复制命令执行；每节输出追加到 `/root/henukit-verify-$(date +%Y%m%d).log`，最后按 §10 汇总「通过/失败/待人工」。

---

## §0 准备与安全规则（约 3 分钟）

**安全规则（违反任何一条即判定该节失败）：**

1. **绝不把任何密钥值写入日志或输出**：`.env.henukit` 里的密码/token/secret（含 `POSTGRES_PASSWORD`、`*_CLIENT_SECRET`、`*_TOKEN`、`*_KEY`、`PLATFORM_CORE_SMTP_PASSWORD`、`WECHAT_PAY_*` 等）只报告「键是否存在 / 是否非空」，**永远不要 `cat` 该文件**。
2. **psql 一律用只读事务**：本 Runbook 所有数据库查询都包在 `BEGIN READ ONLY; ... COMMIT;` 内，且只做 `SELECT` / `to_regclass` / `information_schema` 查询，不写任何数据。
3. 本 Runbook **不删除、不修改、不重启**任何生产对象（除显式标注的只读命令外）。
4. 域名、bucket 名、redirect_uris、行数、状态码等**非密钥信息可以记录**。
5. 每节末尾按模板记录「证据」并打 `[PASS]` / `[FAIL]` / `[MANUAL]` 标记，供 §10 汇总。

**初始化（一次性）：**

```bash
# 登录：ssh -p 22222 root@8.146.200.82
set -o pipefail

# —— 定位活动 release 与 env 文件（服务器上可能有多处，以 watcher 配置为准）——
SHA=$(cat /var/lib/henukit-actions-watch/last-activated-sha 2>/dev/null)
RELEASE_DIR="/opt/henukit-releases/${SHA}"
ENV_FILE=$(grep -oP '^HENUKIT_ENV_FILE=\K.*' /etc/henukit/actions-watch.env 2>/dev/null | head -1)
ENV_FILE=${ENV_FILE:-/opt/henukit/.env.henukit}
# 兼容历史路径：/opt/henukit-releases/<sha>/.env.henukit 与 /opt/henukit/.env.henukit
[ -f "$ENV_FILE" ] || ENV_FILE=$(ls /opt/henukit-releases/*/.env.henukit 2>/dev/null | head -1)
[ -f "$ENV_FILE" ] || { echo "[FAIL] 找不到 env 文件，中止"; exit 1; }

LOG="/root/henukit-verify-$(date +%Y%m%d-%H%M).log"
echo "======== HENU Kit 生产核验 $(date -u +%Y-%m-%dT%H:%M:%SZ) ========" | tee "$LOG"
echo "SHA=$SHA | RELEASE_DIR=$RELEASE_DIR | ENV_FILE=$ENV_FILE | LOG=$LOG" | tee -a "$LOG"
ls -l "$ENV_FILE" 2>/dev/null | tee -a "$LOG"   # 期望 root root 0600
```

**psql 只读查询辅助函数（本 Runbook 后续所有 `DBQ` 调用都走它）：**

```bash
DBQ() { # 用法: DBQ <库名> "<SQL>"   —— 强制 BEGIN READ ONLY
  docker exec -i henukit-postgres-1 psql -X -U henukit -v ON_ERROR_STOP=1 -t -A \
    -d "$1" -c "BEGIN READ ONLY; $2 COMMIT;"
}
# 备选：若服务器存在独立只读账户（如 henukit_ro），把上面 -U henukit 换成该账户并同样只跑只读事务
```

---

## §1 基础盘点（约 5 分钟）

**目的**：确认生产实际运行的服务清单、镜像 SHA、容器健康、磁盘/内存水位——这是 §6「待服务器核验清单」第一项，也是回填 `CURRENT_PRODUCTION_STATE.md` §1/§4 的依据。

```bash
echo "== 1.1 全部容器（含已停止）==" | tee -a "$LOG"
docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}' | tee -a "$LOG"

echo "== 1.2 活动 release 的 compose 服务状态 ==" | tee -a "$LOG"
docker compose --env-file "$ENV_FILE" -f "$RELEASE_DIR/docker-compose.henukit.release.yml" ps -a 2>&1 | tee -a "$LOG"

echo "== 1.3 运行中容器数量 ==" | tee -a "$LOG"
UP_COUNT=$(docker ps --format '{{.Names}}' | wc -l)
echo "running containers: $UP_COUNT" | tee -a "$LOG"
[ "$UP_COUNT" -eq 20 ] && echo "[PASS] 运行容器数=20（17 自建 + postgres/redis/nginx）" | tee -a "$LOG" \
  || echo "[MANUAL] 运行容器数=$UP_COUNT（期望 20；conditional 服务缺席属合法，逐项对照 compose）" | tee -a "$LOG"

echo "== 1.4 镜像 SHA 一致性（期望全部 henukit-* 镜像 tag = last-activated-sha）==" | tee -a "$LOG"
docker ps --format '{{.Image}}' | grep -oE '[0-9a-f]{40}' | sort -u | tee -a "$LOG"
echo "last-activated-sha: $SHA" | tee -a "$LOG"
docker ps --format '{{.Image}}' | grep -oE 'henukit-[a-z-]+' | sort -u | tee -a "$LOG"
echo "== 1.5 容器健康状态 ==" | tee -a "$LOG"
docker ps --format '{{.Names}} {{.Status}}' | tee -a "$LOG"
docker inspect --format '{{.Name}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' \
  $(docker ps -q) 2>/dev/null | tee -a "$LOG"

echo "== 1.6 磁盘 / 内存 ==" | tee -a "$LOG"
df -h / | tail -1 | tee -a "$LOG"
free -m | tee -a "$LOG"
docker system df 2>/dev/null | tee -a "$LOG"
```

**通过判据**：
- §1.4：所有 `henukit-*` 运行镜像的 40 位 SHA 相同，且等于 `last-activated-sha`（postgres/redis/nginx 为固定 tag，不参与）。
- §1.5：`docker ps` 全部 `Up`；带 healthcheck 的容器（postgres/redis/account-portfolio/notice/food/career-opportunities/library/portal-summary）显示 `healthy`。
- §1.6：`df -h /` 可用空间 ≥ 1.5G（部署冗余），内存无持续 swap 抖动。
- 17 个自建镜像名应与 `scripts/ops/henukit-release-images.sh` 的 `release_names` 一致（console、console-gateway、platform-core、platform-mail-worker、platform-smtp-provider、portal、portal-summary、portal-api、account-portfolio、notice、notice-worker、food、food-mcp、library、career-opportunities、career-mcp、portal-gateway）。

**证据记录**：把 1.1–1.6 原始输出复制进本文件同目录的核验回执（或直接引用 `$LOG`），并在回执写：实际容器数 / 实际 SHA / 是否有 unhealthy 容器 / 磁盘余量。

---

## §2 env 核验（约 6 分钟）

**目的**：核对 `/opt/henukit/.env.henukit` 的**键存在性矩阵**（只输出键名 true/false，禁止打印值），并与 `docker-compose.henukit.prebuilt.yml` / 活动 release.yml 的 `:?` 强制变量契约逐项比对。回填 M0「服务器 env 与 repo 严重不一致」疑点（`ALL_NEW_STACK_CUTOVER.md` M4 §8）。

```bash
echo "== 2.1 env 文件属性（期望 root:root 0600）==" | tee -a "$LOG"
stat -c '%U:%G %a %n' "$ENV_FILE" | tee -a "$LOG"
[ "$(stat -c '%U:%G %a' "$ENV_FILE")" = "root:root 600" ] && echo "[PASS] env 权限 0600 root-owned" | tee -a "$LOG" \
  || echo "[FAIL] env 权限不正确: $(stat -c '%U:%G %a' "$ENV_FILE")" | tee -a "$LOG"

echo "== 2.2 关键变量存在性矩阵（只输出键名 true/false/空值，绝不打印值）==" | tee -a "$LOG"
KEYS="ACCOUNT_PORTFOLIO_DATABASE_URL ACCOUNT_PORTFOLIO_CLIENT_ID ACCOUNT_PORTFOLIO_KEY_ID ACCOUNT_PORTFOLIO_CLIENT_SECRET ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID ACCOUNT_PORTFOLIO_CONSOLE_SECRET ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY LIBRARY_OSS_BUCKET LIBRARY_OSS_REGION LIBRARY_OSS_INTERNAL_ENDPOINT LIBRARY_OSS_PUBLIC_ENDPOINT LIBRARY_OSS_ECS_RAM_ROLE LIBRARY_DOWNLOAD_CLIENT_ID LIBRARY_DOWNLOAD_KEY_ID LIBRARY_DOWNLOAD_CLIENT_SECRET FOOD_MCP_ACCESS_TOKEN NOTICE_DELIVERY_URL NOTICE_DELIVERY_TOKEN QUIZCRAFT_CORE_URL PORTAL_ENABLE_QUIZCRAFT_CATALOG PORTAL_ENABLE_QUIZCRAFT_V2_READS CAREER_DATABASE_URL CAREER_CLIENT_SECRET PORTAL_API_MODE"
for k in $KEYS; do
  if grep -q "^${k}=" "$ENV_FILE"; then
    if grep -qE "^${k}=[^[:space:]]+" "$ENV_FILE"; then echo "$k=true(非空)"; else echo "$k=true(空值)"; fi
  else echo "$k=false"; fi
done | tee -a "$LOG"

echo "== 2.3 env 全部键名清单（仅键名，安全）==" | tee -a "$LOG"
grep -oE '^[A-Z_0-9]+=' "$ENV_FILE" | sed 's/=$//' | sort | tee -a "$LOG"

echo "== 2.4 compose 契约强制变量比对 ==" | tee -a "$LOG"
# 活动 release.yml 是服务器上权威契约
if [ -f "$RELEASE_DIR/docker-compose.henukit.release.yml" ]; then
  grep -oE '\$\{[A-Z0-9_]+:\?' "$RELEASE_DIR/docker-compose.henukit.release.yml" 2>/dev/null \
    | sed -E 's/\$\{([A-Z0-9_]+):\?/\1/' | sort -u > /tmp/release-required.txt
  echo "-- release.yml 强制变量缺失项（无输出=齐备）--" | tee -a "$LOG"
  while read -r v; do grep -q "^${v}=" "$ENV_FILE" || echo "MISSING(release契约): $v"; done < /tmp/release-required.txt | tee -a "$LOG"
fi
# 仓库 prebuilt 契约（服务器一般没有此文件；可 scp 一份到 /tmp，或对照下方静态清单人工核验）
if [ -f /tmp/docker-compose.henukit.prebuilt.yml ]; then
  grep -oE '\$\{[A-Z0-9_]+:\?' /tmp/docker-compose.henukit.prebuilt.yml \
    | sed -E 's/\$\{([A-Z0-9_]+):\?/\1/' | sort -u > /tmp/prebuilt-required.txt
  echo "-- prebuilt 契约强制变量缺失项（无输出=齐备）--" | tee -a "$LOG"
  while read -r v; do grep -q "^${v}=" "$ENV_FILE" || echo "MISSING(prebuilt契约): $v"; done < /tmp/prebuilt-required.txt | tee -a "$LOG"
fi

echo "== 2.5 compose 插值通过性 ==" | tee -a "$LOG"
docker compose --env-file "$ENV_FILE" -f "$RELEASE_DIR/docker-compose.henukit.release.yml" config --quiet 2>&1 | tee -a "$LOG"
[ "${PIPESTATUS[0]}" -eq 0 ] && echo "[PASS] compose config 插值通过（EXIT 0）" | tee -a "$LOG" \
  || echo "[FAIL] compose config 插值失败（EXIT ${PIPESTATUS[0]}）" | tee -a "$LOG"
```

**prebuilt 契约 `:?` 强制变量静态清单**（来自仓库 `docker-compose.henukit.prebuilt.yml`，共 83 个去重键；若服务器有该文件则直接跑 §2.4 脚本，否则人工对照本清单与 §2.3 键名清单）：

`ACCOUNT_PORTFOLIO_CLIENT_ID, ACCOUNT_PORTFOLIO_CLIENT_SECRET, ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID, ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID, ACCOUNT_PORTFOLIO_CONSOLE_SECRET, ACCOUNT_PORTFOLIO_DATABASE_URL, ACCOUNT_PORTFOLIO_KEY_ID, ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY, CAREER_AI_API_KEY, CAREER_AI_BASE_URL, CAREER_AI_MODEL, CAREER_CLIENT_SECRET, CAREER_DATABASE_URL, CAREER_SOURCE_ALLOWLIST, CONSOLE_PLATFORM_CLIENT_SECRET, CONSOLE_SESSION_KEY, FOOD_DATABASE_URL, FOOD_REDIS_URL, FOOD_SUMMARY_CLIENT_ID, FOOD_SUMMARY_CLIENT_SECRET, FOOD_SUMMARY_KEY_ID, LIBRARY_DATABASE_URL, LIBRARY_DOWNLOAD_CLIENT_ID, LIBRARY_DOWNLOAD_CLIENT_SECRET, LIBRARY_DOWNLOAD_KEY_ID, LIBRARY_DOWNLOAD_URL, LIBRARY_OSS_BUCKET, LIBRARY_OSS_ECS_RAM_ROLE, LIBRARY_OSS_INTERNAL_ENDPOINT, LIBRARY_OSS_PUBLIC_ENDPOINT, LIBRARY_OSS_REGION, LIBRARY_REDIS_URL, LIBRARY_SUMMARY_CLIENT_ID, LIBRARY_SUMMARY_CLIENT_SECRET, LIBRARY_SUMMARY_KEY_ID, NOTICE_CLIENT_SECRET, NOTICE_DATABASE_URL, NOTICE_REDIS_URL, NOTICE_SUMMARY_CLIENT_ID, NOTICE_SUMMARY_CLIENT_SECRET, NOTICE_SUMMARY_KEY_ID, PLATFORM_CLIENT_SECRET, PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID, PLATFORM_CORE_CAREER_DIGEST_KEY_ID, PLATFORM_CORE_CAREER_DIGEST_SECRET, PLATFORM_CORE_DATABASE_URL, PLATFORM_CORE_IDEMPOTENCY_KEY, PLATFORM_CORE_MAIL_DELIVERY_TOKEN, PLATFORM_CORE_MAIL_PROVIDER_TOKEN, PLATFORM_CORE_REDIS_URL, PLATFORM_CORE_SMTP_ADDRESS, PLATFORM_CORE_SMTP_FROM, PLATFORM_CORE_SMTP_PASSWORD, PLATFORM_CORE_SMTP_USERNAME, PLATFORM_CORE_VERIFICATION_KEY, PLATFORM_SUMMARY_CLIENT_SECRET, PORTAL_DEPLOYED_AT, PORTAL_SESSION_KEY, PORTAL_SUMMARY_CLIENT_SECRET, PORTAL_VERSION, POSTGRES_DB, POSTGRES_PASSWORD, POSTGRES_USER, PRACTICE_COMMAND_CLIENT_ID, PRACTICE_COMMAND_CLIENT_SECRET, PRACTICE_COMMAND_KEY_ID, QUIZCRAFT_AUTH_HMAC_SECRET, QUIZCRAFT_CORE_URL, QUIZCRAFT_CUTOVER_EVIDENCE_SECRET, QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID, QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET, QUIZCRAFT_PORTAL_CATALOG_KEY_ID, QUIZCRAFT_PORTAL_COMMANDS_ENABLED, QUIZCRAFT_PORTAL_COMMAND_CLIENT_ID, QUIZCRAFT_PORTAL_COMMAND_CLIENT_SECRET, QUIZCRAFT_PORTAL_COMMAND_KEY_ID, QUIZCRAFT_SUMMARY_CLIENT_ID, QUIZCRAFT_SUMMARY_CLIENT_SECRET, QUIZCRAFT_SUMMARY_KEY_ID, QUIZCRAFT_V2_DATABASE_URL, QUIZCRAFT_WRITES_ENABLED, RELEASE_SHA, STUDY_DATABASE_URL`

ADR-0037 已退役 Library legacy adapter；`STUDY_LEGACY_API_URL` 与 `STUDY_LEGACY_ADMIN_TOKEN` 不再是启动条件。若旧生产 env 仍保留这两项，只记录为可清理的历史配置，不得把它们重新加入 prebuilt 强制契约。

**通过判据 / 待人工判读**：
- `[PASS] compose config 插值通过` 且 release 契约无缺失项 → 当前活动 release 契约与 env 一致。
- **本节点最重要的产出是「结论」而不是「通过」**：记录 §2.2 矩阵里哪些键为 `false`/`空值`，并与 prebuilt 契约比对——仓库 `ALL_NEW_STACK_CUTOVER.md` M4 §8 已指出「prebuilt 强制 `QUIZCRAFT_CORE_URL`/`ACCOUNT_PORTFOLIO_*`/`LIBRARY_OSS_*` 而 repo env 未设」的矛盾，服务器上若同样缺失，说明**生产并不使用 prebuilt 契约生成 release.yml**（release 契约口径为准）；若连 release 契约也缺失，则该服务无法启动或未接线——逐项打 `[FAIL]` 或 `[MANUAL]`。

**证据记录**：§2.2 矩阵全文 + §2.4 缺失清单 + §2.5 EXIT 码；并写一句结论：生产 release 契约是否被满足、prebuilt 契约是否被生产采用。

---

## §3 域名 / 证书（约 5 分钟）

**目的**：核对 6 个域的证书与到期日、Nginx 配置有效性、vhost 文件清单。回填 `CURRENT_PRODUCTION_STATE.md` §1「henukit.cn apex / console 证书」与 M4 §3「域名/证书矩阵（study/quiz/account.henukit.cn 均未落地）」的现场事实。

```bash
echo "== 3.1 各域名证书（对源站 IP 显式 SNI，绕开解析缓存）==" | tee -a "$LOG"
for d in henukit.cn www.henukit.cn console.henukit.cn study.henukit.cn quiz.henukit.cn account.henukit.cn; do
  echo "--- $d ---" | tee -a "$LOG"
  openssl s_client -connect 8.146.200.82:443 -servername "$d" </dev/null 2>/dev/null \
    | openssl x509 -noout -subject -issuer -dates -ext subjectAltName 2>/dev/null \
    | tee -a "$LOG" || echo "  (TLS 握手失败或无证书)" | tee -a "$LOG"
done

echo "== 3.2 本地证书目录 ==" | tee -a "$LOG"
ls -l /etc/letsencrypt/live/ 2>/dev/null | tee -a "$LOG"
certbot certificates 2>/dev/null | tee -a "$LOG"   # 若 certbot 不在，跳过

echo "== 3.3 nginx 配置有效性 ==" | tee -a "$LOG"
nginx -t 2>&1 | tee -a "$LOG"

echo "== 3.4 vhost 文件清单 ==" | tee -a "$LOG"
ls -l /etc/nginx/sites-enabled/ | tee -a "$LOG"
grep -l "server_name" /etc/nginx/sites-enabled/* 2>/dev/null | while read -r f; do
  echo "-- $f --"; grep -E "server_name|listen" "$f" | head -10
done | tee -a "$LOG"
```

**通过判据**：
- `henukit.cn` / `www.henukit.cn`：证书 subject/SAN 含二者（多域证书），到期日 > 核验日（仓库记录原为 2026-10-28，console 为 2026-11-01；以现场为准，**到期 < 30 天打 `[FAIL]`**）。
- `console.henukit.cn`：独立证书，SAN 含 `console.henukit.cn`（不再回落到 `superhuazai.me` 默认 server）。
- `study.henukit.cn` / `quiz.henukit.cn` / `account.henukit.cn`：M4 §3 记录「均无 vhost 未落地」——预期表现为无证书/回落到默认 server。**记录实际**即可（若出现有效证书反而要标 `[MANUAL]` 复核来源）。
- `nginx -t` 输出 `syntax is ok` + `test is successful`。
- `sites-enabled` 同时含 `henukit.cn` 与 `superhuazai.me`（legacy vhost 保留 = 回滚路径存在）。

**证据记录**：每域 subject/SAN/notAfter 一行；`nginx -t` 输出；`sites-enabled` 清单。

---

## §4 边缘路由 smoke（约 6 分钟）

**目的**：对共享 edge（compose nginx `127.0.0.1:8088` → 主机 Nginx 443）跑路由状态码矩阵，区分「200/302/401/404」预期。融合 `henukit-artifact-deployment.md` 验收 smoke（superhuazai.me 口径）、`henukit-local-deploy.md` §四（henukit.cn 口径）、`watch-henukit-actions.sh` verify_active_release 断言与 `console-subdomain-deploy-checklist.md` §7 实测。

```bash
# 不跟跳转：报告首个响应码 + 跳转目标（用于 /console/ 的 302 断言）
probe_nofollow() { # $1=名称 $2=URL $3=期望
  local out code loc
  out=$(curl -sk -o /dev/null -w '%{http_code} %{redirect_url}' "$2")
  code=${out%% *}; loc=${out#* }
  if [ "$code" = "$3" ]; then echo "[PASS] $1 -> $code (期望$3)"; else echo "[FAIL] $1 -> $code (期望$3) loc=$loc"; fi
}
# 跟最多 3 跳：报告最终响应码（用于 /quiz/、/study-api/healthz 的退役 404 断言）
probe_follow() { # $1=名称 $2=URL $3=期望
  local code
  code=$(curl -sk -L --max-redirs 3 -o /dev/null -w '%{http_code}' "$2")
  if [ "$code" = "$3" ]; then echo "[PASS] $1 -> $code (期望$3)"; else echo "[FAIL] $1 -> $code (期望$3)"; fi
}

echo "== 4.1 主域 henukit.cn ==" | tee -a "$LOG"
probe_nofollow "首页 /"        https://henukit.cn/ 200 | tee -a "$LOG"
probe_nofollow "公共健康 /api/v1/healthz" https://henukit.cn/api/v1/healthz 200 | tee -a "$LOG"
probe_nofollow "Portal /practice"  https://henukit.cn/practice 200 | tee -a "$LOG"
probe_nofollow "Portal /library"   https://henukit.cn/library 200 | tee -a "$LOG"
probe_nofollow "未认证会话 /api/v1/session" https://henukit.cn/api/v1/session 401 | tee -a "$LOG"
probe_nofollow "资料库 API /api/v1/library/courses" https://henukit.cn/api/v1/library/courses 401 | tee -a "$LOG"
probe_nofollow "/console/ 观察期跳转" https://henukit.cn/console/ 302 | tee -a "$LOG"
probe_nofollow "legacy /console-api/v1/session" https://henukit.cn/console-api/v1/session 401 | tee -a "$LOG"
probe_nofollow "Account Center /account-auth/" https://henukit.cn/account-auth/ 200 | tee -a "$LOG"
probe_nofollow "资料 OSS 栅栏 /materials/" https://henukit.cn/materials/ 404 | tee -a "$LOG"
# 刷题公开 catalog：QUIZCRAFT_CORE_URL 未接线时默认 503/404；已接线见 §2.2 后人工判读
curl -sk -o /dev/null -w '[MANUAL] /api/v1/practice/banks -> %{http_code}\n' https://henukit.cn/api/v1/practice/banks | tee -a "$LOG"
# 退役探针（跟跳，最终必须 404）
probe_follow "退役探针 /quiz/          " https://henukit.cn/quiz/ 404 | tee -a "$LOG"
probe_follow "退役探针 /study-api/healthz" https://henukit.cn/study-api/healthz 404 | tee -a "$LOG"

echo "== 4.2 子域 console.henukit.cn ==" | tee -a "$LOG"
probe_nofollow "Console UI /"        https://console.henukit.cn/ 200 | tee -a "$LOG"
probe_nofollow "Console 未认证 /api/v1/session" https://console.henukit.cn/api/v1/session 401 | tee -a "$LOG"

echo "== 4.3 legacy 域名 superhuazai.me（artifact smoke 口径）==" | tee -a "$LOG"
probe_nofollow "legacy 首页 /"     https://superhuazai.me/ 200 | tee -a "$LOG"
probe_nofollow "legacy /practice"  https://superhuazai.me/practice 200 | tee -a "$LOG"
probe_nofollow "legacy /library"   https://superhuazai.me/library 200 | tee -a "$LOG"
probe_follow  "legacy 退役探针 /quiz/" https://superhuazai.me/quiz/ 404 | tee -a "$LOG"
probe_follow  "legacy 退役探针 /study-api/healthz" https://superhuazai.me/study-api/healthz 404 | tee -a "$LOG"

echo "== 4.4 account.superhuazai.me / account.henukit.cn ==" | tee -a "$LOG"
curl -sk -o /dev/null -w 'account.superhuazai.me -> %{http_code}\n' https://account.superhuazai.me/ | tee -a "$LOG"
curl -sk -o /dev/null -w 'account.henukit.cn     -> %{http_code}\n' https://account.henukit.cn/ | tee -a "$LOG"
```

**通过判据**：
- §4.1 全部 `[PASS]`；`/quiz/`、`/study-api/healthz` 最终 404（中间跳转不算通过）。
- `/console/` 为 **302**（观察窗口期，不能是 308/200）。
- `/account-auth/` 返回 200（platform-core 登录页；若 302 到登录页也判 `[MANUAL]` 复核）。
- §4.2 Console 子域：UI 200、未认证 API 401 且请求命中 `console-gateway`（可用 `docker logs --tail=10 henukit-console-gateway-1` 复核）。
- §4.3 legacy 口径与主域口径**同时**记录（M4 §6 要求统一验收口径，本 Runbook 两者都跑，结论由人拍板主口径）。
- §4.4：记录实际码。若 `account.*` 返回 200/302 说明 Platform Core 已部署到该域（需 §5 数据库佐证）；返回 404/连接失败则符合「未落地」。

**证据记录**：4.1–4.4 全部行；对非预期码补 `docker logs` 证据。

---

## §5 数据库核验（约 8 分钟）

**目的**：9 库存在性；study 库数据量；quizcraft 库 Go 契约表 vs FastAPI 表是否同库共存；platform 库迁移/接线表；account_portfolio 行数基线。全部只读（`DBQ` 已强制 `BEGIN READ ONLY`）。回填 `CURRENT_PRODUCTION_STATE.md` §2/§3 与 M0 清单。

```bash
echo "== 5.1 数据库存在性（期望 9 库：study platform account_portfolio quizcraft notice library food career portal）==" | tee -a "$LOG"
DBQ postgres "SELECT datname FROM pg_database WHERE datname NOT IN ('postgres','template0','template1') ORDER BY 1;" | tee -a "$LOG"
DBQ postgres "SELECT count(*) FROM pg_database WHERE datname IN ('study','platform','account_portfolio','quizcraft','notice','library','food','career','portal');" | tee -a "$LOG"

echo "== 5.2 study 库（Portal 资料库数据源）==" | tee -a "$LOG"
DBQ study "SELECT to_regclass('public.courses') AS courses, to_regclass('public.materials') AS materials;" | tee -a "$LOG"
DBQ study "SELECT count(*) AS courses_rows FROM courses;" | tee -a "$LOG"
DBQ study "SELECT count(*) AS materials_rows FROM materials;" | tee -a "$LOG"
echo "-- materials 列清单（挑时间戳列做“最近更新”取证）--" | tee -a "$LOG"
DBQ study "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='materials' ORDER BY ordinal_position;" | tee -a "$LOG"
# 有 updated_at/created_at 列时补跑（把 <时间列> 换成上一步查到的列名）：
# DBQ study "SELECT max(<时间列>) FROM materials;" | tee -a "$LOG"

echo "== 5.3 quizcraft 库（三路径核心）==" | tee -a "$LOG"
echo "-- 全表清单 --" | tee -a "$LOG"
DBQ quizcraft "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY 1;" | tee -a "$LOG"
echo "-- Go 契约表 vs FastAPI 表存在性 --" | tee -a "$LOG"
DBQ quizcraft "SELECT to_regclass('public.quizcraft_banks') AS go_banks, to_regclass('public.quizcraft_bank_versions') AS go_bank_versions, to_regclass('public.quizcraft_questions') AS go_questions, to_regclass('public.quizcraft_question_versions') AS go_question_versions, to_regclass('public.quizcraft_bank_version_questions') AS go_bank_version_questions, to_regclass('public.quizcraft_ranking_settlement_events') AS go_ranking_events, to_regclass('public.question_banks') AS fastapi_question_banks, to_regclass('public.bank_questions') AS fastapi_bank_questions, to_regclass('public.users') AS fastapi_users, to_regclass('public.quizcraft_migration_events') AS migration_events;" | tee -a "$LOG"
# 对“存在”的表补行数（不存在则跳过该行）：
DBQ quizcraft "SELECT count(*) AS go_banks_rows FROM quizcraft_banks;" 2>/dev/null | tee -a "$LOG"
DBQ quizcraft "SELECT count(*) AS fastapi_banks_rows FROM question_banks;" 2>/dev/null | tee -a "$LOG"
DBQ quizcraft "SELECT count(*) AS migration_events_rows FROM quizcraft_migration_events;" 2>/dev/null | tee -a "$LOG"

echo "== 5.4 platform 库（无 schema_migrations 表，迁移由 deploy helper 按 HENUKIT_PLATFORM_MIGRATIONS 显式应用）==" | tee -a "$LOG"
DBQ platform "SELECT to_regclass('public.oauth_clients') AS oauth_clients, to_regclass('public.authorization_roles') AS authorization_roles, to_regclass('public.mail_outbox') AS mail_outbox, to_regclass('public.account_operator_role_grant_audit_events') AS grant_audit;" | tee -a "$LOG"
echo "-- oauth_clients 回调（URL 非密钥，允许记录）--" | tee -a "$LOG"
DBQ platform "SELECT id, redirect_uris FROM oauth_clients WHERE id IN ('portal-gateway','console-gateway') ORDER BY id;" | tee -a "$LOG"
echo "-- mail_outbox 状态分布（platform-mail-worker 消费情况）--" | tee -a "$LOG"
DBQ platform "SELECT status, count(*) FROM mail_outbox GROUP BY 1 ORDER BY 1;" | tee -a "$LOG"
DBQ platform "SELECT count(*) AS undelivered FROM mail_outbox WHERE status IN ('pending','processing','retry_due');" | tee -a "$LOG"

echo "== 5.5 account_portfolio 库（行数基线，只计数不取数据）==" | tee -a "$LOG"
DBQ account_portfolio "SELECT version FROM account_portfolio_schema_migrations ORDER BY version DESC LIMIT 3;" | tee -a "$LOG"
DBQ account_portfolio "SELECT 'accounts' AS t, count(*) FROM account_portfolio_accounts UNION ALL SELECT 'points', count(*) FROM account_portfolio_points UNION ALL SELECT 'memberships', count(*) FROM account_portfolio_memberships UNION ALL SELECT 'orders', count(*) FROM account_portfolio_membership_orders UNION ALL SELECT 'notifications', count(*) FROM account_portfolio_notifications UNION ALL SELECT 'tickets', count(*) FROM account_portfolio_tickets;" | tee -a "$LOG"
```

**通过判据 / 待人工判读**：
- §5.1：9 库全部存在（第二个查询返回 9）。
- §5.2：`courses`/`materials` 存在且有行；行数与最近更新时间记录为基线（「访问量/写方」需结合日志与 `services/api` 是否在跑判断——见 §9）。
- §5.3：**核心结论**——Go 契约表（`quizcraft_banks` 等）是否已建、与 FastAPI 表（`question_banks` 等）是否**同库共存**、`quizcraft_migration_events` 行数与最新 `event_id`（M1 门禁需要）。共存且 FastAPI 仍在写 = 三路径并存，标 `[MANUAL]` 并回填现状文档。
- §5.4：`account_operator_role_grant_audit_events` 存在 → 000018 已应用；`oauth_clients` 的 `console-gateway` redirect_uris 应含 `https://console.henukit.cn/api/v1/auth/callback`（§6 观察期清理后应只剩它）；`mail_outbox` 的 undelivered 数量随时间应趋 0（若持续堆积 → `[FAIL]` 邮件链路）。
- §5.5：`account_portfolio_schema_migrations` 最新 version（期望 ≥ 000008）+ 各表行数基线（首个 Account Portfolio 版本的空库基线在 `henukit-artifact-deployment.md` 有要求，记录当前值即可）。

**证据记录**：5.1–5.5 全部输出 + 结论三句话（study 数据量 / quizcraft 是否共存 / account_portfolio 行数基线）。

---

## §6 服务状态（约 4 分钟）

**目的**：每个关键服务容器内 `/healthz` / `/readyz`；platform-mail-worker 消费；notice 队列长度。补 §1 的容器健康不足以证明 HTTP 层存活。

```bash
# 容器内 HTTP 探针：优先 curl，缺 curl 用 wget 解析状态行
http_get() { # $1=容器名 $2=URL → 打印状态码
  docker exec "$1" sh -c '
    if command -v curl >/dev/null 2>&1; then curl -s -o /dev/null -w "%{http_code}" "$1";
    elif command -v wget >/dev/null 2>&1; then wget -q -O /dev/null --server-response "$1" 2>&1 | grep -m1 "HTTP/" | awk "{print \$2}";
    else echo N/A; fi' sh "$2" 2>/dev/null
}
svc() { # $1=容器名 $2=端口 $3=路径 $4=期望
  local code
  code=$(http_get "$1" "http://127.0.0.1:$2$3")
  [ "$code" = "$4" ] && echo "[PASS] $1 $3 -> $code" || echo "[FAIL] $1 $3 -> ${code:-N/A} (期望$4)"
}

echo "== 6.1 容器内健康探针 ==" | tee -a "$LOG"
svc henukit-platform-core-1        8081 /api/v1/healthz 200 | tee -a "$LOG"
svc henukit-platform-core-1        8081 /api/v1/readyz  200 | tee -a "$LOG"
svc henukit-portal-api-1           8085 /api/v1/healthz 200 | tee -a "$LOG"
svc henukit-portal-gateway-1       8084 /api/v1/healthz 200 | tee -a "$LOG"
svc henukit-console-gateway-1      8082 /api/v1/healthz 200 | tee -a "$LOG"
svc henukit-portal-summary-1       8083 /healthz 200 | tee -a "$LOG"
svc henukit-portal-summary-1       8083 /readyz  200 | tee -a "$LOG"
svc henukit-account-portfolio-1    8097 /healthz 200 | tee -a "$LOG"
svc henukit-notice-1               8094 /healthz 200 | tee -a "$LOG"
svc henukit-food-1                 8096 /healthz 200 | tee -a "$LOG"
svc henukit-library-1              8095 /healthz 200 | tee -a "$LOG"
svc henukit-career-opportunities-1 8097 /healthz 200 | tee -a "$LOG"

echo "== 6.2 platform-mail-worker 消费（platform 库 mail_outbox，见 §5.4）==" | tee -a "$LOG"
DBQ platform "SELECT count(*) FILTER (WHERE status IN ('pending','processing','retry_due')) AS pending, count(*) FILTER (WHERE status IN ('accepted','delivered')) AS done, count(*) FILTER (WHERE status='failed') AS failed FROM mail_outbox;" | tee -a "$LOG"
docker logs --tail 20 henukit-platform-mail-worker-1 2>&1 | tee -a "$LOG"

echo "== 6.3 notice 队列（notice 库 notice_distributions）==" | tee -a "$LOG"
DBQ notice "SELECT status, count(*) FROM notice_distributions GROUP BY 1 ORDER BY 1;" | tee -a "$LOG"
docker logs --tail 20 henukit-notice-worker-1 2>&1 | tee -a "$LOG"
```

**通过判据**：
- §6.1 全部 200（`[FAIL]` 的探针补 `docker logs --tail=50 <容器>` 取证）。
- §6.2：`pending+processing+retry_due` 应接近 0 且随重跑下降；`failed` 大量堆积 → `[FAIL]`（Aliyun DirectMail 链路，见 `aliyun-directmail-setup.md`）。
- §6.3：`notice_distributions` 的 `queued`/`processing` 若大量滞留 → 说明 `NOTICE_DELIVERY_URL`/`NOTICE_DELIVERY_TOKEN` 未接线（对应 §2.2 矩阵），标 `[MANUAL]`（设计如此，delivery 依赖 provider webhook）。

**证据记录**：§6.1 全行 + 两个 worker 的日志尾部摘要 + 队列数字。

---

## §7 资料 / OSS（约 3 分钟）

**目的**：library 库激活 release 查询、OSS bucket 对象清单（服务器有 aliyun CLI 时）、material 激活状态、维护栅栏。回填 `CURRENT_PRODUCTION_STATE.md` §6「materials OSS 当前激活的 release 与对象清单」。

```bash
echo "== 7.1 library 库激活记录（只读）==" | tee -a "$LOG"
DBQ library "SELECT to_regclass('public.library_public_releases') AS releases_t, to_regclass('public.library_public_release_activation_events') AS activation_events_t, to_regclass('public.library_public_material_snapshots') AS snapshots_t;" | tee -a "$LOG"
DBQ library "SELECT release_id, activated_at, material_count FROM library_public_release_activation_events ORDER BY activated_at DESC LIMIT 5;" | tee -a "$LOG"
DBQ library "SELECT release_id, (oss_commit_sha256 IS NOT NULL) AS has_oss_commit, (activation_digest IS NOT NULL) AS has_activation FROM library_public_releases ORDER BY 1 DESC LIMIT 10;" | tee -a "$LOG"
DBQ library "SELECT count(*) AS snapshot_count FROM library_public_material_snapshots;" | tee -a "$LOG"

echo "== 7.2 OSS bucket 对象清单（bucket 名非密钥；aliyun CLI 不在则改为控制台核验）==" | tee -a "$LOG"
BUCKET=$(grep -oP '^LIBRARY_OSS_BUCKET=\K.*' "$ENV_FILE" | head -1)
echo "bucket: ${BUCKET:-<未设置>}" | tee -a "$LOG"
if command -v aliyun >/dev/null 2>&1 && [ -n "$BUCKET" ]; then
  aliyun oss ls "oss://${BUCKET}/" --limit 100 2>&1 | tee -a "$LOG"
  # 列出对象数（不打印对象名之外的敏感信息）：
  aliyun oss ls "oss://${BUCKET}/" --limit 10000 2>/dev/null | wc -l | tee -a "$LOG"
else
  echo "[MANUAL] aliyun CLI 不可用或 bucket 未设置，对象清单改为阿里云控制台人工核验" | tee -a "$LOG"
fi

echo "== 7.3 维护栅栏与静态挂载 ==" | tee -a "$LOG"
ls -la /srv/materials/ 2>/dev/null | tee -a "$LOG"
ls -la /opt/henukit-materials/public/ 2>/dev/null | tee -a "$LOG"
if [ -f /srv/materials/.maintenance ]; then echo "[FAIL] 维护栅栏存在（资料库处于维护态）"; else echo "[PASS] 无维护栅栏"; fi | tee -a "$LOG"
curl -sk -o /dev/null -w '资料库 API /api/v1/library/courses -> %{http_code}\n' https://henukit.cn/api/v1/library/courses | tee -a "$LOG"
```

**通过判据**：
- §7.1：`library_public_release_activation_events` 最新一行存在（release_id + 时间 + material_count）；`library_public_releases` 中最新 release 的 `has_oss_commit`/`has_activation` 为 t。
- §7.2：bucket 可达、对象数与激活 release 的 `material_count` 量级吻合（数量级不符 → `[MANUAL]` 复核管线，见 `henukit-materials-oss-release.md`）。
- §7.3：无 `.maintenance` 栅栏；`/api/v1/library/courses` 未认证返回 401（gateway 鉴权正常，非 503 维护态）。

**证据记录**：激活 release 行、对象数量级、栅栏状态。

---

## §8 支付（约 3 分钟）

**目的**：判断「微信支付 Native 是否上线」的证据采集（D3 决策输入）。注意：WeChat Native 属于**旧栈 `services/api`**（`wechat-pay-native.md`），新栈支付是 Account Portfolio 的 EasyPay；本节点两类都查，只输出键存在性，不打印密钥。

```bash
echo "== 8.1 支付相关 env 键存在性（只输出 true/false/非空，绝不打印值）==" | tee -a "$LOG"
for k in WECHAT_PAY_MODE WECHAT_PAY_APPID WECHAT_PAY_MCH_ID WECHAT_PAY_API_V3_KEY WECHAT_PAY_MERCHANT_SERIAL_NO WECHAT_PAY_NOTIFY_URL ACCOUNT_PORTFOLIO_EASYPAY_ENABLED ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL ACCOUNT_PORTFOLIO_EASYPAY_PID ACCOUNT_PORTFOLIO_EASYPAY_KEY ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL; do
  if grep -q "^${k}=" "$ENV_FILE"; then
    if grep -qE "^${k}=[^[:space:]]+" "$ENV_FILE"; then echo "$k=true(非空)"; else echo "$k=true(空值)"; fi
  else echo "$k=false"; fi
done | tee -a "$LOG"
# EasyPay 开关值（0/1 非密钥，可打印）：
grep -oP '^ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=\K.*' "$ENV_FILE" | tee -a "$LOG"

echo "== 8.2 网关支付回调路由（watcher 断言口径：4xx 且非 404）==" | tee -a "$LOG"
curl -sk -o /dev/null -w 'easypay notifications -> %{http_code}\n' \
  -X POST https://henukit.cn/api/v1/payment-providers/easypay/notifications \
  -H 'Content-Type: application/json' -d '{}' | tee -a "$LOG"

echo "== 8.3 旧栈 WeChat 路由（services/api 退役后应 404）==" | tee -a "$LOG"
curl -sk -o /dev/null -w 'superhuazai.me /api/v1/payments/wechat/native -> %{http_code}\n' \
  https://superhuazai.me/api/v1/payments/wechat/native | tee -a "$LOG"

echo "== 8.4 订单/支付事实表（account_portfolio，只计数）==" | tee -a "$LOG"
DBQ account_portfolio "SELECT status, count(*) FROM account_portfolio_membership_orders GROUP BY 1 ORDER BY 1;" | tee -a "$LOG"
DBQ account_portfolio "SELECT count(*) AS payment_intents FROM account_portfolio_payment_order_intents; SELECT count(*) AS payment_facts FROM account_portfolio_payment_facts;" | tee -a "$LOG"
```

**通过判据 / 待人工判读**（D3 决策输入，本节点不替人拍板）：
- `WECHAT_PAY_*` 全为 `false` → 旧栈 Native 未配置、未上线；若 `WECHAT_PAY_MODE=live` 且商户键齐备但 `services/api` 已退役（§9）→ 配置存在但无流量，需归档清理。
- `ACCOUNT_PORTFOLIO_EASYPAY_ENABLED` = `1` → 新栈 EasyPay 上线中；= `0`/缺省 → 支付关闭（`henukit-artifact-deployment.md` #166 门禁前的默认）。
- §8.2 返回 4xx 且非 404 → 回调路由存在且被鉴权拒绝（符合 watcher 断言）；404 → 路由缺失 `[FAIL]`。
- §8.3 旧栈路由 404 → 退役生效；非 404 → 旧 `services/api` 仍在对外服务支付路由 `[MANUAL]`（与 §9 交叉验证）。
- §8.4 订单/支付表行数：`paid`/在途订单 > 0 → 有历史/在途数据，归档与对账需要纳入 D3 决策。

**证据记录**：§8.1 矩阵、§8.2/8.3 状态码、§8.4 行数；结论一句（支付上线状态的事实陈述）。

---

## §9 残留检查（约 2 分钟）

**目的**：确认旧栈退役边界——`/study-api/healthz` 404（`watch-henukit-actions.sh` 断言）、FastAPI（:10086）不在监听、**Go core 已容器化（方案 2）：宿主机不再有 :10089 监听（容器内 expose，不发布宿主端口）**、旧容器不存在、legacy vhost 保留但仅作回滚。

```bash
echo "== 9.1 退役探针（最终 404）==" | tee -a "$LOG"
curl -sk -L --max-redirs 3 -o /dev/null -w 'henukit.cn/study-api/healthz -> %{http_code}\n' https://henukit.cn/study-api/healthz | tee -a "$LOG"
curl -sk -L --max-redirs 3 -o /dev/null -w 'superhuazai.me/study-api/healthz -> %{http_code}\n' https://superhuazai.me/study-api/healthz | tee -a "$LOG"

echo "== 9.2 旧栈容器（deploy helper 的 legacy_names 正则）==" | tee -a "$LOG"
docker ps -a --format '{{.Names}}' | grep -E '^(henukit-)?(study-api|study-worker|quizcraft-api|quizcraft-web)(-|$)' \
  && echo "[FAIL] 存在旧栈容器（上方列表）" || echo "[PASS] 无 study-api/study-worker/quizcraft-api/quizcraft-web 容器" | tee -a "$LOG"
echo "-- 全部容器名（人工过目，找 apps/web 旧前台等）--" | tee -a "$LOG"
docker ps -a --format '{{.Names}}' | tee -a "$LOG"

echo "== 9.3 端口监听（FastAPI :10086 应无；Go core :10089 容器化后宿主应无）==" | tee -a "$LOG"
ss -ltnp 2>/dev/null | grep -E ':(10086|10089)\b' \
  && echo "[FAIL] 10086/10089 有宿主机监听（上方进程；10089 说明 systemd quizcraft-go.service 未停，10086 说明 FastAPI 未停）" || echo "[PASS] 10086/10089 无宿主机监听" | tee -a "$LOG"
echo "-- quizcraft 容器运行状态（distroless 无 shell，不 exec；容器内 :10089 由 §6 探测经网关验证）--" | tee -a "$LOG"
docker ps --filter "name=henukit-quizcraft" --format '{{.Names}} {{.Status}}' | tee -a "$LOG"

echo "== 9.4 legacy vhost 状态（保留=回滚路径，但不应承载新流量）==" | tee -a "$LOG"
ls /etc/nginx/sites-enabled/ | tee -a "$LOG"
grep -n "proxy_pass\|10086\|superhuazai" /etc/nginx/sites-enabled/superhuazai.me 2>/dev/null | head -15 | tee -a "$LOG"
```

**通过判据**：
- §9.1 两者均 404。
- §9.2 无旧栈容器；若有 `web-1`/`api-1`/`worker-1`/`admin-1` 等旧栈前台容器（apps/web），标 `[FAIL]`（M3 退役项未完成）。
- §9.3 无宿主机监听（FastAPI 已停服 + Go core 已容器化）；若 :10089 有监听 → systemd `quizcraft-go.service` 未停，`[FAIL]` 并回填 `CURRENT_PRODUCTION_STATE.md` §3；若 :10086 有监听 → 旧 QuizCraft FastAPI 仍在运行，`[FAIL]`。
- §9.4 legacy vhost 文件保留（回滚依赖），但若 `superhuazai.me` 仍代理到 `:10086` 且该端口在监听 → 旧服务仍在对外，`[FAIL]`。

**证据记录**：探针码、容器名清单、端口监听结果、vhost proxy_pass 行。

---

## §10 证据收集与汇总（约 3 分钟）

**目的**：把 §0–§9 已追加到 `$LOG` 的全部输出做自动汇总，输出「通过/失败/待人工」表格模板，并回填仓库两份清单。

```bash
echo "== 10.1 标记统计 ==" | tee -a "$LOG"
grep -hoE '\[(PASS|FAIL|MANUAL)\]' "$LOG" | sort | uniq -c | tee -a "$LOG"
echo "== 10.2 未通过项清单 ==" | tee -a "$LOG"
grep -hE '\[(FAIL|MANUAL)\]' "$LOG" | tee -a "$LOG"
echo "日志文件: $LOG" | tee -a "$LOG"
```

**汇总表模板（复制到核验回执，逐行填结果）：**

| 节 | 检查项 | 结果 | 证据（行号/输出摘要） | 备注 |
|---|---|---|---|---|
| §1 | 容器清单 / 镜像 SHA / 健康 / 磁盘 | 通过/失败/待人工 | | 期望 21 容器、18 镜像同 SHA（含 quizcraft） |
| §2 | env 键矩阵 / compose 契约 | 通过/失败/待人工 | | 重点：prebuilt vs release 契约是否被生产采用 |
| §3 | 证书 6 域 / nginx -t / vhost | 通过/失败/待人工 | | study/quiz/account 子域未落地属预期 |
| §4 | 路由 smoke 矩阵 | 通过/失败/待人工 | | /quiz/、/study-api/healthz 必须最终 404 |
| §5 | 9 库 / study 数据 / quizcraft 共存 / platform / account_portfolio | 通过/失败/待人工 | | 三路径并存结论 |
| §6 | 服务 /healthz /readyz / 邮件消费 / notice 队列 | 通过/失败/待人工 | | |
| §7 | library 激活 release / OSS / 栅栏 | 通过/失败/待人工 | | |
| §8 | 支付（WeChat 键 / EasyPay / 订单表） | 通过/失败/待人工 | | D3 决策输入 |
| §9 | 残留（study-api 404 / 10086 / 旧容器） | 通过/失败/待人工 | | |
| §10 | 证据归档 | 通过 | | `$LOG` 路径 |

**回填动作（核验完成后，在仓库内执行）：**
1. 更新 `docs/operations/CURRENT_PRODUCTION_STATE.md` §6「待服务器核验清单」：已核验项打勾并写证据日期；把「仓库无法自答、必须服务器回答」的结论写进 §1–§4 相应小节。
2. 更新 `docs/migrations/ALL_NEW_STACK_CUTOVER.md` M0 清单与 D6 决策行：服务器核验执行人回填完成。
3. 若发现 `release.yml` 契约与服务器 env 不一致（§2.4 有 MISSING），先修服务器 env 或记录「release 契约未覆盖」结论，再决定是否回同步仓库 `.env.henukit.example`。
4. 本 Runbook 不修改任何生产对象；任何修复动作走既有变更流程（`henukit-local-deploy.md` §7 或 release 发布流程），不在核验会话内执行。

---

## 附：核验中发现「仓库无法自答、必须服务器回答」的检查点汇总

以下每一项都无法仅凭仓库代码/文档判定，必须由本 Runbook 在服务器现场取证（这也是本 Runbook 存在的意义）：

| # | 待服务器回答的问题 | 对应节 | 仓库侧已知事实 |
|---|---|---|---|
| 1 | 实际运行服务数与镜像 SHA 是否与 `last-activated-sha` 一致 | §1 | 仓库只知 compose 定义 21 服务/18 镜像 |
| 2 | `/opt/henukit/.env.henukit` 是否仍残留 `STUDY_LEGACY_API_URL`/`STUDY_LEGACY_ADMIN_TOKEN`（仅用于清理盘点） | §2 | ADR-0037 已移除 Library legacy adapter；此二键不再决定 Library 启动，也不得重新加入 prebuilt 强制契约 |
| 3 | 生产是否采用 prebuilt `:?` 契约；`ACCOUNT_PORTFOLIO_*`/`LIBRARY_OSS_*`/`LIBRARY_DOWNLOAD_*`/`QUIZCRAFT_CORE_URL`/`CAREER_DATABASE_URL` 是否真实配置 | §2 | prebuilt 强制这些键而 repo env 未设 → 生产要么不用 prebuilt、要么服务器 env 与 repo 严重不一致 |
| 4 | study 库 `courses`/`materials` 线上行数与最近更新、写方是否还在 | §5/§9 | 表结构来自 GORM AutoMigrate（无 SQL 迁移），行数/写方仓库不可知 |
| 5 | quizcraft 库是否已含 Go 契约表、与 FastAPI 表是否同库共存、`quizcraft_migration_events` 版本 | §5 | Go 契约表在 go-service migration 中定义；是否已应用到生产库不可知 |
| 6 | `account.superhuazai.me`/`account.henukit.cn` 是否已部署 Platform Core | §4 | 仓库无这两个子域的 vhost 模板落地记录（M4 §3：均未落地） |
| 7 | 微信 Native 是否上线（旧栈 `WECHAT_PAY_*` 键）/ 新栈 EasyPay 是否 enabled / 订单表在途数据 | §8 | 仓库只知「未设置则默认关闭」与 #166 门禁未执行 |
| 8 | materials OSS 当前激活 release 与对象清单 | §7 | 需服务器 aliyun CLI + library 库现场查询 |
| 9 | platform 库实际应用到的迁移版本 | §5 | platform-core 无 `schema_migrations` 表，迁移由 deploy helper 显式应用，仓库无法推断服务器状态 |
| 10 | 旧 FastAPI（:10086）/systemd quizcraft-go.service（:10089）/旧容器是否还在跑、`/study-api/healthz` 是否 404 | §9 | 仓库只知「应退役、Go core 已容器化（方案 2）」，现场状态未知 |
| 11 | 验收 smoke 主域口径（superhuazai.me vs henukit.cn）以哪个为准 | §4 | M4 §6 已标注需统一，两口径都要现场记录 |

---

*文档维护：本 Runbook 为只读核验流程；与 `CURRENT_PRODUCTION_STATE.md`（现状记录）冲突时以后者为准并同步修正。执行日期与结论回填见 §10。*

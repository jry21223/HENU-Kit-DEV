# HENU Kit 资料同步：HENU-Final-Review → henukit.cn

公开仓库 [jry21223/HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review)
的 `main` 分支是课程资料的来源，`manifest.json` 是唯一发布清单。此链路只解决资料镜像、
派生 Slides 与遗留 Study 目录同步，不替代 #250 的 Library 数据所有权决策，也不授权任何
Portal、Food 或首页视觉变更。

## 唯一生产链路

生产只配置一个 GitHub Webhook：

- URL：`https://henukit.cn/webhooks/materials`
- loopback listener：`127.0.0.1:10088`
- secret：root-only 的 `/etc/henukit-deploy/materials-webhook-secret`，通过 systemd
  credential 只交给非特权 receiver
- receiver env：`/etc/henukit-deploy/materials-webhook.env`，只含 HTTP 与队列配置，
  不含数据库或 runner 凭据
- privileged sync env：root-only `0600` 的 `/etc/henukit-deploy/materials-runner.env`，只保存
  mirror/release 配置与可选数据库 URL；receiver 和 queue runner 都不能读取
- queue：`/var/lib/henukit-materials-webhook` 的持久队列，
  `HENUKIT_WEBHOOK_MAX_QUEUE=1`、`HENUKIT_WEBHOOK_QUEUE_MODE=latest`
- receiver：`henukit-materials-webhook.service`，只验签、过滤 repository/ref、落盘并返回
  `202`
- watcher：`henukit-materials-webhook.path`
- queue runner：`henukit-materials-runner.service` 仍以 `henukit-deploy` 读取和二次校验持久事件；
  它只调用固定 `/usr/local/libexec/henukit/henukit-materials-sudo`
- privilege seam：sudoers 只允许上述用户调用 root-owned
  `/usr/local/libexec/henukit/henukit-materials-root`；root helper 再次校验完整事件 tuple、读取
  root-only sync env，并执行 canonical driver

不存在第二个 Node webhook、第二套 secret 或绕过持久队列的生产 runner。相同 delivery ID
会去重；burst push 只保留最新的一次待执行任务。若 runner 正在运行，队列至多保留一个最新
rerun。

```text
GitHub push (jry21223/HENU-Final-Review, refs/heads/main)
  -> HTTPS /webhooks/materials
  -> unprivileged receiver + durable latest-only queue
  -> systemd path
  -> unprivileged oneshot queue runner
  -> exact sudo target with a second event-policy check
  -> verified SHA snapshot -> mirror -> Slides -> SQL -> publish + DB transaction
```

## Runner 的 fail-closed 顺序

`henukit-materials-sync` 获取 receiver 已验证的完整 SHA、repository 与 ref，并使用进程锁串行化
手动调用和队列调用。它按以下顺序执行：

1. 只读检查 versioned Study expand migration `0002` 的列、索引与 marker 表均已存在；缺失时在
   修改 public layout 前停止；
2. fetch 目标分支并确认 checkout HEAD 精确等于 webhook SHA；
3. 在 staging 中只复制 manifest 点名且 role 不以“待复核”开头的普通文件；
4. 拒绝越界路径、symlink、点文件、缺失文件，以及 bytes/sha256 与 manifest 不一致的资产；
5. 在 staging 中完成所有适用的 PPT/PPTX Slides 派生；缺依赖或任一转换失败即停止；
6. 从同一个 manifest 生成一次仅含 DML、带 `BEGIN`/`COMMIT` 的幂等目录 SQL；同一事务更新
   `henukit_materials_sync_state` 的 SHA + delivery 恢复 marker；
7. 保持 Docker 已 bind 的 `public/` 根目录 inode 不变，通过 `public/current` 相对 symlink
   原子切换到新 snapshot，再执行一次数据库事务；
8. 数据库失败、进程中断或换入失败时查询事务内 marker：明确未提交才恢复旧 snapshot，
   marker 已提交则完成新 snapshot；数据库不可达、提交结果不明时保留 journal 与两份 snapshot
   并停止，绝不猜测回滚；只有提交已确认后才清理回滚副本。

因此 forged signature、非目标 ref、过期 SHA、损坏 manifest、转换失败和数据库失败都不会留下
最终的半发布状态。SQL 以 `storage_key` 幂等 upsert，并只 archive 不再出现在 manifest 中、且
带 mirror `sha256` 标记的行。Nginx 只读服务 public 目录；git checkout 永不挂载到边缘容器。

## 首次安装

主机需要 `git`、Node.js 18+、Python 3、`python3-pptx`、LibreOffice、Docker Compose、
`flock`、`sudo` 与 `visudo`。直接数据库模式还需要 `psql`。

```bash
sudo services/deploy-webhook/deploy/install.sh --enable-materials-sync
sudo cat /etc/henukit-deploy/materials-webhook.env
sudo cat /etc/henukit-deploy/materials-runner.env
```

安装器会生成独立 secret、安装共享 Go receiver、固定 helper/sudoers，以及三套 materials
systemd unit。升级旧安装时，它先停 watcher、runner 和 receiver，再替换共享二进制；完整
root env 会先在同目录 staging、校验并原子安装，随后才原子收敛 receiver env。旧的
`henukit-materials-sync.sh` 绝对路径会被收敛为 canonical driver symlink。安装结束显式 restart
receiver，但 watcher 保持关闭等待验证。若旧 receiver 进程曾持有数据库 URL，应在升级后轮换
对应数据库凭据；安装器不会把 URL 写回非特权 env。

### 必须先执行的 Study expand migration

安装器只安装 migration 制品，不把 DDL 放进同步运行时。保持 materials watcher 关闭，使用独立、
root-only 的 Study migration 身份执行版本化 Up；不要把该 DDL 身份写入 receiver 或
`materials-runner.env`。以下示例假定主机已有受控 libpq service/pgpass：

```bash
sudo env \
  PGSERVICEFILE=/etc/henukit-deploy/study-migrator.pg_service.conf \
  PGSERVICE=henukit-study-migrator \
  PGPASSFILE=/etc/henukit-deploy/study-migrator.pgpass \
  psql -X -v ON_ERROR_STOP=1 \
    -f /usr/local/share/henukit/migrations/study/0002_henukit_materials_sync_expand.up.sql

sudo env \
  PGSERVICEFILE=/etc/henukit-deploy/study-migrator.pg_service.conf \
  PGSERVICE=henukit-study-migrator \
  PGPASSFILE=/etc/henukit-deploy/study-migrator.pgpass \
  psql -X -v ON_ERROR_STOP=1 -Atc \
    "SELECT to_regclass('public.henukit_materials_sync_state'), to_regclass('public.materials_storage_key_active_idx');"
```

Up 在同一 migration advisory lock 下可安全重复，先检查 Study baseline 与重复 active
`storage_key`，再做 additive expand。前置检查、锁影响、验证和兼容性 Down 记录在同目录
`0002_henukit_materials_sync_expand.md`。首次 manual sync 与 watcher 启用都必须晚于验证；driver
自身只做 schema preflight 和事务 DML。

数据库二选一：

- 默认：driver 读取 `/var/lib/henukit-actions-watch/last-activated-sha`，只允许同 SHA 的
  `/opt/henukit-releases/<sha>/RELEASE_SHA` 与 release compose，并只读取现有受控发布链路的
  root-only `/etc/henukit-deploy/materials-production.env`，随后显式注入 `RELEASE_SHA=<sha>`
  后进入 `postgres` 容器；该文件及从 `/` 开始的每层父目录都必须 root-owned、不可由 group/other
  写入且不可为 symlink，避免检查后被 `henukit-deploy` 换掉；不按 mtime 猜 release，也没有开发
  凭据 fallback；
- 显式设置 `HENUKIT_MATERIALS_DATABASE_URL`：root helper 解析 URL 后通过 libpq 环境调用主机
  `psql`，密码不出现在进程参数中。

首次安装或从旧默认 `/opt/henukit/.env.henukit` 升级时，从受控 secret 来源重建一份独立副本，
不要让 materials runner 继续按路径读取 `henukit-deploy` 可写目录中的文件：

```bash
sudo install -o root -g root -m 0600 /root/henukit-production.env \
  /etc/henukit-deploy/materials-production.env
sudoedit /etc/henukit-deploy/materials-runner.env
```

安装后先保持 watcher 关闭。把
`services/deploy-webhook/deploy/nginx-materials.conf.example` 中唯一的
`location = /webhooks/materials` 合入当前 HTTPS vhost；生产 vhost 使用既有
`/etc/nginx/sites-enabled/henukit.cn`，不得另起一个抢占同域名的 server 块。随后按以下顺序验证：

首次 manual 会把旧资料布局收敛到 `public/current/files/`。因此在执行 manual 之前，必须先按
[`henukit-artifact-deployment.md`](./henukit-artifact-deployment.md) 的固定 SHA、签名、preflight 与审批流程，
激活已经包含本变更的 release；不得让旧 edge alias 在布局迁移后继续承载 `/materials/`。激活后还要从
正在运行的 edge 配置证明 `current/files/` alias 已生效，任何一步失败都保持 watcher 关闭且不得运行 manual。

```bash
# 1. 受控发布流程已完成 preflight/审批后，先激活包含新版 materials alias 的固定 SHA release。
sudo /usr/local/sbin/activate-henukit-release <full-main-sha> --execute
active_sha="$(sudo cat /var/lib/henukit-actions-watch/last-activated-sha)"
test "$active_sha" = "<full-main-sha>"
sudo env RELEASE_SHA="$active_sha" docker compose \
  --env-file /etc/henukit-deploy/materials-production.env \
  -f "/opt/henukit-releases/$active_sha/docker-compose.henukit.release.yml" \
  exec -T nginx nginx -T | grep -F 'alias /srv/materials/current/files/;'

# 2. 落地 webhook location，配置测试通过后才 reload。
sudoedit /etc/nginx/sites-enabled/henukit.cn
sudo nginx -t
sudo systemctl reload nginx

# 3. 唯一 manual 入口是 root helper；它仍读取并校验 root-only runner env。
sudo /usr/local/libexec/henukit/henukit-materials-root --manual

curl -fsS http://127.0.0.1:10088/readyz
curl -fsS http://127.0.0.1:10088/statusz
curl --fail --silent --show-error --head \
  'https://henukit.cn/materials/%E9%AB%98%E7%AD%89%E6%95%B0%E5%AD%A6A%EF%BC%88%E4%BA%8C%EF%BC%89/%E5%A4%8D%E4%B9%A0%E8%AE%B2%E4%B9%89/%E9%AB%98%E7%AD%89%E6%95%B0%E5%AD%A6A%EF%BC%88%E4%BA%8C%EF%BC%89_%E8%80%83%E5%89%8D%E5%A4%8D%E4%B9%A0%E7%9F%A5%E8%AF%86%E7%82%B9%E8%AE%B2%E4%B9%89.pdf'
test "$(curl -s -o /dev/null -w '%{http_code}' https://henukit.cn/materials/.git/config)" = 404
test "$(curl -s -o /dev/null -w '%{http_code}' https://henukit.cn/materials/course/.hidden.pdf)" = 404

# 4. 只有新版 edge alias、live location、manual sync 和公网 probe 都成功后才启 watcher。
sudo systemctl enable --now henukit-materials-webhook.path
sudo journalctl -u henukit-materials-runner.service -f
```

GitHub Webhook 只选择 push event，content type 为 `application/json`，secret 必须与上述
root-only 文件一致。发送 Ping 后，再以一次真实目标分支 push 验证 `statusz.last_success` 的
delivery 与 SHA。

## Nginx 不变量

`infra/nginx/henukit.conf.example` 与 release compose 必须共同保留：

- `autoindex off`；
- `/materials/` 任意层级的点路径段拒绝；
- `X-Content-Type-Options: nosniff`；
- `Content-Disposition: attachment`；
- `Content-Security-Policy: default-src 'none'; sandbox`；
- `Cache-Control: public, max-age=86400`；
- `public/` 是稳定的只读 bind 根，Nginx 只 alias `current/files/`；checkout、Slides、事务
  metadata 都不在可服务子树内。

## 回滚

失败运行会由 driver 在数据库 marker 明确未提交时原子恢复本轮开始前的 `current` snapshot；
`slides` 与 `SYNCED_SHA` 均通过该 snapshot 一同恢复，数据库事务由 `ON_ERROR_STOP` 回滚。
若数据库暂时不可达且 COMMIT 结果不明，保持 watcher 关闭并恢复数据库可用性，再让同一 driver
按 journal + marker 收敛；不得手动删 `.sync-transaction` 或任一 snapshot。对已成功发布但需要撤销
的内容，唯一正常回滚路径是在
`HENU-Final-Review/main` revert 对应 manifest/asset commit 并 push；该 push 重新进入同一
webhook、queue 和 runner，文件与目录数据库一起收敛。

若回滚 migration，先停 watcher，再用同一受控 migration 身份执行
`0002_henukit_materials_sync_expand.down.sql`。Down 只删除私有 marker 表，保留已承载数据的
additive columns/index；此后 driver 会在 public 变更前 fail closed，直到重新执行并验证 Up。

紧急停止新同步时先停 watcher，不删除当前文件或数据库行：

```bash
sudo systemctl disable --now henukit-materials-webhook.path
sudo systemctl stop henukit-materials-runner.service
curl -fsS http://127.0.0.1:10088/statusz
```

确认原因和回滚 commit 后重新启用 watcher。不得直接运行低层 mirror/import helper、手改
public、跳过验签或绕过 queue；这些都会破坏文件与目录的一致性证据。

## 验证命令

```bash
bash -n scripts/ops/sync-henukit-materials.sh scripts/ops/henukit-materials-sync.sh services/deploy-webhook/deploy/install.sh services/deploy-webhook/deploy/henukit-materials-root services/deploy-webhook/deploy/henukit-materials-sudo
python3 -m py_compile scripts/ops/convert-henukit-slides.py
node --test scripts/ops/tests/henukit-materials-sync.test.mjs scripts/ops/tests/henukit-materials-deploy-path.test.mjs scripts/ops/tests/import-henukit-materials.test.mjs
cd services/deploy-webhook
test -z "$(gofmt -l .)"
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go test -race -count=1 ./...
```

# HENU Kit 资料库同步（HENU-Final-Review → henukit.cn）

仓库能力不等于生产已启用。以下是 #306 唯一受支持的生产路径；启用 watcher 前仍须完成
主机配置、Nginx 检查、数据库 schema 预检和受控 smoke。

## 唯一入口与信任边界

| 边界 | 固定值 |
| --- | --- |
| Webhook URL | `https://henukit.cn/webhooks/materials` |
| Listener | `henukit-materials-webhook.service`，`127.0.0.1:10088` |
| Secret | systemd credential `/etc/henukit-deploy/materials-webhook-secret` |
| Queue | `/var/lib/henukit-materials-webhook` latest-arrival：最多一个 running、一个 waiting |
| Runner | `henukit-materials-runner.service`（root、固定 orchestrator） |
| Public root | `/opt/henukit-materials/public` |
| Sealed root | `/opt/henukit-materials/sealed` |

接收器以 `henukit-deploy` 运行，恒定时间校验 GitHub HMAC，并只接受配置仓库与目标 ref。
root runner 不接受 payload 指定命令、路径或数据库：它核对 root-owned 配置后，将准备步骤
降权给 `henukit-deploy`，再依次 seal 和 activate。不要运行退休的
`sync-henukit-materials.sh` 或 `henukit-materials-sync.sh`，也不要建立第二套 webhook/service。

## 启用前

1. 安装 Node、Python 3、`python3-pptx`、PostgreSQL client 与 LibreOffice（`soffice`），再用
   `install.sh --enable-materials-sync` 安装固定 wrapper、unit 和拒绝连接的配置模板；安装器会
   对这些依赖失败关闭。
2. 核对 `/etc/henukit-deploy/materials-seal.env` 的 repository 和 ref 与
   `materials-webhook.env` 一致。精确 SHA 只能来自已验签并入队的 push 事件；配置中没有
   可人工漂移的 `HENUKIT_MATERIALS_SOURCE_SHA`。
3. 将 root-owned `0600` 的 `/etc/henukit-deploy/materials-postgresql.conf` 改为实际 Study
   PostgreSQL service 配置；`materials-activate.env` 只保存该文件路径和 service 名称。public
   root 必须是宿主的 `/opt/henukit-materials/public`，不是容器内 `/srv/materials`。
4. 首次切换前，从现有 catalog 导出并人工核对旧镜像的全部裸 `storage_key`（包括已从新
   manifest 删除的资料），写入 root-owned `0600` 的
   `/etc/henukit-deploy/materials-legacy-inventory.json`：
   `{"version":1,"storage_keys":["科目/资料.pdf"]}`。新安装且没有旧 catalog 时保留空数组。
5. 验证配置与 legacy inventory 均为 root-owned、不可被 group/other 写；secret 为 `root:root 0400`。
6. 安装 `infra/nginx/henukit.conf.example`，运行 `nginx -t`。Compose 只读挂载 public root
   到 `/srv/materials`；Nginx 只将 `/materials/releases/<release-id>/<public-path>` 映射到
   `/srv/materials/releases/<release-id>/public/<public-path>`，不公开内部 `current` 指针。
7. 先保持 `henukit-materials-webhook.path` disabled；完成 schema preflight、备份和回滚演练后
   才 `systemctl enable --now henukit-materials-webhook.path`。

## 激活、故障恢复与维护围栏

激活持有 `/run/henukit-materials-activate.lock`。它先创建
`/opt/henukit-materials/public/.maintenance`；此时 `/materials/`、Portal Library API 与
Console Library API 均返回 503。随后 `activation-journal.json` 依次记录：

1. `prepared`：目标 release 的文件和目录已校验并持久化；
2. `static_switched`：`current` 已指向目标公开树；
3. `database_running`：catalog 事务结果可能未知；
4. `database_committed`：数据库已提交，只需完成 marker/pointer；
5. 写入 `ACTIVE_RELEASE`，删除 journal，最后删除 fence。

`prepared` 或 `static_switched` 阶段失败会恢复旧 pointer/marker。`database_running` 失败必须
保持 503：确认数据库可用后，用同一 release ID 和 receipt digest 重试；导入是幂等的。
`database_committed` 重试不会再次执行 catalog SQL。不要手工删除 fence、journal 或修改
`current`，否则会丢失恢复依据。

Catalog 的 `storage_key` 带不可变 release 前缀，因此浏览器的一天缓存不会把旧文件伪装成
新 catalog 内容。数据库凭据只在 root-only PostgreSQL service file 中，client 通过
`PGSERVICEFILE`/`PGSERVICE` 选择它；凭据不得出现在命令行参数、环境变量或日志中。
第一次切换时，事务只会归档受审 legacy inventory 与本次 manifest 精确列出的旧裸路径；
它不会按 `sha256` 扫描或归档其他来源的资料。首切 smoke 还应确认 Library 不再返回旧裸路径。

## 显式 rollback

Rollback 是把上一个仍保留的 sealed release 再做一次完整激活，而不是单独回退文件：

```bash
sudo /usr/local/libexec/henukit/henukit-materials-activate \
  --release-id <previous-release-id> \
  --receipt-sha256 <previous-sealed-receipt-sha256>
```

该命令使用相同 lock、fence、journal、catalog 事务和恢复规则。执行前核对 previous release、
receipt、数据库备份及当前 `ACTIVE_RELEASE`；完成后同时验证资料 URL、Library API catalog 和
`ACTIVE_RELEASE`。禁止只改 symlink、只跑 SQL 或恢复旧同步脚本。

## 验证

```bash
go test -race -count=1 ./... # cwd: services/deploy-webhook
node --test scripts/ops/tests/deploy-webhook-materials-ci.test.mjs \
  scripts/ops/tests/activate-henukit-materials.test.mjs \
  scripts/ops/tests/henukit-materials-nginx.test.mjs \
  scripts/ops/tests/henukit-materials-orchestrate.test.mjs \
  scripts/ops/tests/henukit-materials-activate-wrapper.test.mjs
docker compose -f docker-compose.henukit.yml config --quiet
docker run --rm \
  -v "$PWD/infra/nginx/henukit.conf.example:/etc/nginx/conf.d/default.conf:ro" \
  nginx:1.27-alpine nginx -t
```

生产完成证据还必须包含 webhook delivery、queue drain、runner 日志、目标 SHA/release/receipt、
无 fence/journal 残留、真实资料下载、两侧 Library API 和 rollback 演练结果。

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

本轮产品边界是 **OSS 原文件下载 only**：prepare/seal/activate 不调用 Python、LibreOffice 或
Slides converter，不生成在线预览页。`slides/` 必须为空，`derived-inventory.json` 必须是空
`assets`；Library owner snapshot 不保存任何在线预览字段。Portal 详情页只导航到同源 owner façade
`/api/v1/library/materials/<id>/download`；Gateway 从 Library 获取最长 60 秒、固定 OSS public
host 的 grant 后返回 `303`。浏览器页面不接收、不保存也不拼接 OSS URL，不提供任何资料的
在线 reader、Slides viewer、“立即阅读”或“免费试读”。旧 `/library/read/<id>` 与
`/library/slides/<id>` 路由只重定向回资料详情。

接收器以 `henukit-deploy` 运行，恒定时间校验 GitHub HMAC，并只接受配置仓库与目标 ref。
root runner 不接受 payload 指定命令、路径或数据库：它核对 root-owned 配置和 webhook
携带的 exact SHA 后，将准备步骤降权给 `henukit-deploy`，再依次 seal、完整 OSS publish 和
Library owner activate。不要运行退休的
`sync-henukit-materials.sh` 或 `henukit-materials-sync.sh`，也不要建立第二套 webhook/service。

准备阶段的 detached checkout 与 reviewed-asset mirror 只是临时校验字节，不是审计或恢复依据。
非特权 prepare wrapper 在成功和失败路径都会删除本次 exact direct-child
`.attempt.<10 位字母数字>`，确认删除后才向 root runner 返回 locator；清理失败必须阻断 seal、
OSS publish 和 Library activation。C0 仍按 authenticated SHA 独立拉取并校验源仓库，sealed
`.audit` 只保留 locator 与 release/receipt 的相关性。因此 runner 空闲时
`/var/lib/henukit-materials-webhook/candidates` 应为空。

## 启用前

1. 只使用 `docs/operations/henukit-artifact-deployment.md` 的正式流程部署签名 runtime；生产侧
   `scripts/ops/deploy-henukit-artifact.sh` 会在验签和 SHA-256 校验后调用 artifact 内的
   `materials-runtime/install.sh`，以预构建 Linux/amd64 binary-only 方式安装 wrapper、unit 和
   Library activation binary。禁止运行已退休的 `install.sh --enable-materials-sync`，也不得在生产执行
   `go build`、Python、`python3-pptx`、LibreOffice 或 Slides converter。
2. 核对 `/etc/henukit-deploy/materials-seal.env` 的 repository 和 ref 与
   `materials-webhook.env` 一致。精确 SHA 只能来自已验签并入队的 push 事件；配置中没有
   可人工漂移的 `HENUKIT_MATERIALS_SOURCE_SHA`。
3. `materials-activate.env` 只保留 sealed/public/OSS audit/activation staging 路径、Library
   数据库 URL 与 Library OSS RAM role；不得恢复 Study importer、psql service 或 legacy inventory。
   public root 必须是宿主的 `/opt/henukit-materials/public`，不是容器内 `/srv/materials`。
4. 验证配置均为 root-owned、不可被 group/other 写；secret 为 `root:root 0400`。
5. 安装 `infra/nginx/henukit.conf.example`，运行 `nginx -t`。Compose 只读挂载 public root
   到 `/srv/materials`，仅用于 Library API 激活维护围栏；Nginx 对所有 `/materials/**`（包括
   release-prefixed 和 legacy URL）固定返回 no-store `404`，不得映射或回退到本地文件。
6. 先保持 `henukit-materials-webhook.path` disabled；完成 Library schema 核验、备份和回滚演练后
   才 `systemctl enable --now henukit-materials-webhook.path`。
7. 按 manifest 中非待复核资料的 `bytes` 总和核对磁盘峰值；prepare 与 seal 都会独立拉取并
   校验 exact SHA，且激活前必须同时保留上一 sealed/public release。若 idle candidate root
   存在旧 `.attempt.*`，先保持 path/runner 停用，证明没有 running event、进程或打开文件，
   再只以 `henukit-deploy` 身份删除已核对的 direct child。不得删除 candidate root、queue、
   processed/failed、sealed `.audit`、`ACTIVE_RELEASE`、journal、fence 或当前 public release。
8. `henukit-materials-runner.service` 必须保留 `NoNewPrivileges=yes`，但不能同时设置
   `RestrictSUIDSGID=yes`。该 runner 以 root 进入固定 orchestrator，再有意通过 `runuser`
   降权执行 prepare；生产 systemd 在两项组合时会以 `EPERM` 阻断这次 UID 转换，使流程在
   Git、OSS 与数据库操作之前失败。receiver 仍保留自己的完整非特权沙箱。

## 激活、故障恢复与维护围栏

激活持有 `/run/henukit-materials-activate.lock`。它先创建
`/opt/henukit-materials/public/.maintenance`；此时 Portal Library API 与 Console Library API
返回 503，而 Nginx 对 `/materials/**` 仍始终返回 no-store 404。随后
`activation-journal.json` 依次记录：

1. `prepared`：目标 release 的文件和目录已校验并持久化；
2. `static_switched`：`current` 已指向目标公开树；
3. `library_running`：Library owner 事务结果可能未知；
4. `library_committed`：Library owner catalog 已原子提交；
5. 写入 `ACTIVE_RELEASE`，删除 journal，最后删除 fence。

`prepared` 或 `static_switched` 阶段失败会恢复旧 pointer/marker。`library_running` 结果未知时
保持 503，并只允许用同一 release ID 和 receipt digest 重试；Library activation 按 release
identity 幂等收敛。历史 `database_running` journal 不会恢复已退役 Study importer，必须保持
围栏并人工核对；历史 `database_committed` 只允许完成 marker。不要手工删除 fence、journal 或修改
`current`，否则会丢失恢复依据。

若 runner 在 prepare 开始前因 unit/sandbox 故障退出，latest-arrival Store 会把原事件及错误
保留到 `failed/`，不会把它伪装成成功。修复并部署 unit 后，保持 path 停用，确认
queue/running 为空、HENU-Final `main` 仍是该 exact SHA，随后只在 GitHub 仓库 Webhooks 的
原 push delivery 上执行 **Redeliver**。GitHub 重投会保留原 `X-GitHub-Delivery` 并重新携带
有效 HMAC；failed 记录不参与 receiver dedupe，因此同一真实事件可重新入队且原失败审计仍
保留。不得运行 generic `retry`、复制/编辑 queue JSON、删除 failed marker、伪造 webhook，
也不得用空提交代替真实重投。重投到达后、重新启用 path 前，必须确认 receiver 返回
`queued=true`、`duplicate=false`，queue latest 仍是该 exact delivery GUID 与 SHA，并再次确认
HENU-Final `main` 仍是同一 SHA；任一项不一致都保持 path 停用并人工核对。启用后继续核对
runner 日志中的 delivery 与 SHA。

激活前必须验证 sealed receipt 的 `slides.status` 为 `disabled`，派生目录为空且 canonical
digest 等于空列表；任一派生预览资产、converter 参数或旧 `--slides-dir` 参数都必须失败关闭。
这不是“转换失败后降级”，而是明确不提供在线预览的发布契约。

Catalog 的 `object_key` 带不可变 release/receipt/SHA 前缀并绑定精确 OSS VersionId，因此浏览器
的一天缓存不会把旧文件伪装成新 catalog 内容。Library 数据库 URL 和 OSS RAM role 只从
root-owned activation 配置传给固定 Library activator；不得出现在调用参数或日志中。

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

下载 smoke 必须从 Portal 资料详情的同源 download URL 开始，先观察 `303`，只记录重定向的
host、path、TTL/状态而不输出完整 query，再跟随 grant 下载并将 bytes/SHA-256 与 sealed
manifest 对账。同时确认页面没有“浏览幻灯片”动作、旧 slides URL 回到详情页、数据库
active release 的 `library_public_material_snapshots.material_type` 只包含
`handout/exam/slides/exercise/answer/note/textbook`，且本次授权教材以 `textbook` 出现。
直接公开 Bucket/Object URL、在 Portal 注入签名 URL、或用本地 `/materials/` 作为成功证据均不合格。

生产完成证据还必须包含 webhook delivery、queue drain、runner 日志、目标 SHA/release/receipt、
无 fence/journal 残留、真实资料下载、两侧 Library API 和 rollback 演练结果。

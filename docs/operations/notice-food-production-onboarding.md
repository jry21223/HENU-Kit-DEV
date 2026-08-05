# Notice / Food 生产编排接入（#239 #240）

Notice（通知审核与分发）与 Food（美食运营）是完整实现的独立数据所有者：各有数据库、幂等表、审计与 CI workflow，此前只是从未纳入生产编排，导致 Console 中两个模块长期显示「暂不可用」。本文记录将它们接入生产编排的方式、部署与回滚步骤。

## 编排拓扑

- `notice`：`services/notice`，端口 `:8094`，容器名 `henukit-notice`
- `notice-worker`：`services/notice/Dockerfile.worker`，通知分发守护进程（SKIP LOCKED 领取队列、失败重试 3 次、记录审计结果）
- `food`：`services/food`，端口 `:8096`，容器名 `henukit-food`
- 三个服务只接受 console-gateway 的签名调用，校验 `NOTICE_SUMMARY_*` / `FOOD_SUMMARY_*` 凭据对（Gateway 侧展示的同一对凭据）
- Console 网关通过 `NOTICE_API_URL` / `FOOD_API_URL` 指向对应服务；未配置时网关照旧优雅降级为不可用，不会 panic

生产 PostgreSQL 中 `notice` 与 `food` 数据库已存在，无需建库；迁移文件随发布产物打包（`runtime/migrations/notice/`、`runtime/migrations/food/`），由 `deploy-henukit-artifact.sh` 在激活发布前按文件名顺序逐条应用到对应 owner 数据库（两个服务没有内嵌迁移运行器；全新主机上数据库不存在时脚本会先 `createdb`，既有生产库不受影响）。

## 配置项

| 变量 | 说明 | 默认 |
|---|---|---|
| `NOTICE_API_URL` | console-gateway 访问 notice 的基地址；未配置或置空则模块降级为不可用 | 空 |
| `FOOD_API_URL` | console-gateway 访问 food 的基地址；未配置或置空则模块降级为不可用 | 空 |
| `NOTICE_SUMMARY_URL` / `FOOD_SUMMARY_URL` | 概览汇总接口基地址；未配置则对应模块在概览页降级 | 空 |
| `NOTICE_DATABASE_URL` / `FOOD_DATABASE_URL` | 各自数据库连接 | compose 内默认指向 `postgres:5432/notice`、`/food` |
| `NOTICE_REDIS_URL` / `FOOD_REDIS_URL` | 幂等/队列使用的 Redis（DB 3 / 4） | `redis://redis:6379/3`、`/4` |
| `NOTICE_SERVICE_CLIENT_ID/SECRET/KEY_ID` | notice 校验的服务间凭据 | 复用 `NOTICE_SUMMARY_*` |
| `FOOD_SERVICE_CLIENT_ID/SECRET/KEY_ID` | food 校验的服务间凭据 | 复用 `FOOD_SUMMARY_*` |
| `NOTICE_DELIVERY_URL` / `NOTICE_DELIVERY_TOKEN` | 通知分发 webhook 提供方；为空时分发任务留在队列中，worker 重试但不影响发布 | 空 |

## 发布

1. `deploy-henukit.yml` 构建并固定 SHA：`henukit-notice`、`henukit-notice-worker`、`henukit-food` 三个镜像，与其余服务同一流程（构建 → 校验 → 加载）。
2. 发布产物包含三个服务的迁移文件与 `docker-compose.henukit.prebuilt.yml` 中的 `!reset null` 服务块（`image: henukit-*:${RELEASE_SHA:?RELEASE_SHA is required}`、`pull_policy: never`）。
3. 激活发布时设置上表环境变量；`NOTICE_DATABASE_URL` / `FOOD_DATABASE_URL` / Redis / 服务间凭据缺失时 compose 直接报错（`:-?`），避免「以为在跑其实没接上」的半配置状态。这与其余服务的强制校验一致。`NOTICE_API_URL` / `FOOD_API_URL` / 两个 `*_SUMMARY_URL` 未配置或置空时保持空值（与 `LIBRARY_API_URL` 同模式），网关照旧降级为不可用。
4. 健康检查：`/notice-healthcheck`（food 同款），compose `service_healthy` 依赖 `postgres` / `redis`。

## 回滚

- 镜像按固定 SHA 发布，回滚 = 把 `RELEASE_SHA` 指回上一版本并重新加载，与其余服务完全一致（见 `docs/operations/henukit-artifact-deployment.md` 的回滚章节）。
- 数据库迁移只追加不修改；若必须撤销本次迁移，按各服务 `db/migrations/*.down.sql` 执行。
- 需要临时摘除模块时：清空 `NOTICE_API_URL` / `FOOD_API_URL` 并重启 console-gateway，模块回到「暂不可用」降级状态，不影响其他模块。

## 验证

- 本地 compose：`docker compose -f docker-compose.henukit.yml config` 通过；`docker compose up notice food notice-worker` 健康检查通过。
- 集成：拥有 `notice.read` / `food.read` 权限的运营在 Console 看到真实数据，能完成审核、分发与调档确认（`apps/console/tests/notice.spec.ts`、`tests/food.spec.ts`）。
- 未配置时：网关对两模块返回 503 降级，不 panic（网关既有降级逻辑，未改动）。

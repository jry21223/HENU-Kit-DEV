# 全量切换新版本服务修复方案（ALL-NEW STACK CUTOVER）

> 目标：**所有产品与数据全部运行在 HENU Kit 新栈上**（`docker-compose.henukit.yml` 20 个服务），旧 Study 栈（apps/web、apps/study-legacy-admin、services/api、services/worker、study 库依赖）退役。
> 依据：4 个并行验证（QuizCraft 切流就绪度 / study legacy 依赖图 / 新栈部署完备性 / 文档漂移核对）+ [`CURRENT_PRODUCTION_STATE.md`](../operations/CURRENT_PRODUCTION_STATE.md)。
> 状态：**v1 完成**（4 个验证 subagent 证据已全部并入，含 M4 部署细节）。日期基准 2026-08-17。

## 0. 当前事实基线（验证结论摘要）

1. **生产状态（2026-08-19 服务器实测）**：20 个 henukit 容器全量运行（镜像 SHA 5c40cf0f）；console.henukit.cn 已 GO（08-03）；henukit.cn apex 有证书；QuizCraft FastAPI 已停服、Go 已切流（见下条）；旧栈已物理退役（无 study 容器，superhuazai.me 全站 301 到 henukit.cn 栈）；study 库仍是生产 Portal 资料库的数据源。
2. **QuizCraft Go 已切流（8-14 起；2026-08-19 服务器实测）**：宿主机 quizcraft-go.service（quizcraft-server）运行中，`QUIZCRAFT_WRITES_ENABLED=1`、`QUIZCRAFT_PORTAL_COMMANDS_ENABLED=1`，quizcraft_v2 12 题库/3457 题/45 sessions；FastAPI 已停服。剩余为**代码部署与收尾**（部署 M1 代码、排行结算、cutover 证据文档，见 M1 状态更新）；`PRACTICE_SERVICE_URL` 四路复用仍是部署 M1 的接线点（ADR-0036 / 研究文档）。
3. **旧栈退役的真实依赖收敛为三点**：① portal-api 直读 study 库 courses/materials（SQL 硬编码列名）；② services/library 对 services/api admin 路由的条件性 HTTP 适配（当前 compose 默认配置下 fail-closed **启动失败**，非降级）；③ ops 材料导入脚本直写 study 库。
4. **study 库 47 张表**：schema owner 仍是 services/api 的 GORM AutoMigrate；memberships/entitlements/notifications 的 manifest owner 是 platform_core 但实现仍在 study 库；blog/forum/moments/wiki/points/ai/orders 等无 owner（冻结功能）。
5. **文档/配置漂移成体系**：modules.yaml 缺 8 个活跃服务 + platform_worker 命名错 + campus_life 悬空；执行规格 Current 行过时（Console Gateway 已交付）；镜像清单与权威脚本矛盾。

## 1. 分阶段方案

### M0 现状固化与漂移修复（1–2 天，纯文档，无风险）

- [x] 建 `docs/operations/CURRENT_PRODUCTION_STATE.md`（唯一现状记录）
- [x] 修 README `deploy-study.yml` 漂移；修现状文档 console GO / 20 服务 / 开关名 / library fail-closed
- [ ] `henukit.modules.yaml`：`platform_worker` → `platform-mail-worker`（platform-core 内，status active）；补录 8 个活跃服务（account-portfolio、notice、notice-worker、food、library、career-opportunities、portal-summary、food-mcp、career-mcp）；`food_rankings` owner 改 `food`
- [ ] 执行规格 `henukit-console-executable-spec.md`：重写 Delivery Status Current 行（Console Gateway 已交付并生产验证 08-03、六模块真实接线、materials OSS 已交付）；Planned 只留未交付项
- [ ] `engineering-release-spec.md` §6 镜像清单改为以 `scripts/ops/henukit-release-images.sh`（18 镜像，含 quizcraft）为权威
- [ ] CONTEXT-MAP.md：Notice 移入 Current contexts；根 README「美食榜单是 Portal 内模块」改为独立 Food 服务
- [ ] 服务器核验回填：运行清单/SHA、`/opt/henukit/.env.henukit` 的 `STUDY_LEGACY_API_URL`、study 库数据量、quizcraft 库表结构、account 域部署
- [ ] 顺手清理：`apps/admin`（仅 dist 无源码）、根 `docker-compose.yml`（废弃标注或删除）、`.gitignore` 残留行

### M1 QuizCraft 全 Go 切流（执行 ADR-0013）

> 2026-08-19 更新（服务器实测）：**切流已实际执行**（quizcraft-go.service 8-14 起、writes/portal commands 开启、quizcraft_v2 12 题库/3457 题、FastAPI 已停服）；本节降级为**收尾清单**——剩余：① 部署 M1 代码（生产 gateway 仍为旧代码，`/api/v1/practice/banks` 直读幽灵路径 200-空目录）；② 排行结算（settlements=0，settleranking 未跑）；③ 补 cutover 证据文档（ADR-0013 门禁脚本此前从未执行）。

**前置裁决（必须先拍板，出 ADR）**：Portal 刷题读路径的最终形态。选项：
- **A（推荐）**：读也走 Gateway → Go core（V2 catalog/rankings），portal-api 直读降级为过渡并最终移除——单一事实来源，与 ADR-0018 方向一致，但 Portal UI 要换 catalog 数据源
- **B**：portal-api 直读 DB 正式成为 practice 读路径 owner（补 ADR 划定边界），Go core 只做写——改动小，但双读来源长期并存
- 无论选哪个，都要出一份**接线矩阵**文档裁决 `PRACTICE_SERVICE_URL` 四路复用与 `QUIZCRAFT_CORE_URL` 的语义（网关侧切流配置）

**执行步骤**（每步含门禁）：
1. 服务器核验：quizcraft 库当前表结构（Go 契约表是否已存在）、FastAPI 是否仍写同一库、`quizcraft_migration_events` 触发器版本
2. 数据准备：`migrate` / `importbank` / `reconcile` 全量对账；`quizcraft_v2` 独立库快照；`migration_run_id` 固化
3. 影子阶段：Go 服务以 `QUIZCRAFT_WRITES_ENABLED=0` 部署，与 FastAPI 并行观察（shadow-gate 按 ADR-0013 已非强制，离线对比保留）
4. 技术停写窗口：最终增量同步 + 双库可恢复快照 + 数据对账；`verify-cutover.sh` 三重门禁（服务器现场：GO_BASE_URL/LEGACY_BASE_URL/EXPECTED_* 24+ 变量、root-owned 证据文件、`max(event_id)` 头部核对、双库备份 SHA256 重算）
5. Browser gate：`verify-browser-cutover.mjs`（desktop + mobile_390 双视口：练习/收藏/反馈/排行/工坊）
6. Platform Core 真实邮件验收（30 天 Core Session、8 小时 Console Session、OAuth 回调、撤销传播；`verify-platform-core-cutover.py`）
7. 激活写入：`switch-cutover-release.sh`（dry-run 评审先行——276 行，建议先跑一次演练）；`EXPECTED_WRITES_ENABLED=true` 阶段验证
8. 切后运维：FastAPI 停服脱离 Nginx → 只读冷备 7 天 → 人工批准移除；双库备份保留 30 天；`VITE_QUIZCRAFT_GO_WRITES=1` 构建发布 quizcraft web-app；排行榜切到 Go rankings（legacy 快照只读保留）
9. 修复 `products/quizcraft/README.md` 的旧开关名（`VITE_QUIZCRAFT_GO_SHADOW` → `VITE_QUIZCRAFT_GO_WRITES`）

**风险提示**：portal 直读把 `quizcraft_ranking_settlement_events` 的 standings 当昵称展示（可能泄漏 user UUID）——切流前必须核对 settlement 行内容或改契约。

### M2 数据面迁移（最大硬骨头，按依赖排序）

1. **courses/materials 迁出 study 库**：迁入 library 独立库（`library_adapter_operations` 等表已就位，ADR-0020）；portal-api 停止直读 study 库（删 `internal/library/StudyDB` 的硬编码 SQL）；`/library` 全回归（浏览/详情/下载）
2. **ops 脚本脱钩**：`scripts/ops/import-henukit-materials.mjs` 及 activate/prepare/seal 配套脚本改写入 library 库
3. **services/library 兼容层裁决**：接线的 legacy 依赖（10 条 `/api/v1/admin/*` 路由）在数据迁移后移除 adapter；修启动矛盾（要么接线 `STUDY_LEGACY_API_URL` 让服务能起，要么迁移完成后删 fail-closed 检查）
4. **study 库 schema owner 移交**：47 张表逐一裁决——
   - 迁移：courses、materials、material_download_logs → library
   - 移交：users/email_verification_codes → platform-core；**memberships/membership_plans → account-portfolio（终身 VIP / 普通两档在用，先做「用户→会员」映射对账再切，不得丢权益）**；entitlement 相关 → account-portfolio；notifications → notice 或 platform-core
   - 冻结归档：orders/payment_records/payment_incidents → **EasyPay 为当前通道**，在途/历史订单核对后迁入 account-portfolio；wiki_*/blog_*/forum_*/moments/points_*/ai_*/reports/leaderboard_snapshots 等 → **已确认关闭**：只读快照归档，不迁移
   - 裁决：study 库 quiz_* 表（与 quizcraft 库关系）、schools/colleges/majors
5. **GORM AutoMigrate → 正式 migration**：把 47 张表固化为版本化 migration 交给新 owner（`docs/development/306-materials-secure-preparation.md:52` 已要求不再给 services/api 加 migration）
6. **新栈种子工具**：`scripts/seed` 目前只指向 legacy（`services/api/cmd/seed`、`import-materials`），M2 内为 portal-api/library 建新栈种子入口（或明确用 go-service 工具替代）

### M3 旧栈退役

> 2026-08-19 更新（服务器实测）：**旧栈已物理退役**——无 study-api/study-worker/quizcraft-api/quizcraft-web 容器，宿主机 8080 无监听，superhuazai.me 全站 301 到 henukit.cn 栈；本节降级为文档/代码层面的清理收尾。

1. **删 `apps/study-legacy-admin`**（新栈零引用，M0 后即可做；git 历史保留）
2. **归档 `apps/web`**：16 个路由面中 login/materials/courses/部分 me 子页已被 portal 替代；blog/forum/moments/wiki/search/users/workspace/packages 为冻结功能——路由与 19 个 e2e spec 一并归档（17 个未进默认 CI 的直接随功能归档）
3. **冻结 `services/api` + `services/worker`**（顺序：先完成 M2 的依赖解除，再停服；worker 的 AI 职责随 ai 功能冻结）
4. **清理**：根 `docker-compose.yml`/`docker-compose.dev.yml`（含 mysql）、`docker-compose.prod.example.yml`、`infra/nginx/final-review.conf.example`、`infra/docker/README.md` 旧栈段落
5. **package.json**：移除 `build:web`、`test:web-ui`、`build:study-legacy-admin`（或从 `build:all`/`lint:all` 摘除）；root name 旧品牌名清理
6. **文档终扫**：deployment.md/internal-smoke.md/wechat-pay-native.md 的 `go run ./cmd/*` 流程、checklist 的 Platform Worker 旧名等

### M4 新栈生产验收（GO）

1. **接线补齐（以服务器 `/opt/henukit/.env.henukit` 为准——repo 的 `.env.henukit.prod` 已落后，`henukit-local-deploy.md` §7 记录 2026-08-16 已补 career 等变量，需把服务器最新 env 同步回 repo 示例并核对 prebuilt `:?` 强制变量契约）**：
   - 缺失项核对：`ACCOUNT_PORTFOLIO_*`（**硬必须**——VIP 在用 + EasyPay 是当前支付通道，首次 8-to-9 cutover 未执行）、`LIBRARY_OSS_*`/`LIBRARY_DOWNLOAD_*`（library OSS 下载未接线）、`FOOD_MCP_ACCESS_TOKEN`（food-mcp 未接线）、`NOTICE_DELIVERY_*`、`QUIZZFCRAFT_CORE_URL`（暗命令）、`PORTAL_ENABLE_QUIZCRAFT_*`、`STUDY_LEGACY_*`（library 启动必需）
   - 逐项确认 20 服务（17 自建镜像：9 baseline + 8 conditional）在生产 compose 中真实启动且 ready；健康检查覆盖补齐
2. **CI 补齐**：food-mcp、career-mcp 当前无任何 workflow；前端烘焙 flag（`VITE_QUIZCRAFT_GO_WRITES` 等）与网关开关不一致问题一并修
3. **域名/证书矩阵**：henukit.cn（已有）、console.henukit.cn（GO 08-03，剩真实邮箱登录 Smoke + 观察窗口清理）、study.henukit.cn / quiz.henukit.cn / account.henukit.cn（**均无 vhost 未落地**，新子域需先过边界批准）、`/console-api/` 观察期路由清理、`/mcp/` 路由缺仓库模板（补 nginx 模板）
4. **数据迁移入口**：quizcraft 库需先由 go-service 迁移建表（compose 无迁移入口是刻意设计，但 M1 前必须确认生产库状态）；platform-core 迁移（000017/000018 等）执行；资料激活/OSS 管线；**新栈种子入口缺失**——`scripts/seed` 只指向 legacy `services/api/cmd/seed` 与 `import-materials`，M2 需补新栈种子工具；portal-api 的 portal 库迁移只有 up 无 down（补 down 或注明不可回滚）
5. **支付/会员落地**（D3/D4 已定）：EasyPay 通道在 account-portfolio 接线并开启（`ACCOUNT_PORTFOLIO_EASYPAY_ENABLED` 等），历史订单迁入；VIP 权益（终身/普通）迁移后对账验证（现有 VIP 用户在 Portal/Console 可见正确档位）
6. **种子数据与回滚演练**：备份恢复演练、`PRODUCTION_RELEASE_CHECKLIST` 全项回填（含真实 `@henu.edu.cn` 注册 Smoke）、审计报告更新为 GO；**验收 smoke 主域口径统一**（`henukit-artifact-deployment.md` 用 superhuazai.me vs `henukit-local-deploy.md` 用 henukit.cn，需定一个为准）
7. **🔐 密钥轮换**：`.env.henukit.prod` 含疑似真实 SMTP 密码与 MCP token（gitignored 未入库，但文件在本地工作树，且文件头注明"勿放真实密钥"）——上线前统一轮换，服务器侧 root-owned 文件权限（0600）核验
8. **核验 prebuilt 契约矛盾**：`docker-compose.henukit.prebuilt.yml` 强制 `QUIZCRAFT_CORE_URL`/`ACCOUNT_PORTFOLIO_*`/`LIBRARY_OSS_*` 等 `:?` 变量，而 repo env 未设——说明当前生产要么不用 prebuilt 契约、要么服务器 env 与 repo 严重不一致；上服务器核验定论并统一（`henukit-local-deploy.md:301-312` 有验证方法）

## 2. 需要你拍板的决策项

| # | 决策 | 选项 | 影响 |
|---|---|---|---|
| D1 | Portal 刷题读路径最终形态 | A 走 Gateway→Go core / B portal-api 直读转正 | ✅ **已确认（2026-08-17）：方案 A** —— 已落 ADR [`0036-portal-practice-read-path-owner-go-core`](../adr/0036-portal-practice-read-path-owner-go-core.md)（读走 Gateway→Go core，portal-api 直读切流后删除）；研究见 `docs/research/practice-read-path-ownership.md` |
| D2 | 冻结功能去留 | 归档 / 迁移 / 直接关闭（blog、forum、moments、wiki、points、AI、search、user 主页、workspace、packages） | ✅ **已确认（2026-08-17）：没人用 → 直接关闭**；数据只读快照归档，不迁移 |
| D3 | 支付通道 | 微信 Native / EasyPay / 冻结订单 | ✅ **已确认（2026-08-17）：当前走 EasyPay**，微信 Native 不接入；account-portfolio EasyPay 通道需生产接线并开启 |
| D4 | 会员/积分（VIP）是否在 V1 启用 | 启用 / 保持隐藏候选 | ✅ **已确认（2026-08-17）：会员在用（终身 VIP + 普通两档）**；memberships 数据迁入 account-portfolio 且不丢权益；points 随 D2 关闭 |
| D5 | 旧身份绑定 | ADR-0013 已定：旧 QuizCraft 身份不自动映射 | ✅ **已确认（2026-08-17）：不做旧身份映射** |
| D6 | 服务器核验执行人 | 你上服务器回填 M0 清单（或授权脚本自动采集） | ✅ **Runbook 已交付**：`docs/operations/PRODUCTION_VERIFY_RUNBOOK.md`（520 行 11 节，预计 40 分钟，root + 只读库账户） |

## 3. 执行顺序与依赖

```
M0（文档+裁决 D2–D6 的摸底） → D1 裁决
  → M1 QuizCraft 切流（依赖 D1、服务器核验）
  → M2 数据迁移（依赖服务器核验、D2/D3/D4）
  → M3 旧栈退役（依赖 M2 完成）
  → M4 生产验收（依赖 M1–M3 + 全部核验）
```

## 4. 成功标准（Done 定义）

- [x] quizcraft web-app 生产流量 100% 走 Go 服务，FastAPI 已移除，排行榜/收藏/反馈全契约化 —— **已达成（服务器实测 2026-08-19）**：Go core 接管 catalog/rankings 与写路径、FastAPI 停服；收尾项（排行结算、portal-api 幽灵路径、cutover 证据文档）见 M1 状态更新
- [ ] portal `/library` 数据全部来自 library 服务（或新 owner），portal-api 不再直读 study 库
- [ ] study 库 47 张表全部有明确 owner 或已归档；services/api、services/worker、apps/web、apps/study-legacy-admin 从工作树移除（git 历史保留）
- [ ] `docker-compose.henukit.yml` 20 服务全量生产运行，`PRODUCTION_RELEASE_CHECKLIST` 全项 GO
- [ ] modules.yaml / 执行规格 / 镜像清单 / CONTEXT-MAP / 现状文档四方一致，无漂移

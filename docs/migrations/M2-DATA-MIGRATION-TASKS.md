# M2 数据面迁移 · 可执行任务清单

> 上游方案：[`ALL_NEW_STACK_CUTOVER.md`](./ALL_NEW_STACK_CUTOVER.md)（M2 节）
> 现状基线：[`CURRENT_PRODUCTION_STATE.md`](../operations/CURRENT_PRODUCTION_STATE.md)（2026-08-19 服务器实测）
> 已确认决策（2026-08-17，直接引用，不再重议）：
> - **D2**：冻结功能（blog/forum/moments/wiki/points/ai/search/user 主页/workspace/packages）**直接关闭**，数据只读快照归档、不迁移。
> - **D3**：支付通道 **EasyPay**（account-portfolio 接线），微信 Native 不接入；历史/在途订单核对后处理。
> - **D4**：会员**在用**（终身 VIP + 普通两档），memberships 迁入 account-portfolio **不丢权益**；points 随 D2 关闭。
>
> 任务粒度：**半天到一天可完成**；凡标注「裁决/盘点」的任务产出必须是文档/ADR 或核验回填，不写业务数据。
> 本清单只覆盖 M2 数据面；旧栈删除（M3）、生产验收（M4）不在此列，但每任务注明交接口。

## 0. 现状事实基线（本清单依据）

| # | 事实 | 证据 |
|---|---|---|
| F1 | study 库：courses **24**、materials **715**（Portal `/library` 数据源）；schema owner 仍是 services/api 的 GORM AutoMigrate | CURRENT_PRODUCTION_STATE §5；ALL_NEW_STACK_CUTOVER M2-4 |
| F2 | 双 Postgres：宿主机 postgres16（quizcraft/quizcraft_v2）+ docker postgres17 容器 9 库（study/platform/account_portfolio/quizcraft/notice/library/food/career/portal）；study 与 library 同实例不同库 | CURRENT_PRODUCTION_STATE §2 |
| F3 | library 库 6 张表就位：`library_public_releases`、`library_public_material_snapshots`、`library_public_release_activation_events`、`library_download_start_events`、`library_adapter_operations`、`library_adapter_audit_events`（迁移 000001–000003） | services/library/db/migrations |
| F4 | portal-api 直读 study 库：`internal/library/db.go`（StudyDB，3 条硬编码 SQL）+ `internal/httpapi/router.go` 接线（L35-47、L118-119、L173-260）；live 模式 `STUDY_DATABASE_URL` 缺失即启动失败 | 代码 + db/connect.go |
| F5 | services/library 兼容层 adapter 经 HTTP 代理 10 条 legacy admin 写路由（course/material 增改删、submission 通过/驳回、correction 处理/驳回）+ 5 条 workspace 读路径；`STUDY_LEGACY_API_URL` 缺失 → fail-closed 启动失败（ADR-0020）；生产实测该 URL=`http://127.0.0.1:1`（死端口，服务能起但适配断链） | services/library/adapter.go、cmd/server/main.go、CURRENT_PRODUCTION_STATE §4 |
| F6 | 资料 OSS 管线：`prepare → seal → complete OSS publish → activate`（publish 原子落 complete release commit；activate 经 `build-henukit-library-activation-bundle.mjs` + `services/library/cmd/activate-public-release` 写 **library 库**快照/激活表）；`import-henukit-materials.mjs` 生成 SQL 写 **study 库**（courses/materials/schools/colleges/majors） | scripts/ops/*.mjs、scripts/ops/henukit-materials-sync.sh、docs/development/306-materials-secure-preparation.md |
| F7 | account_portfolio 库 17 张表（migrations 000001–000008 枚举；生产文档口径 18，T4-0 校正）：accounts/points/memberships/point_ledger/membership_orders/notifications/tickets/ticket_messages/service_nonces/ticket_events/command_idempotency/membership_events/payment_order_intents/payment_facts/payment_audits/point_adjustment_audits/membership_order_refunds | services/account-portfolio/db/migrations |
| F8 | `account_portfolio_memberships.plan` 仅 `('free','lifetime')`，**无 expires_at 列**；membership_events from_plan/to_plan 仅 free↔lifetime；membership_orders.plan 仅 `'lifetime'`、amount_cents 固定 990。**普通档（有时效）无承接结构** —— T4 必须先扩展 | 000001/000003/000004 迁移 |
| F9 | study 侧会员：`memberships`（user_id, plan_code, status, source, expires_at）+ `membership_plans`（code/name/price_fen/points_cost/duration_days/benefits/status）；plan 排序 `tier1→1、tier2→2`；来源 `manual_admin` / `points_redeem` | services/api/internal/platform/model/models.go:460-478、internal/member/handler.go |
| F10 | 用户映射通道：platform-core `email_identities.email_lookup_hash = HMAC-SHA256(verificationKey, "henukit-verification:email" \| 0x00 \| normalized_email)`；study `users.email` 明文唯一。**映射必须拿到 verificationKey 才能离线算 hash**（服务器侧执行） | services/platform-core/internal/operatorbootstrap/service.go:107、000011_email_identity_login.up.sql |
| F11 | GORM 模型清单 **45 张表**（`internal/platform/model/models.go:562-579` AllModels）；方案文档写「47 张」为估算，T5-0 以生产 `\dt` 校正 | 代码 + ALL_NEW_STACK_CUTOVER M2-4 |
| F12 | `henukit.modules.yaml` data_owners 现状：courses/materials/material_files/material_downloads→study_api；memberships/entitlement_grants/notifications→platform_core（manifest 目标态，实现仍在 study 库）；quizcraft 五表→quizcraft_product | henukit.modules.yaml:244-265 |
| F13 | `scripts/seed` 只指向 legacy：`services/api/cmd/seed` + `cmd/import-materials` | scripts/seed/README.md |

---

## 1. 执行顺序图（依赖）

```
                        ┌──────────────────────────────────────────────────────────┐
                        │ T5-0 生产 study 库表清单核验（半天，只读）                  │
                        └───────────────┬──────────────────────────────────────────┘
                                        ▼
   ┌────────────────────────────────────┴───────────────────────────────────────────┐
   │ T1-0 目标表形态裁决 + T1-1 library 建表（000004）                                │
   └───────────────────────────────┬────────────────────────────────────────────────┘
                                   ▼
   T1-2 迁移脚本（幂等/可重放/对账） ──→ T1-3 portal-api 切读 library ──→ T1-4 /library 回归
        ▲                                   │
        │                                   ▼
   T2-0 导入脚本改写 library ─→ T2-1 sync 脱钩 ─→ T2-2 activate/seal/prepare 核验 ─→ T2-3 回归
                                                    （T2 与 T1-3 独立可并行）
   T4-0 会员/订单盘点 ──→ T4-1 AP 迁移扩展 ──→ T4-2 用户→会员映射导入 ──→ T4-3 权益对账验收
                              （T4 全程独立并行；T4-4 订单核对与 T5 联动）
   T5-1 47 表裁决表（依赖 T5-0）──→ T5-2 冻结功能快照归档 ──→（供 M3 使用）
   T6（依赖 T1/T4 的表落地 + T5 裁决）→ 固化 DDL 到各 owner 迁移 + 执行流程
   T7 新栈种子（独立，可与 T1 并行）
   T3-0 兼容层裁决（文档）──（T1+T2 完成后触发）──→ T3-1 移除 adapter ─→ T3-2 删 fail-closed ─→ T3-3 测试回归
```

关键路径：`T5-0 → T1-0 → T1-1 → T1-2 → T1-3 → T1-4 → T3 触发`。
并行泳道：T2、T4、T5-1/5-2、T6、T7 与关键路径无硬依赖处可并跑。
硬门禁：**T3 移除 adapter 必须在 T1-4 与 T2-3 回归通过后**；**T4-2 写库必须在 T4-1 迁移上库后**。

---

## 2. T1 courses/materials 数据迁入 library 库

> 目标：Portal `/library` 数据源从 study 库切换为 library 库；portal-api 停止直读 study 库。

### T1-0 目标表形态裁决 + 生产数据盘点（0.5–1 天，只读）

- **目标**：定 library 库「课程/资料目录」的落表形态，产出 ADR；盘点 study 侧真实数据分布。
- **涉及文件/服务/库**：`docs/adr/`（新增 00xx-library-owns-course-material-catalog）；服务器只读核验（Runbook §5 模式）。
- **数据与依赖**：读 study 库 `courses`/`materials` 全部状态分布（published/archived、access_level free/login_required/paid、slides 有无、storage_key 空否、sha256 缺失率、duplicate storage_key）；读 library 库 6 表现状（已有 active release 快照行数）。
- **裁决内容（默认推荐）**：现有 6 张表是**公开 OSS release 快照/账本**语义（append-only、绑定 release_id），**不承接**完整课程/资料目录（draft/paid/归档态、课程组织维度）。推荐**新增 owner 目录表 `library_courses` + `library_materials`**（迁移 000004），6 张既有表用途不变；Portal 读目录表，OSS 下载继续走快照表。备选（不推荐）：复用快照表承载全部目录 —— 违反 append-only 与 release 绑定约束。
- **验收标准**：
  - [ ] ADR 落文：新表形态、字段草案、与 6 张既有表的关系、与 ADR-0027/0029（公开免费 OSS 面）的边界。
  - [ ] 盘点回填 `CURRENT_PRODUCTION_STATE.md`：courses 24 的状态分布；materials 715 的 published 子集数、free 子集数、slides 非空数、storage_key 空数、重复 storage_key 数。
- **风险/回滚**：无写操作，零风险；若盘点发现 published≠715，后续任务验收口径以盘点数为准。

### T1-1 library 库新增目录表迁移 000004（0.5–1 天）

- **目标**：`library_courses` / `library_materials` 建表（版本化 SQL，幂等，可 down）。
- **涉及文件/服务/库**：`services/library/db/migrations/000004_owner_catalog.{up,down}.sql`（对照既有 000001–000003 风格：CHECK 约束、部分唯一索引、down 可逆）。
- **数据与依赖**：字段草案见下「字段映射」；不依赖业务数据。
- **验收标准**：
  - [ ] up/down 各执行两次结果一致（幂等）；CI 空库 up、已有 baseline up、重复执行均通过（对齐 database-migration-spec §2）。
  - [ ] `library_materials` 含局部唯一索引 `materials_storage_key_active_idx (storage_key) WHERE deleted_at IS NULL` 等价物（对齐 306-A02 前置检查契约，供 T2 复用）。
  - [ ] 表结构支持 T1-2 全字段写入且保留 study 侧原 UUID（Portal 详情 URL 不变）。
- **风险/回滚**：新表无消费方，down 即回滚；无锁竞争（同库新建表）。

### T1-2 幂等可重放迁移脚本 + 字段映射（1 天）

- **目标**：写迁移脚本（读 study → 写 library），幂等、可重放、带对账输出；产出字段映射表。
- **涉及文件/服务/库**：新增 `scripts/ops/migrate-study-to-library.mjs`（Node，对齐 `import-henukit-materials.mjs` 形态：schema 前置检查 + BEGIN/COMMIT + upsert + 统计到 stderr；跨库用只读 study 连接读 + psql 写 library）；或 `services/library/cmd/import-study-catalog`（Go）。**推荐 .mjs**（与 ops 脚本链一致，审查面小）。
- **数据与依赖**：study 只读连接（`STUDY_DATABASE_URL` 只读账户）+ library 写连接；依赖 T1-1 表结构与 T1-0 盘点数。
- **字段映射（对照 `portal-api/internal/library/db.go` 查询列与 model）**：

  | study `courses` | library_courses | 说明 |
  |---|---|---|
  | id | id (uuid) | 原值保留，Portal 课程卡片 ID 不变 |
  | name | name / subject | subject=name（db.go L23 语义） |
  | status/deleted_at | status/deleted_at | 仅迁 published 且未删；其余按 state 列归档标记 |
  | school_id/college_id/major_id/grade/slug/description/exam_scope | 同名列（可空） | T5-3 裁决 schools/colleges/majors 去向后再定是否外键 |

  | study `materials` | library_materials | 说明 |
  |---|---|---|
  | id | id (uuid) | 原值保留（详情 URL `GET /api/v1/library/materials/{id}`） |
  | course_id | course_id | 指向 library_courses 新 id（映射表） |
  | type | type | `mapMaterialType` 后值：note/exam/mock/path/lab/slides（db.go:207-224） |
  | title | title | 原值 |
  | description | intro | db.go 以 description 当 intro |
  | access_level | access_level | free/login_required/paid 原样保留（Portal 价格推导用） |
  | file_size / storage_key / file_name | 同名列 | 原样 |
  | slides (jsonb) | slides (jsonb) | 原样（detail 用） |
  | sha256 / reviewed_at / review_reason | 同名列 | 可空 |
  | created_at/updated_at/deleted_at | 同名列 | 原样保留审计 |
  | — | download_available / price / rating 派生列 | **不落库**：由查询层派生（free+storage_key→可下载；free→0 否则 50），保持 db.go 语义 |

- **迁移脚本形态要求**（对齐 306-A02 / database-migration-spec §3）：
  - 前置检查（读库列/索引就位，缺则拒绝）；dry-run 输出 `create/link/conflict/skip`；批量分批 + 批次记录 + 异常清单；同 storage_key 幂等 upsert（`ON CONFLICT (storage_key) WHERE deleted_at IS NULL`）；**不改、不删 study 源数据**；对账段输出两库计数与 hash 比对。
- **验收标准**：
  - [ ] 连跑两次，library 行数不变（幂等）；断点续跑不重复。
  - [ ] 对账：library_courses=study published 课程数（≤24）；library_materials=study published materials 数；抽查 10 条字段级 hash 一致；slides 非空集合一致。
  - [ ] 产物可评审：dry-run 报告 + 异常清单 + 对账 JSON（含批次/耗时）。
- **风险/回滚**：library 新表无消费方，重跑覆盖安全；源库零改动；迁移期间 study 持续可读（无需停写，M2 阶段 study 已是只读事实源）。

### T1-3 portal-api 停止直读 study 库（0.5–1 天）

- **目标**：删除 `internal/library/db.go` 的 StudyDB 硬编码 SQL；librarySource 改读 library 库。
- **涉及文件/服务/库**：`services/portal-api/internal/library/db.go`（删/改 StudyDB 三查询为 LibraryDB，SQL 列对齐 T1-2 新表）；`services/portal-api/internal/httpapi/router.go`（L35-47 连库、L118-119 类型、L173-260 三 handler 的 503 文案与空判）；`services/portal-api/internal/library/material_public_test.go`（契约测试对齐）；compose：`docker-compose.henukit.yml` L135 `STUDY_DATABASE_URL` → `LIBRARY_DATABASE_URL`（`postgres:5432/library`）；`.env.henukit.prod` 同步。
- **数据与依赖**：依赖 T1-1/T1-2（表与数据就位）；库内无 `slides` 列探测逻辑（db.go:134-138）可删。
- **两种切读形态（T1-0 ADR 一并裁决，默认 A）**：A（推荐，临时）portal-api 直读 library 库目录表，改动最小、可灰度（env 开关）；B portal-api 调 library 服务公开目录路由（`/api/v1/library/...`），更贴合 ADR-0027/0029 owner 语义但引入服务间调用与凭据，留 M4 收口。
- **验收标准**：
  - [ ] live 模式启动不再要求 `STUDY_DATABASE_URL`；grep 全仓无 `internal/library/StudyDB` 残留。
  - [ ] `/api/v1/library/courses`、`/materials`、`/materials/{id}` 与切读前**逐字段等价**（同 24/同 published 数/同详情字段含 slides）；筛选（type/subject/q）行为不变；404/503 语义不变。
  - [ ] 端口/连接核验：进程无 study 库连接（`pg_stat_activity` 只读核验）。
- **风险/回滚**：灰度开关（保留 `STUDY_DATABASE_URL` 分支一版或环境变量二态）可秒回；library 表为迁移镜像，回退无需回灌。

### T1-4 /library 回归验收（0.5–1 天）

- **目标**：Portal 浏览/详情/下载全链路回归，作为 T3 触发门禁之一。
- **涉及文件/服务/库**：`apps/portal` /library 页面（浏览器回归）；`services/library/tests/`（download/activation 用例引用）；`docs/operations/PRODUCTION_RELEASE_CHECKLIST.md` 回填。
- **数据与依赖**：T1-3 已切读；library OSS 下载链路（快照表 + 签名 grant）不受本组改动影响，需连测。
- **验收标准**：
  - [ ] 浏览：课程列表 24 条、资料列表 published 数、分页/筛选/搜索与切读前一致（截图/计数对比）。
  - [ ] 详情：字段（title/intro/type/slides/fileSize/downloadAvailable）逐项与 study 源数据核对。
  - [ ] 下载：free+storage_key 资料经 gateway→library 签名 grant→OSS 下载 200；计数写入 `library_download_start_events`（append-only 不回归）。
  - [ ] 空态/异常：无数据、404、503 文案不泄漏内部细节（Public-ready Copy 轴）。
  - [ ] 自动化：portal-api 单测 + 一条 curl 链路脚本入 CI（或 ops 脚本）。
- **风险/回滚**：纯验证；发现差异回 T1-3 开关回退。

### T1-5 study 侧目录写路径冻结确认（0.5 天，可选并入 T2）

- **目标**：确认 M2 后 study 库 courses/materials 无业务写方（只剩只读快照用途），为 T5/T6 打底。
- **涉及**：`scripts/ops/henukit-materials-sync.sh`（T2 脱钩后）、deploy-webhook `materials-run` 链、`services/api` 相关 cmd。
- **验收标准**：只读核验 study 库 `materials` 最近 updated_at 无新写入；代码 grep 无剩余写 study 目录的脚本。
- **风险/回滚**：无写操作。

---

## 3. T2 ops 材料脚本脱钩（与 T1 并行）

> 目标：`import-henukit-materials.mjs` 及 prepare/seal/activate 配套链从写 study 库改为写 library 库。

### T2-0 导入脚本改目标库（1 天）

- **目标**：`scripts/ops/import-henukit-materials.mjs` 生成的 SQL 改为写入 library 库目录表。
- **涉及文件**：`scripts/ops/import-henukit-materials.mjs`（SQL 生成段：schools/colleges/majors 段落随 T5-3 裁决调整或删除；courses/materials INSERT/UPDATE 指向 `library_courses`/`library_materials`；schema 前置检查改为检查 library 侧列与 `materials_storage_key_active_idx` 等价索引）。
- **数据与依赖**：T1-1 表结构与索引；`--release-id` 语义不变（OSS 路径 `releases/{id}/...`）。
- **验收标准**：
  - [ ] 前置检查在缺表/缺列/缺索引时于 BEGIN 前拒绝（306-A02 契约不变，目标库换成 library）。
  - [ ] 输出 SQL 对 library 库执行两遍幂等；统计 JSON（subjects/imported/archived/duplicate_sha256）正确。
  - [ ] 不再生成任何写 study 库的语句（SQL 文本断言入测试）。
- **风险/回滚**：与 T1-2 共用目标表，两者必须同一份字段映射（以 T1-2 映射表为唯一事实源，脚本间禁止分叉）；回滚＝重新生成旧版 SQL 指向 study（保留一版旧脚本标签）。

### T2-1 sync 脚本脱钩（0.5 天）

- **目标**：`henukit-materials-sync.sh` 第 3 段 import 的数据库目标改为 library。
- **涉及文件**：`scripts/ops/henukit-materials-sync.sh`（`HENUKIT_MATERIALS_DATABASE_URL` 语义 → library 库，或改名 `HENUKIT_MATERIALS_LIBRARY_DATABASE_URL`；`import_sql()` 调用不变）。
- **验收标准**：本地全链路（sync-henukit-materials.sh → convert slides → import）落 library 库；`materials` 计数对账 715 口径一致；study 库无新写入。
- **风险/回滚**：改环境变量名需同步 `henukit-local-deploy.md` / 部署文档；保留兼容读取旧变量名一版。

### T2-2 prepare/seal/complete-publish/activate 链核验（0.5 天）

- **目标**：确认 prepare/seal/complete publisher 无 DB 写入（纯文件/校验与 OSS 提交）；publisher 只有在全部对象固定版本核验完成后才落 root-owned complete commit；activate 链已写 library 库、无需改动或仅补字段。
- **涉及文件**：`scripts/ops/prepare-henukit-materials.mjs`（核验无 DB 段）、`seal-henukit-materials.mjs`（同上）、`services/deploy-webhook/cmd/materials-oss-release` 与 `internal/materialsoss`（核验逐对象版本收据、完整 commit、失败/重放边界，且无 DB 写入）、`activate-henukit-materials.mjs`（`--importer` 目标已随 T2-0 切换；`--library-activator` → `build-henukit-library-activation-bundle.mjs` + `services/library/cmd/activate-public-release`，验证写 `library_public_*` 四表路径正确）。
- **验收标准**：一次完整 publish + activate dry-run 通过；缺失/不完整 OSS commit 时 activation 不启动；激活后 `library_public_releases` 恰好一条 active；snapshots 行数与 manifest 一致；**study 库无任何 DML**。
- **风险/回滚**：activate 已具备原子切换 + 回滚（激活上一受审 release），无新增风险。

### T2-3 回归 + 文档（0.5 天）

- **目标**：全链路回归并更新操作文档。
- **涉及文件**：`docs/development/306-materials-secure-preparation.md`（public seam 描述：import 目标库变更）、`docs/operations/henukit-materials-oss-release.md` / `henukit-materials-sync.md`、`docs/operations/PRODUCTION_VERIFY_RUNBOOK.md`（study 库写方检查口径）。
- **验收标准**：文档中「导入 study 库」表述全部更新；回归脚本（manifest→library 幂等）入 `scripts/ops/tests/`。
- **风险/回滚**：文档任务，无风险。

---

## 4. T3 services/library 兼容层裁决

> 背景：F5 —— 生产 `STUDY_LEGACY_API_URL=http://127.0.0.1:1`（死端口），服务能起但 legacy 适配断链；Console 资料运营面实际不可用（workspace partial/unavailable）。

### T3-0 兼容层裁决文档（0.5 天，T1-4+T2-3 通过后触发）

- **目标**：三选项正式裁决并落文档（ADR 或方案文档更新），给出触发条件。
- **选项**：
  - **A（推荐，M2 终态）**：数据迁移完成后**移除 adapter** 及其 10 条 legacy admin 路由依赖；Console 资料运营面改由 library 自身库目录表支撑；`STUDY_LEGACY_API_URL` / `STUDY_LEGACY_ADMIN_TOKEN` 从 compose 与 env 移除；删 fail-closed 启动检查（ADR-0020 作废或标注 superseded）。触发条件：T1-4、T2-3 回归通过，且 Console「资料库运营」模块验收（见 T3-3）。
  - **B**：修正 URL 暂留（把死端口改回可用的 legacy 端点）——**不推荐**：legacy services/api 已在退役路径上（M3），为已冻结接口维持活链路违背方向；仅在 A 因故推迟且运营必须在 Console 操作投稿/纠错时作为临时止血，且必须设删除期限。
  - **C**：保留但标注废弃（不改行为、不接线，服务继续以死端口启动、workspace 恒 degraded）——当前生产**事实上就是 C**；可作 A 落地前的过渡态，但不得作为终态（方案 M2-3 明确要求消除启动矛盾）。
- **涉及文件**：`docs/migrations/ALL_NEW_STACK_CUTOVER.md`（M2-3 标记决策）、`docs/adr/0020-library-owner-production-onboard.md`（更新或 supersede）、`services/library/CONTEXT.md`。
- **验收标准**：文档明确：选项、触发条件（T1-4/T2-3 门禁）、B/C 仅限过渡的期限条款、A 的执行任务拆分引用本清单 T3-1~T3-3。
- **风险/回滚**：文档任务；但决策延迟会阻塞 M3（旧栈退役依赖 adapter 移除）。

### T3-1 移除 adapter（1 天，仅选 A 时执行）

- **目标**：删除 legacy 代理代码，Console 运营面改读/写 library 自身库。
- **涉及文件**：`services/library/adapter.go`（legacyAdapter/legacyCommandRoute/filteredPayload/libraryAccessLevel）、`services/library/library.go`（Config.Legacy*、service.legacy、workspace/consoleSummary handler）、`services/library/operations.go`（command/runOperation/versionMatches 改查自身目录表）、`services/library/types.go`、`services/library/cmd/server/main.go`（legacyURL/legacyToken 必填校验移除）、`docker-compose.henukit.yml` L358-359。
- **数据与依赖**：T1 目录表已承载 courses/materials；**投稿/纠错/历史下载**随 D2 冻结 → Console 面相应区块返回关闭/空态（不造假数据）；`library_adapter_operations`/`library_adapter_audit_events` 表保留（历史账本只读）。
- **验收标准**：
  - [ ] 服务在无 `STUDY_LEGACY_*` env 下正常启动（fail-closed 解除）；healthz 200。
  - [ ] Console「资料库运营」：课程/资料列表来自自身库（数=library 目录表数）；投稿/纠错区块明确关闭文案（Public-ready Copy 轴）。
  - [ ] workspace 不再有 partial/unavailable 状态（只有 ok 或自身库错误）。
- **风险/回滚**：git 历史保留 adapter；若 Console 回归失败，回退到 C 态（恢复 env + 旧镜像），目录表数据不受影响。

### T3-2 删除 fail-closed 检查与依赖清理（0.5 天，并入 T3-1 或紧随）

- **目标**：清 `library.go`/`cmd/server/main.go` 的 legacy 必填校验；删 `adapter.go` 引用的测试夹具。
- **涉及文件**：`services/library/tests/library_test.go`（`newLegacyServer` 相关用例改造/删除，改自身库断言）、`services/library/CONTEXT.md`（Compatibility Adapter 词条移除/降级）、ADR-0020 更新。
- **验收标准**：`go test ./...` 全绿；无 legacy URL/token 引用的编译残留。
- **风险/回滚**：测试改造属删除性变更，与 T3-1 同一提交回滚。

### T3-3 Console 资料运营验收（0.5 天）

- **目标**：Console 侧端到端验收，作为 M3 前置之一。
- **涉及文件**：`apps/console` 资料运营模块、`services/console-gateway`（LIBRARY_API_URL 路由不变）。
- **验收标准**：Console 资料运营展示 library 目录表真实数据；与 Portal `/library` 数字一致；无 503/502 噪音日志。
- **风险/回滚**：验收类，无写操作。

---

## 5. T4 memberships 迁移（敏感，全程独立泳道）

> 硬性约束：**不丢权益**（D4）——迁移前后会员数/档位/到期日逐项对账；写库前必须 dry-run + 双读对账。
> 关键前置发现（F8/F9/F10）：目标结构缺「普通档」承接能力；用户映射必须用 verificationKey 离线算 email lookup hash。

### T4-0 会员/订单数据盘点（0.5–1 天，只读）

- **目标**：量化迁移对象与映射可行性，回填现状文档。
- **涉及服务/库**：study 库（memberships、membership_plans、users、orders、payment_records）、platform 库（email_identities、users）、account_portfolio 库（account_portfolio_memberships、account_portfolio_membership_orders、payment_*）。
- **数据与依赖**：服务器只读（Runbook §5/§8 模式）。
- **盘点清单**：
  - [ ] study memberships 行数、plan_code 分布（**核验 tier1/tier2 之外是否还有自定义 code**）、status 分布（active/revoked）、expires_at 空/非空分布、source 分布（manual_admin/points_redeem/其他）。
  - [ ] membership_plans 行数与字段（code/name/duration_days/price_fen）——确认「终身 VIP」与「普通」对应的 code。
  - [ ] study users 与 platform `email_identities` 的可映射数：归一化邮箱（lower/trim）在两侧的命中率、重复邮箱、缺失 verificationKey 时的可执行方案（在服务器上借助 platform-core 进程内函数或独立 Go helper）。
  - [ ] account_portfolio_memberships 现况（可能为 0）；account_portfolio_membership_orders / payment_order_intents / payment_facts 是否已有行（**EasyPay 是否已承接历史订单**——预期为 0，须确认）。
  - [ ] study orders 中 product_type 命中 VIP/membership 的 paid 订单数（D3 在途/历史核对输入）。
- **验收标准**：盘点表回填 `CURRENT_PRODUCTION_STATE.md` §5；给出映射覆盖率（可自动映射 % / manual_review 数）。
- **风险/回滚**：只读，零风险；若验证 key 不可得，T4-2 需改为平台侧 operator 流程（降级方案，工期 +0.5 天）。

### T4-1 account-portfolio 承接结构扩展（1 天）

- **目标**：让 account_portfolio 能表示「普通档（有时效）」与「终身档」，且保留审计。
- **涉及文件**：`services/account-portfolio/db/migrations/000009_membership_regular_tier.{up,down}.sql`。
- **数据与依赖**：依赖 T4-0 的档位盘点（确认 plan code 枚举）。
- **改动**：`account_portfolio_memberships` 增加 `expires_at timestamptz`（可空=终身）；plan CHECK 扩展 `('free','lifetime','regular')`（或按盘点命名）；`account_portfolio_membership_events` 的 from_plan/to_plan CHECK 同步扩展（free↔lifetime↔regular 合法迁移形状，注意 000003 约束）；`account_portfolio_membership_orders` 若需承接普通档购买再加 plan 值（M2 内只读历史，可不改，注明）。
- **验收标准**：
  - [ ] 迁移 up 在空库/已有 000008 库上幂等；down 可逆；迁移 guard：若 memberships 已含非 free 行则拒绝改约束（向前迁移必须显式）——对齐 000006 风格。
  - [ ] `tests/membership_test.go` 增补 regular 档 grant/revoke/expiry 用例。
- **风险/回滚**：约束扩展是结构性变更，回滚 = down 迁移（需先清 regular 行，guard 写明）；上线顺序必须在 T4-2 写库之前。

### T4-2 用户→会员映射导入工具（1 天）

- **目标**：幂等、可审计地把 study memberships 迁入 account_portfolio，不丢权益。
- **涉及文件**：新增 `scripts/ops/migrate-study-memberships.mjs`（或服务器侧 Go helper，因需要 verificationKey；推荐 .mjs 读 study + 调 platform-core 提供的一次性映射端点或进程内函数算 lookup hash）。
- **数据与依赖**：T4-0 映射方案 + T4-1 表结构。
- **导入规则**：
  - 每用户：`account_portfolio_accounts`（无则建）→ `account_portfolio_memberships`（plan 映射：终身档→`lifetime` 且 expires_at=NULL；普通档→`regular` 且 expires_at=原值；status=active 且未过期才迁，revoked/过期行进异常清单不丢）；source 统一 `legacy_migration`（memberships 表无 source CHECK，仅受 000004 `payment_source_shape` 约束——非 payment 来源无 payment_fact_id，合法）。
  - 每个成员一行 `account_portfolio_membership_events`（kind=grant、from='free'、to=目标档、actor=迁移执行 operator、reason 注明 legacy id/plan_code、idempotency_key=`legacy_membership:<study_membership_id>`）——审计链完整。**注意**：000004 对 events 的 source CHECK 仅允许 `'operator'|'payment'` 且带 shape 校验，000009（T4-1）必须把 source 扩展 `'legacy_migration'`（actor_user_id 必填）或将迁移事件标为 `'operator'` 来源；二选一在 T4-1 落定。
  - **用户映射**：study users.email 归一化 → `emailLookupHash(verificationKey, email)` → 匹配 platform `email_identities` → 取 platform user_id；未命中进 `manual_review` 清单（database-migration-spec §6：重复/异常邮箱不自动合并）。
  - **幂等**：全部写入带 idempotency_key/唯一约束；连跑两次结果一致；dry-run 输出 `create/link/conflict/skip/manual_review`。
- **验收标准**：
  - [ ] dry-run 报告与盘点覆盖率一致；异常清单非空时**不进入写库阶段**（fail-closed）。
  - [ ] 写库后：account_portfolio_memberships 行数 = 可映射 active 会员数；events 行数 = 导入行数；idempotency_key 唯一。
  - [ ] 重跑幂等（第二次 0 变更）。
- **风险/回滚**：写库前 pg_dump account_portfolio 快照（M2 例行）；回滚 = 快照恢复 + 删除导入行（脚本提供反向 dry-run）；**永不**修改 study 源表。

### T4-3 权益不丢验收（0.5–1 天）

- **目标**：迁移前后逐项对账，Portal/Console 可见正确档位。
- **涉及文件/服务**：`services/account-portfolio`（`/api/v1/account/summary`、`/api/v1/account/membership`、Console grants 页）、`apps/portal` 会员入口、`services/console-gateway`（account-portfolio 代理）。
- **验收标准**：
  - [ ] 对账表：迁移前 study 会员数/档位分布/到期日集合 与 迁移后 account_portfolio 完全一致（终身 N 人、普通 M 人、到期日逐条比对）。
  - [ ] 抽样用户登录 Portal：会员档位/到期日正确展示；Console 运营页可查该用户 grant 审计链。
  - [ ] EasyPay 订单状态核对（T4-4 联动）：已付 VIP 订单数 vs 已导入 lifetime 会员数一致（若历史订单存在）。
- **风险/回滚**：验收发现差异 → 冻结新写入（account-portfolio 会员入口可先关）→ 修复导入脚本重跑；权益数据以导入事件链可审计撤销/重授。

### T4-4 历史订单/在途订单核对（0.5 天，与 T5 联动）

- **目标**：D3 落地——EasyPay 为当前通道，历史订单去向定案。
- **涉及服务/库**：study orders/payment_records/payment_incidents、account_portfolio membership_orders/payment_*。
- **核对结论（默认推荐，以 T4-0 数据为准）**：
  - account_portfolio 订单表若为 0：确认 EasyPay 尚未承接任何订单（`ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=1` 但无下单链路数据）→ 历史 study 订单**只读快照归档**（随 T5 冻结），不导入 account_portfolio（shape 不符：study 订单含 material/package 购买，AP membership_orders 仅 lifetime 990）。
  - 若存在 paid 且对应会员的订单：按 T4-2/4-3 导入链补记，来源标 `legacy_payment`。
  - 在途（pending_payment/paying）订单：**不迁移**（wechat_native 通道已废弃，无回调可能），随订单冻结归档并注明。
- **验收标准**：核对结论写入 `CURRENT_PRODUCTION_STATE.md`；EasyPay 下单链路 smoke（新建一笔 membership_orders 走通）确认通道可用。
- **风险/回滚**：只读核对 + 新建 smoke 订单（可关闭/标记）。

### T4-5 切流后观察与回滚预案（0.5 天）

- **目标**：会员读路径正式切到 account-portfolio 后观察期与回滚。
- **涉及**：`henukit.modules.yaml` data_owners（memberships→account_portfolio）；`docs/operations/` 更新。
- **验收标准**：观察窗（建议 7 天）内无权益类工单；回滚预案文档化（快照恢复 + 关会员入口开关）。
- **风险/回滚**：见上。

---

## 6. T5 study 库表 owner 裁决表

### T5-0 生产表清单核验（0.5 天，只读）

- **目标**：以生产 `\dt` 校正表数口径（F11：模型 45 张 vs 方案「47」）。
- **涉及**：服务器只读核验 study 库全表清单 + 行数（Runbook §5 模式）。
- **验收标准**：表清单（含行数）回填本文档 §8 裁决表，45/47 口径差注明来源（可能含 GORM 附加索引表或估算误差）。
- **风险/回滚**：只读。

### T5-1 裁决表落文（0.5–1 天，产出见 §8）

- **目标**：45 张表逐表三分法裁决（迁移→library / 移交→account-portfolio|platform-core|notice / 冻结归档→只读快照），每表一行：裁决、目标 owner、数据与依赖、快照方式、执行顺序。
- **涉及文件**：本文档 §8（作为权威裁决表）+ `henukit.modules.yaml` data_owners 更新清单 + `CURRENT_PRODUCTION_STATE.md`。
- **验收标准**：§8 表每行有明确 owner 或归档；无「待定」残留（quiz_* 与 schools/colleges/majors 在 T5-3 定案）。
- **风险/回滚**：文档任务。

### T5-2 冻结功能只读快照归档（1 天）

- **目标**：D2 冻结表数据只读快照 + 恢复演练。
- **涉及文件**：新增 `scripts/ops/archive-study-tables.mjs`（或 bash，pg_dump `--table` 清单 → 快照目录 `docs/operations/archives/study-<date>/`，逐表数据 + sha256 清单 + 时间戳；只读账户）。
- **数据与依赖**：T5-0/5-1 冻结清单；磁盘注意（服务器 89% 已用 ⚠️——快照先估算体积，必要时先清旧镜像）。
- **验收标准**：快照完成 + sha256 校验；随机 3 表恢复演练（恢复到临时库）通过；快照清单与裁决表一一对应。
- **风险/回滚**：快照为一次性归档，不可重复——执行前 dry-run 体积；失败重跑需清半成品目录（脚本幂等化）。

### T5-3 专项裁决：quiz_* 与 schools/colleges/majors（0.5 天）

- **目标**：方案遗留的「裁决项」定案。
- **裁决内容（默认推荐）**：
  - **quiz_questions/quiz_options/quiz_attempts/quiz_answers/wrong_questions/weakness_reports**：与 quizcraft 库**无血缘**（quizcraft 有自己的 question_banks/quizcraft_v2 契约表，M1 已切流，F2）。study quiz_* 是旧站刷题数据，**冻结归档**（快照），不并入 quizcraft；如生产行数为 0（预期），裁决表注明「空表仅归档 schema」。
  - **schools/colleges/majors**：是 courses 的组织维度且导入脚本依赖其 upsert 链（F6）。**随 courses 迁入 library**（library_courses 保留 school_id/college_id/major_id 可空外键或去外键纯字段）；若 T1-0 裁决 library 新表不需要组织维度，则改判冻结归档并在 import 脚本删除该段落（T2-0 联动）。
- **涉及文件**：`docs/adr/`（quiz_* 归档 + 组织维度去向）、T1/T2 任务联动项。
- **验收标准**：§8 表无待定行；T1/T2 字段映射与 import 脚本与裁决一致。
- **风险/回滚**：文档 + 联动代码；两处裁决必须同一提交内一致。

---

## 7. T6 schema owner 移交（GORM AutoMigrate → 正式 migration）

### T6-0 现状盘点（0.5 天）

- **目标**：列出 AutoMigrate 全部触达点与生产约束。
- **涉及文件**：`services/api/pkg/database/database.go:18-19`、`cmd/server/main.go:33-38`、`cmd/seed/main.go:25`、`cmd/import-materials/main.go:40-41`、`internal/preflight/preflight.go:33`（`AUTO_MIGRATE=false` 生产门禁）、tests/auth_test.go:126。
- **验收标准**：盘点表列出每个触达点、触发条件、生产状态；确认生产 `AUTO_MIGRATE=false`。
- **风险/回滚**：只读。

### T6-1 固化 47/45 表 DDL 到正式 migration（1 天）

- **目标**：每张表的 DDL 从「GORM 运行时推导」固化为版本化 SQL 文件，交给新 owner。
- **落地方式**：
  - **迁移组**（courses/materials→library 已由 T1-1 承担；memberships→account-portfolio 由 T4-1 承担；users/email_verification_codes→platform-core）：以 `pg_dump --schema-only` 摘取源表 DDL 为基线，逐列对照 GORM 模型与目标库风格改写，落入目标服务 `db/migrations/`（对齐 database-migration-spec §2：前置检查/up/down/验证 SQL）。
  - **冻结归档组**：`pg_dump --schema-only` 全库 → `docs/migrations/study-archive/0001_study_frozen_schema.sql`（每表注释标注 §8 裁决），不建业务表。
- **数据与依赖**：T1-1/T4-1 已覆盖的表不重复建；T5 裁决定分组。
- **验收标准**：migration 文件与生产 schema diff 为零（CI 对比脚本）；services/api 不再新增 migration（306-A02 约束满足）；冻结 schema 文件含逐表裁决注释。
- **风险/回滚**：DDL 固化只落文件、不执行（执行在 T6-2）；diff 工具失败时以 pg_dump 哈希比对兜底。

### T6-2 执行人/流程与 CI 门禁（0.5–1 天）

- **目标**：明确「由谁、何时、怎么执行」迁移，与 M4 部署流程对接。
- **落地**：沿用生产既有执行通道：部署 helper `apply_owner_migrations`（library/account-portfolio 迁移随镜像发布）；platform-core 迁移按 `HENUKIT_PLATFORM_MIGRATIONS` 显式应用（M4 §4 已确认）；执行人 = 服务器 root + 发布批准（`activate-henukit-release.sh` 同款批准流）；CI 增加空库 up / baseline up / down 验证（对齐 database-migration-spec §2 的 CI 要求）。
- **验收标准**：迁移执行 Runbook 段落（谁/何时/门禁/回滚）落文 `docs/operations/`；CI 全绿。
- **风险/回滚**：迁移执行属 M4 范围的生产动作，本任务只固化流程与门禁，不实际执行。

### T6-3 owner 标记同步（0.5 天）

- **目标**：`henukit.modules.yaml` data_owners 与裁决一致（F12）。
- **涉及文件**：`henukit.modules.yaml`：courses/materials/material_files/material_downloads→library；memberships→account_portfolio；entitlement_grants→account_portfolio（或随 material_access_grants 冻结而删除）；notifications→notice；schools/colleges/majors→library（T5-3 定）；quizcraft 五表保持。
- **验收标准**：data_owners 与 §8 裁决表逐行一致；`CONTEXT-MAP.md` 同步。
- **风险/回滚**：配置文档。

---

## 8. T7 新栈种子工具

### T7-0 现状盘点（0.5 天）

- **目标**：确认 scripts/seed 只指 legacy（F13）。
- **验收标准**：盘点表：`scripts/seed/README.md`、`services/api/cmd/seed`、`services/api/cmd/import-materials` 的职责与依赖（LOCAL_UPLOAD_DIR 等）。
- **风险/回滚**：只读。

### T7-1 新栈种子入口设计 + 实现（1 天）

- **目标**：给 portal-api/library 等新栈服务建种子入口。
- **建议（推荐 A+B 组合）**：
  - **A（主体）**：各 owner 服务自带 `cmd/seed`（Go，与 go-service 工具链一致）：`services/library/cmd/seed`（插 library_courses/library_materials 演示数据，fixture 定义与 T1-2 映射表同源）；`services/account-portfolio/cmd/seed`（两档会员 + 订单 smoke）；platform-core 已有 `cmd/grant-initial-operator`（身份种子沿用）。portal-api 是读端，不建种子。
  - **B（壳）**：`scripts/seed` 升级为统一编排入口（shell/mjs）：`scripts/seed henukit` 依序调用各服务 cmd/seed（带库 URL 参数、幂等、dry-run 说明）；`scripts/seed legacy` 保留指向旧 cmd 的兼容路径（M3 前可用）。
- **数据与依赖**：依赖 T1-1 表结构；种子与迁移脚本共用 fixture 校验。
- **验收标准**：`scripts/seed henukit` 在本地 compose 一键造出 library 目录 + account 会员种子；重跑幂等；README 更新。
- **风险/回滚**：种子只写本地/演示库（明确禁止生产执行标记），无生产风险。

### T7-2 文档同步（0.5 天）

- **目标**：`scripts/seed/README.md`、`henukit-local-deploy.md`（种子段）、`ALL_NEW_STACK_CUTOVER.md` M2-6 标记完成。
- **验收标准**：README 不再只指 legacy；文档与实现一致。
- **风险/回滚**：文档。

---

## 9. 总依赖关系表

| 任务 | 直接前置 | 提供物（下游消费） | 并行可跑 |
|---|---|---|---|
| T1-0 | T5-0（盘点口径） | ADR、盘点数 | — |
| T1-1 | T1-0 | 000004 迁移（目录表） | — |
| T1-2 | T1-1 | 迁移脚本、字段映射表 | T2-0（共用映射表） |
| T1-3 | T1-2 | portal-api 切读 | — |
| T1-4 | T1-3 | /library 回归门禁（T3 触发） | — |
| T1-5 | T2-3 | study 写路径冻结证据 | — |
| T2-0 | T1-1 | import 脚本改 library | T1-2 |
| T2-1 | T2-0 | sync 脱钩 | — |
| T2-2 | T2-0 | activate 链核验 | — |
| T2-3 | T2-1/T2-2 | 回归门禁（T3 触发） | — |
| T3-0 | T1-4 + T2-3 | 兼容层裁决文档 | — |
| T3-1/T3-2 | T3-0 | adapter 移除 | — |
| T3-3 | T3-1 | Console 验收 | — |
| T4-0 | — | 盘点、映射方案 | T1/T2/T5 全程 |
| T4-1 | T4-0 | AP 000009 迁移 | — |
| T4-2 | T4-1 | 映射导入工具 | — |
| T4-3 | T4-2 | 权益对账验收 | — |
| T4-4 | T4-0 | 订单核对结论（T5 联动） | T4-1 |
| T4-5 | T4-3/T4-4 | 观察窗/回滚预案 | — |
| T5-0 | — | 表清单校正 | T1/T4 |
| T5-1 | T5-0 | §8 裁决表 | T1/T4 |
| T5-2 | T5-1 | 冻结快照（M3 前置） | — |
| T5-3 | T5-0 | quiz_*/组织维度裁决 | T1-0（联动） |
| T6-0 | — | AutoMigrate 触达盘点 | 全部 |
| T6-1 | T1-1/T4-1/T5-1 | DDL 固化文件 | — |
| T6-2 | T6-1 | 执行流程/门禁 | — |
| T6-3 | T5-1 | modules.yaml 同步 | — |
| T7-0 | — | 种子现状盘点 | 全部 |
| T7-1 | T1-1 + T7-0 | 新栈种子 | — |
| T7-2 | T7-1 | 文档 | — |

## 10. 风险清单

| # | 风险 | 等级 | 缓解/回滚 |
|---|---|---|---|
| R1 | 目标表形态裁决（T1-0）与 T5-3 组织维度裁决不一致，导致迁移脚本返工 | 中 | T1-0 与 T5-3 同一提交落 ADR；字段映射表（T1-2）为唯一事实源，T2-0 强制引用 |
| R2 | 「47 张表」口径与生产不符（模型 45 张） | 低 | T5-0 以生产 `\dt` 校正并回填；裁决表以生产清单为准 |
| R3 | portal-api 切读（T1-3）与 T2 导入并行写 library 目录表 → 数据竞争/字段分叉 | 中 | T2-0 与 T1-2 共用同一字段映射与 upsert 语义；T1-3 切读放在 T1-2 对账通过后；保留 env 开关秒回 |
| R4 | 会员迁移丢权益（T4）——目标结构缺普通档（F8）、映射命中率低、verificationKey 不可得 | **高** | T4-0 先量化；T4-1 先扩结构再写库；T4-2 强制 dry-run + fail-closed；未命中进 manual_review 不自动合并（database-migration-spec §6）；快照 + 反向 dry-run |
| R5 | EasyPay 订单承接误判（T4-4）：历史 paid 订单与 AP 订单表 shape 不符 | 中 | T4-0 先查行数；paid 会员订单走导入链补记，非会员订单只读归档；在途订单不迁移并注明 |
| R6 | 移除 adapter（T3）过早，Console 资料运营在回归未通过时失去面 | 中 | T3 触发条件硬门禁（T1-4+T2-3）；回滚到 C 态（恢复 env + 旧镜像）成本低；git 历史保留 adapter |
| R7 | 冻结快照（T5-2）体积大 / 磁盘 89% 已用 | 中 | 先 dry-run 估算体积；必要时先清旧镜像/日志（M0 待办 #5）；快照脚本幂等化 |
| R8 | GORM AutoMigrate 残留触达点漏网（T6-0 未全列），生产 AUTO_MIGRATE 被误开 | 中 | T6-0 全量盘点 + preflight 门禁保持；CI 校验生产约束 |
| R9 | ops 脚本链（T2）改库后旧脚本残留写 study，造成双写 | 中 | T2-3 回归断言 study 库无新写入（T1-5）；旧脚本仅 git 历史保留 |
| R10 | 文档/配置漂移复发（T6-3/T7-2 遗漏） | 低 | 本清单与 `ALL_NEW_STACK_CUTOVER.md` 完成标记同步；每任务验收含文档项 |

## 11. Done 定义（M2 数据面）

- [ ] Portal `/library` 数据 100% 来自 library 库，portal-api 无 study 库连接（T1 全绿）。
- [ ] 材料 ops 链（import/sync/activate）全部写 library 库，study 库目录零写入（T2 全绿）。
- [ ] services/library 无 legacy adapter，`STUDY_LEGACY_*` env 移除，服务正常启动（T3 全绿）。
- [ ] memberships 迁入 account-portfolio，会员数/档位/到期日对账通过、Portal/Console 可见正确档位；历史订单去向定案（T4 全绿）。
- [ ] §8 裁决表 45 行全部定案；冻结表快照完成并演练（T5 全绿）。
- [ ] 47/45 表 DDL 固化到各 owner 版本化迁移，执行流程与门禁落文（T6 全绿）。
- [ ] 新栈种子入口可用且文档一致（T7 全绿）。
- [ ] `henukit.modules.yaml` data_owners、`CURRENT_PRODUCTION_STATE.md`、`ALL_NEW_STACK_CUTOVER.md` M2 节三方一致（M0 漂移修复规则的延续）。

---

## 附录 A：T5 裁决表（45 行；T5-0 核验后校正口径）

> 图例：**迁移**＝数据迁入目标 owner 库；**移交**＝数据/owner 语义移交，行数对账后落库；**冻结归档**＝只读快照，不迁移（D2/D3）。
> 目标 owner 简称：L=library 库、AP=account_portfolio 库、PC=platform 库、N=notice 服务、Q=quizcraft（已切流，不合并）。

| # | 表 | 裁决 | 目标 owner | 数据与依赖 | 快照/迁移方式 | 执行顺序 |
|---|---|---|---|---|---|---|
| 1 | courses | 迁移 | L | 24 行；目录数据源 | T1-2 幂等导入 | T1 |
| 2 | materials | 迁移 | L | 715 行（published 子集） | T1-2 幂等导入 | T1 |
| 3 | material_download_logs | 迁移（历史只读） | L | 历史下载记录；与 append-only `library_download_start_events` 结构不同 | 单独归档只读表（`library_material_download_logs_archive`）或冻结快照 | T1-5 |
| 4 | schools | 裁决→随 courses 迁 L（或冻结） | L | courses 组织维度；import 脚本依赖 | T5-3 定案后 T1-2 联动 | T5-3→T1 |
| 5 | colleges | 同上 | L | 同上 | 同上 | 同上 |
| 6 | majors | 同上 | L | 同上 | 同上 | 同上 |
| 7 | users | 移交 | PC | 身份数据；经 email_identities 映射（F10） | 映射对账移交；原表冻结保留 | T4 后 |
| 8 | email_verification_codes | 冻结归档 | — | 短期验证码（24h 清理），无保留价值 | 随 users 快照 | T5-2 |
| 9 | memberships | 迁移 | AP | 在用权益（D4） | T4-2 导入链 | T4 |
| 10 | membership_plans | 迁移 | AP | 档位定义（终身/普通） | T4-2 导入（或手工转录配置） | T4 |
| 11 | material_access_grants | 冻结归档 | — | 权益授予历史；commerce 已冻结（D2/D3） | 只读快照 | T5-2 |
| 12 | orders | 冻结归档（核对后） | — | 历史订单；EasyPay 为当前通道（D3） | T4-4 核对 → 只读快照；paid 会员订单补记 AP | T4-4→T5-2 |
| 13 | payment_records | 冻结归档 | — | 支付流水 | 只读快照 | T5-2 |
| 14 | payment_incidents | 冻结归档 | — | 支付事故记录 | 只读快照 | T5-2 |
| 15 | course_packages | 冻结归档 | — | 打包售卖（commerce 冻结） | 只读快照 | T5-2 |
| 16 | course_package_items | 冻结归档 | — | 同上 | 只读快照 | T5-2 |
| 17 | quiz_questions | 冻结归档 | — | 旧站刷题数据；与 quizcraft 无血缘（T5-3） | 只读快照（可能空表） | T5-3→T5-2 |
| 18 | quiz_options | 冻结归档 | — | 同上 | 同上 | 同上 |
| 19 | quiz_attempts | 冻结归档 | — | 同上 | 同上 | 同上 |
| 20 | quiz_answers | 冻结归档 | — | 同上 | 同上 | 同上 |
| 21 | wrong_questions | 冻结归档 | — | 同上 | 同上 | 同上 |
| 22 | weakness_reports | 冻结归档 | — | 同上 | 同上 | 同上 |
| 23 | wiki_entries | 冻结归档 | — | D2 关闭 | 只读快照 | T5-2 |
| 24 | wiki_edit_histories | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 25 | wiki_edit_proposals | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 26 | wiki_creator_applications | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 27 | blog_posts | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 28 | blog_comments | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 29 | forum_boards | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 30 | forum_posts | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 31 | forum_replies | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 32 | moments | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 33 | media_assets | 冻结归档 | — | D2（moments/论坛附件） | 只读快照 | T5-2 |
| 34 | moment_comments | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 35 | user_relations | 冻结归档 | — | D2（关注关系） | 只读快照 | T5-2 |
| 36 | points_logs | 冻结归档 | — | D2（points 关闭）；AP 有独立 point_ledger | 只读快照 | T5-2 |
| 37 | points_rules | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 38 | ai_tasks | 冻结归档 | — | D2（AI 随 worker 冻结） | 只读快照 | T5-2 |
| 39 | ai_drafts | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 40 | ai_usage_logs | 冻结归档 | — | D2 | 只读快照 | T5-2 |
| 41 | notifications | 移交/冻结 | N 或 PC | 旧站内通知历史；新通知走 notice 服务 | 推荐：只读快照归档（一次性历史）+ 新通知由 notice 承接 | T5-2 |
| 42 | reports | 冻结归档 | — | 旧站纠错/举报（D2） | 只读快照 | T5-2 |
| 43 | operation_logs | 冻结归档 | — | 审计日志，只读快照 | 只读快照 | T5-2 |
| 44 | leaderboard_snapshots | 冻结归档 | — | 旧排行榜快照（排行走 Go rankings） | 只读快照 | T5-2 |
| 45 | system_configs | 冻结归档 | — | 迁移前人工转录关键配置 | 只读快照 + 转录清单 | T5-2 |

> 校正说明：模型清单为 45 张（F11）；若 T5-0 生产核验确为 47，差额表补行并注明来源（GORM 附加/历史表）。所有「冻结归档」表在 M3 前只读保留，M3 后随 services/api 退役归档（不删除数据，快照已离线）。

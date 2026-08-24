# 求职雷达（Career）端到端验收与发布守门（#404）

> 本文件是求职雷达 epic（#391）最后一张票 #404 的验收输出：把 #392–#397、
> #399–#403 已交付能力收敛为可验证的完整闭环，并给出失败恢复矩阵、权限矩阵
> 回归、smoke 证据与发布守门清单。本票只补真实缺口测试、文档与 smoke 证据，
> 不引入新功能。
>
> 主链路：登录 → 配置画像 → 开始扫描 → 快速返回 `search_id` → 页面显示
> `queued/running` → 后台完成 → 结果持久化 → 页面直接显示 → 关闭页面不影响
> 任务 → 完成后受控邮件到已验证邮箱 → 重开 `/career` 恢复结果。

## 1. 自动化测试清单与命令

### 1.1 后端 · career 服务

```bash
cd services/career-opportunities && go test -count=1 ./...
```

无环境变量时自动经 testcontainers 启动 PostgreSQL 16 + Redis 7 并应用迁移；
也可预置 `CAREER_TEST_DATABASE_URL` / `CAREER_TEST_REDIS_ADDR` 指向已有环境。

对应验收覆盖：

| 验收点 | 用例 |
|---|---|
| 创建快速返回 `queued`，画像快照冻结 | `TestCreateSearchReturnsQueuedImmediately` |
| 浏览器自报 `user_id` 拒绝 | `TestCreateSearchRejectsBrowserSuppliedUserID` |
| actor 隔离（状态/历史/画像） | `TestStatusAndHistoryAreActorIsolated`、`TestProfileRoundTripAndActorIsolation` |
| worker `queued → running → completed`、结果落库 | `TestWorkerAdvancesSearchToCompleted` |
| 失败落 `failed` + 稳定错误码，不写结果 | `TestWorkerFailureLandsOnFailedWithStableCode` |
| 错误信息浏览器安全（不泄漏内部路径/凭据） | `TestFailureErrorMessageIsBrowserSafe` |
| 幂等：Idempotency-Key 重放同一 search | `TestIdempotencyKeyReplayReturnsSameSearch` |
| 完成重放不写第二份结果 | `TestReplayedCompletionWritesNoSecondResult` |
| worker 崩溃恢复（stale running 回收） | `TestWorkerReclaimsStaleRunningSearch` |
| worker 循环瞬态错误保活/退避重置 | `worker_run_test.go`（#409 合入） |
| 画像校验（非法 job_type/年份） | `TestProfileRejectsInvalidJobTypeAndYear` |
| 迁移可重复应用 | `TestMigrationAppliesTwice` |
| digest 入队（开/关/默认）、`email_sent_at` 记录 | `TestCompletedSearchEnqueuesDigestWhenNotificationsEnabled`、`TestCompletedSearchSkipsDigestWhenNotificationsDisabled`、`TestCompletedSearchDefaultsToNotificationsEnabled` |
| 邮件失败不回滚 search，`email_sent_at` 不置位可重试 | `TestDigestEnqueueFailureDoesNotRollBackSearch` |
| **DB 不可用 fail closed（503）** | `TestDatabaseUnavailableFailsClosed`（#404 补） |
| **超大 payload 拒绝（400，无写入）** | `TestCreateSearchRejectsOversizedPayload`（#404 补） |
| **端到端 smoke 单测闭环** | `TestSmokeLifetimeSearchJourney`（#404 补） |

### 1.2 后端 · GetWork 适配 seam（同模块单测）

```bash
cd services/career-opportunities && go test -count=1 -run GetWork -v .
```

| 验收点 | 用例 |
|---|---|
| 空 allowlist = 生产安全关闭态（0 来源，正常完成空结果） | `TestGetWorkEmptyAllowlistYieldsNoJobs` |
| allowlist 门控（未授权来源不执行） | `TestGetWorkAllowlistGatesSources` |
| 单来源超时/失败降级，成功来源结果保留 | `TestGetWorkSingleSourceFailureDegrades` |
| **全部已授权来源失败则任务失败，不把空结果伪装为成功** | `TestGetWorkAllSourcesFailedReturnsError` |

### 1.2.1 后端 · 简历上传 AI 提取（上传 → 识别 → 回填）

```bash
cd services/career-opportunities && go test -count=1 -run Extraction -v ./tests/
```

| 验收点 | 用例 |
|---|---|
| AI 未配置 = 生产安全关闭态（503 `AI_UNCONFIGURED`，不假装成功） | `TestCreateExtractionRejectsUnconfiguredAI` |
| 上传校验：缺文件/非法扩展名/伪造内容/空文件（400，零写入） | `TestCreateExtractionValidatesUpload` |
| 超大文件 413 `FILE_TOO_LARGE` | `TestCreateExtractionRejectsOversizedFile` |
| 每人每小时限流（429 `EXTRACT_RATE_LIMITED`，不同 actor 独立预算） | `TestCreateExtractionRateLimitsPerActor` |
| 完整闭环：queued → worker → completed，提取结果可读、文件字节已清除 | `TestExtractionLifecycleCompletesAndPurgesFile` |
| 失败落 failed + 稳定错误码，内部原因不外泄、文件字节清除 | `TestExtractionFailureLandsOnFailedWithStableCode` |
| actor 隔离（他人读取 404） | `TestExtractionIsActorScoped` |
| worker 崩溃恢复（stale running 回收） | `TestExtractionStaleRunningIsReclaimed` |
| OpenAI 兼容提取器：解析/围栏 JSON/截断/非法输出拒绝 | `extract_test.go`（同模块单测） |

### 1.3 后端 · platform-core（digest 邮件）

```bash
cd services/platform-core && go test ./internal/mailworker/ ./internal/smtpprovider/ ./tests/...
```

`./tests/...` 无环境变量时自动经 testcontainers 启动 PostgreSQL 17 + Redis；
`internal/mailworker/`、`internal/smtpprovider/` 为纯单测。

对应验收覆盖：

| 验收点 | 用例 |
|---|---|
| enqueue 202 → outbox 行 → mailworker 投递模板 `henukit_career_digest` | `TestCareerDigestEnqueueDeliversThroughMailWorker` |
| 未验证邮箱 / 未知用户 fail closed（404，零写入） | `TestCareerDigestEnqueueFailsClosedWithoutVerifiedEmail` |
| **邮件幂等：dedupe key `career_search_completed:{search_id}` 只入队一次** | `TestCareerDigestEnqueueIsIdempotentPerSearch` |
| 坏签名 401 | `TestCareerDigestEnqueueRejectsBadSignature` |
| 未配置凭据 503 | `TestCareerDigestEnqueueFailsClosedWhenUnconfigured` |
| dispatcher 失败进入 `retry_due` + 退避，可重试投递 | `verification_mail_test.go`（retry 状态机）、`mailworker/http_sender_test.go` |
| 响应不泄漏邮箱/密文/凭据 | 上表首行用例内断言 |

### 1.4 后端 · Portal Gateway（Lifetime 门）

```bash
cd services/portal-gateway && go test ./internal/httpapi/
```

| 验收点 | 用例 |
|---|---|
| anonymous → 401，不触达 upstream | `TestCareerCreateSearchRequiresSessionAndLifetime` |
| free → 403 `lifetime_required` | 同上 |
| lifetime → 放行且 actor 绑定自 Session（忽略自报 actor） | `TestCareerLifetimeCreateSearchBindsSessionActor` |
| membership 依赖失败 → 503 fail closed，不触达 upstream | `TestCareerCreateSearchFailsClosedWhenMembershipUnavailable` |
| 读路由（history/status/profile）同门禁并绑定 actor | `TestCareerReadsBindActorAndForward` |
| 未配置 career upstream → 503 | `TestCareerUnconfiguredFailsClosed` |

### 1.5 前端 · Portal

```bash
cd apps/portal && pnpm test && pnpm typecheck
```

| 验收点 | 用例文件 |
|---|---|
| `/career` 四态（anonymous / lifetime_required / ready / error） | `src/lib/career/page-state.test.ts`（8 用例） |
| 扫描状态机轮询、终态即停、瞬态失败继续轮询 | `src/lib/career/career-scan-state.test.ts`（12 用例） |
| API client：403 门 / 401 / 503 不透传 mock | `src/lib/career/gateway.test.ts`（6 用例） |
| 画像设置 API（Console A-08 与 /career 共用） | `src/lib/api/career-profile.test.ts` |
| reduced-motion：雷达动画以 `prefers-reduced-motion: no-preference` 门控，reduced 时静态 | `src/components/career/work-radar.tsx`（#406） |

## 2. 失败/恢复矩阵

| 故障注入 | 期望 | 测试位置 | 状态 |
|---|---|---|---|
| Account Portfolio（membership）不可用 | Gateway 503 fail closed，不触达 career | `portal-gateway` career_test.go | 已覆盖（#400 交付） |
| Career DB 不可用 | create / list / status 一律 503 `DEPENDENCY_UNAVAILABLE`，不误报 404、不返回空数据 | `TestDatabaseUnavailableFailsClosed` | **#404 补齐** |
| worker 领取后崩溃 | stale running 超窗回收，重新完成且只写一份结果 | `TestWorkerReclaimsStaleRunningSearch` + `worker_run_test.go`（#409） | 已覆盖 |
| 单来源超时/失败 | 该来源 failed，成功来源结果与 `sources` 状态保留 | `TestGetWorkSingleSourceFailureDegrades` | 已覆盖（#396） |
| 全部来源失败 | worker 将 search 标记为 failed，不生成虚假的成功空结果 | `TestGetWorkAllSourcesFailedReturnsError` + `TestWorkerFailureLandsOnFailedWithStableCode` | 已覆盖 |
| GetWork 非法/超大 payload | 512 KiB 上限，400 `INVALID_REQUEST`，零写入 | `TestCreateSearchRejectsOversizedPayload` | **#404 补齐** |
| 结果已写但邮件失败 | search 保持 completed，`email_sent_at` 不置位 → 后续可重试 | `TestDigestEnqueueFailureDoesNotRollBackSearch` | 已覆盖（#397） |
| 邮件 dispatcher 重试 | 失败进 `retry_due` + 指数退避，到期重投 | platform-core mailworker/verification_mail 测试 | 已覆盖 |
| 邮件重复入队 | dedupe key 幂等，outbox 仅一行 | `TestCareerDigestEnqueueIsIdempotentPerSearch` | 已覆盖（#397） |
| 刷新 / 关闭 / 重开 | 状态持久化于 PostgreSQL，重开 `/career` 恢复；轮询瞬态失败提示后继续 | `TestWorkerAdvancesSearchToCompleted` + `career-scan-state.test.ts` | 已覆盖（#402） |
| 按钮快速重复点击 | Idempotency-Key 重放返回同一 search | `TestIdempotencyKeyReplayReturnsSameSearch` | 已覆盖（#392） |

## 3. 权限矩阵回归

| 身份 | 期望 | 覆盖 |
|---|---|---|
| anonymous | 401，不触达上游 | Gateway 单测 + 前端四态 |
| free（非 Lifetime） | 403 `lifetime_required`，手工构造请求同样被拒 | Gateway 单测 |
| lifetime | 放行，actor 绑定自加密 Session | Gateway 单测 |
| 浏览器自报 `user_id` | 服务端忽略/拒绝，按签名 actor 判定 | career 服务单测 |
| 跨 actor 读他人 search/history/profile | 404 / 空列表 / 空画像 | career 服务单测 |

结论：Portal UI 锁定不是唯一授权层——Gateway `requireLifetime` 与 career 服务的
actor 绑定均为服务端强制，回归通过。

## 4. 邮件幂等与失败恢复

- 幂等键：`career_search_completed:{search_id}`（career 服务入队与 platform-core
  enqueue 共用）。
- `email_sent_at` 守卫：仅入队成功才置位；入队失败不置位、不回滚 search，重跑
  worker 可补投（`TestDigestEnqueueFailureDoesNotRollBackSearch`）。
- 收件人约束：仅已验证邮箱，未知/未验证用户 enqueue fail closed（404），零写入。
- dispatcher 侧：失败 `retry_due` + 退避，恢复后自动重投。

### 4.1 验收发现的 blocker 与修复（#404）

验收回归在 main 上暴露了一个 #397 的遗留 blocker：`000019_career_digest_mail`
把 `mail_outbox.kind` 泛化为 `career_digest`，但 `mail_outbox_priority_check`
仍只允许 `'critical'`，而 digest 入队写 `priority='bulk'` —— 每次成功路径的
digest enqueue 都因 `SQLSTATE 23514` 返回 503（`career_digest_mail_test.go`
两条成功用例红）。修复按迁移规范新增 `000020_mail_outbox_allow_bulk_priority`
（纯增量，允许 `priority IN ('critical','bulk')`；worker 领取查询本就按
critical 优先、其余同级排序，无需改代码）。修复后 `TestCareerDigestEnqueue
DeliversThroughMailWorker` / `TestCareerDigestEnqueueIsIdempotentPerSearch`
转绿。

## 5. 发布守门（发布前必读）

### 5.1 source allowlist 授权状态

- **生产启用**：`CAREER_SOURCE_ALLOWLIST=official.meituan` 注册独立编写的美团
  官方校招接口适配器；浏览器不能新增来源、URL 或选择器。
- 未注册的 allowlist key 会令服务启动失败，避免健康服务静默返回 0 来源。
- 空 allowlist 仅保留为本地开发和事故 kill switch；正式发布的 prebuilt
  Compose 要求该变量非空。
- 已启用来源全部失败时搜索进入稳定失败态，不把上游故障伪装成“0 岗位”。

### 5.2 kill switch

**allowlist 即 kill switch**：生产突发问题时清空 allowlist（配置为空数组/空
map）即停掉全部来源，无需改代码、无需新开关。空 allowlist 是安全默认值。

### 5.3 邮件与失败恢复守门

- dedupe key 幂等回归：`TestCareerDigestEnqueueIsIdempotentPerSearch`。
- `email_sent_at` 守卫回归：`TestCompletedSearchEnqueuesDigestWhenNotificationsEnabled` /
  `TestDigestEnqueueFailureDoesNotRollBackSearch`。
- 未验证邮箱 fail closed 回归：`TestCareerDigestEnqueueFailsClosedWithoutVerifiedEmail`。

### 5.4 前端守门

- reduced-motion：雷达动画（`work-radar.tsx`，模块内唯一的表盘组件）以
  `(prefers-reduced-motion: no-preference)` 门控，reduced-motion 用户看到静态
  展示：光束不转、命中呼吸的涟漪圈保持 `opacity=0`、目标点保持墨色。
- 表盘不渲染任何读数：准确计数只由任务状态区（`career-scan-status-panel.tsx`）
  给出，避免两处数字打架。表盘点亮的目标点数只在 completed 时取服务端确认的
  `result.matched_count`（超过表盘容量即截断——它是刻度盘不是计数器），进行中
  一个都不点亮，因为后端此时只返回 `stage`、不返回任何计数。
- `schematic` 模式（首页 05 区块、未登录介绍页）表头标 `SCHEMATIC`，并对辅助
  技术 `aria-hidden`，装饰表盘不会被读成一次真实扫描。
- 状态机/四态/权限门文案均经单元测试覆盖；生产模式失败不静默回退 mock
  （`gateway.test.ts`）。

## 6. Smoke 证据

```bash
scripts/dev/career-smoke.sh            # 单条 Lifetime 闭环（默认）
scripts/dev/career-smoke.sh --all      # 整个 career 服务套件
```

脚本自起 testcontainers（PostgreSQL + Redis），驱动完整闭环：画像 PUT →
search 创建（queued）→ worker Step → completed → 结果落库 → digest 入队 →
状态/历史读取；输出 tee 到 `docs/career-smoke-evidence.txt` 作为证据。

最近一次证据：`docs/career-smoke-evidence.txt`（#404 合入时生成，含 testcontainers
环境信息与 `--- PASS: TestSmokeLifetimeSearchJourney`）。

## 7. 回归结果（#404 合入时）

- `services/career-opportunities`：`go test -count=1 ./...` 通过
  （`henukit.dev/career` + `henukit.dev/career/tests`，含 #404 新增 4 个用例）；
  `gofmt`（改动文件）与 `go vet ./...` 通过。
- `services/platform-core`：`go test ./internal/mailworker/ ./internal/smtpprovider/ ./tests/...` 全绿
  （修复 `000020` 前，digest 两条成功用例红——见 4.1）。
- `services/portal-gateway`：`go test ./internal/httpapi/` 通过。
- `apps/portal`：`pnpm test`（14 文件 / 75 用例）与 `pnpm typecheck` 通过。
- 详细输出见各服务命令执行结果；#404 关闭评论附带对照。

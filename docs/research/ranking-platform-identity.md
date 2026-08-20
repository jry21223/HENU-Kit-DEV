# 排行平台身份研究：废除 Ranking Profile，排行直接复用平台唯一身份

- 状态：研究 / 待评审
- 日期：2026-08-19
- 决策依据：用户拍板 —— 废除「用户主动上传排行 Profile」机制；排行条目直接显示 platform-core `users.display_name`；未设置显示名显示中性标签；游客（匿名练习者）也可上榜；对外绝不出现 `user_id`/UUID（ADR-0036 隐私契约不变）。
- 关联文档：ADR-0036（本方案 amend 的目标）、ADR-0018、`docs/research/practice-read-path-ownership.md`

---

## 1. 现状梳理

### 1.1 当前排行数据流（三段）

```
quizcraft_practice_attempts ──(JOIN)── quizcraft_ranking_profiles p ON p.user_id=a.user_id AND p.visible
   │ WHERE a.correct AND a.user_id IS NOT NULL
   ▼
Go core ListOverallRanking / ListBankRanking   ← 只输出 {rank, nickname, system_avatar, correct_answer_count}
   ▼ (five-part HMAC Portal read, X-Actor-User-Id=anonymous)
portal-gateway practice.Client.ranking()
   │ DisallowUnknownFields 解码 + validateRanking（拒绝 user_id、拒绝 32 位 hex 昵称、拒绝空昵称、校验 avatar）
   ▼
Portal UI /api/v1/rankings/overall、/api/v1/banks/{bank_id}/rankings
```

### 1.2 关键代码事实

| 事实 | 位置 |
|---|---|
| 排行读路由挂 five-part Portal read；`writeRanking` 只回 nickname/avatar/count，**无 user_id** | `products/quizcraft/go-service/practice_http.go:291-294, 397-450` |
| 排行 SQL 强制 `JOIN quizcraft_ranking_profiles … AND p.visible` 且 `a.user_id IS NOT NULL`（游客与不可见者缺席） | `db/queries/practice.sql:312-342`（live）、`344-370`（window） |
| settlement window 查询**输出 user_id**，standings JSONB 序列化含 `user_id` | `practice.sql.go:847-899`（ListOverallRankingWindow 含 `user_id`）、`ranking_settlement.go:32-57` |
| attempts 表只有 `user_id uuid NULL`，**没有 per-attempt 匿名身份列**；sessions 有 `actor_key CHECK '^(guest|user):[0-9a-f-]{36}$'`；两张表都带 immutability trigger | `db/migrations/000002_practice_core.up.sql:8-19, 36-54, 100-119` |
| 游客身份 = `quizcraft_anonymous` cookie（90 天 JWT）→ `practiceActor{key: "guest:"+subject}`；登录 = `user:<uuid>` | `practice_http.go:1392-1445` |
| PATCH `/api/v1/ranking-profile`（writes 门禁 + 幂等 + per-user advisory lock + UpsertRankingProfile） | `practice_http.go:303-325, 452-565`；`db/queries/practice.sql:304-310` |
| gateway `ranking()` 用 `DisallowUnknownFields` 解码；`validateRanking` 拒绝 user_id / 32-hex 昵称 / 空昵称 / 非法 avatar | `services/portal-gateway/internal/practice/client.go:137-156, 468-514` |
| 隐私契约测试：Core 响应夹带 `user_id` → 网关 502 拒绝并断言不泄露 | `services/portal-gateway/internal/httpapi/quizcraft_dark_test.go:424-505` |
| 生成契约 `RankingEntry{rank, nickname, system_avatar, correct_answer_count}`，注释明言"internal account identifiers deliberately absent" | `services/portal-gateway/internal/practice/contract_generated.go:51-71` |
| quizcraft.yaml 排行契约 + RankingProfileUpdate schema + ranking-profile 路径；portal-gateway.yaml 外部排行响应含 `system_avatar` | `packages/api-contracts/openapi/quizcraft.yaml:427-475, 960-987, 1226-1234`；`portal-gateway.yaml:842-889, 1755-1788` |
| 隐私契约原文：公开排行只有 `rank/nickname/system_avatar/correct_answer_count`；UUID 永不出现；无可见 profile 者缺席；空昵称 → `匿名学习者` | `docs/adr/0036-portal-practice-read-path-owner-go-core.md:30-36` |
| platform-core `users.display_name`：text NULL，CHECK NULL 或 1-80 字符；已有单用户查询 `GetPlatformUser`（按 id），**无批量 display_name 接口** | `services/platform-core/db/migrations/000013_password_registration.up.sql:6-11`；`internal/store/identity.sql.go:419-443` |
| 服务间认证：网关 `serviceauth.Signer.Sign`（5 行 canonical：METHOD\nURI\ntimestamp\nnonce\nbodyHash），platform-core `authenticateServiceRequest` 校验 5 行 + nonce 一次性 + 5min 时钟窗；六段 = `SignWithActor` 追加 actor 行（仅 gateway→QuizCraft 用） | `services/portal-gateway/internal/serviceauth/signer.go:36-92`；`services/platform-core/internal/identity/service.go:149-188` |
| 网关已持有 platform-core 凭据：`PLATFORM_CORE_URL` + `PlatformClientID/Secret/KeyID`，已有 `platformcore.NewClient` + ExchangeCode（5 行签名）可作范本 | `services/portal-gateway/internal/config/config.go:19-23, 92-96`；`internal/platformcore/client.go:26-101` |
| settlement 事件表只写不读（CLI `settleranking` 落库；无 HTTP 端点消费），standings 幂等 `UNIQUE(period_start,period_end,scope,bank_id)` | `products/quizcraft/go-service/cmd/settleranking/main.go`；`db/migrations/000004_ranking.up.sql:16-33` |
| `quizcraft_ranking_profiles` 属于 pre-history baseline 表清单（冻结不删最省事） | `products/quizcraft/go-service/migration_artifacts.go:36-67` |

### 1.3 现有测试面（必须随之调整）

- `products/quizcraft/go-service/tests/ranking_test.go`：6 个用例全部依赖 profile 机制（PATCH profile、opt-out 缺席、匿名 profile、settlement standings 断言含 user_id/nickname、隐藏 bank settlement=`[]`）。
- `services/portal-gateway/internal/practice/client_test.go`：mock Core 排行响应带 nickname/avatar。
- `services/portal-gateway/internal/httpapi/quizcraft_dark_test.go`：隐私契约测试（user_id 夹带→502）。

---

## 2. 方案选择（A：display_name 映射发生在哪一层）

### 2.1 候选路径对比

**路径 1（推荐）：Go core 内部契约带 `user_id`，gateway 经新 platform-core 批量接口解析 display_name，对外剥离。**

- Go core 排行查询返回 `{rank, user_id(可空), correct_answer_count}`（游客 `user_id=null`），`nickname`/`system_avatar` 从 core 内部契约**移除**（core 是纯事实服务，不含身份展示层）。
- gateway 收到内部响应后：收集 `user_id` 集合 → 查本地 TTL 缓存 → 未命中走 `POST /api/v1/users/display-names`（5 行 HMAC，复用现有 `PLATFORM_CORE_URL` 凭据）→ `nickname = display_name` 或 `匿名学习者`（user_id 为 null / display_name 为空）→ 组装对外条目，**剥离 user_id**。
- 缓存：进程内 TTL map（如 10min、上限 2048 条、`singleflight` 合并并发未命中）；display_name 变更罕见，TTL 足够；platform-core 故障时**降级为 `匿名学习者` + 日志/指标**（排行可用性 > 昵称准确性）。

**路径 2（不推荐）：Go core 在提交/结算时落库 display_name 快照。**

- 昵称漂移问题：`users.display_name` 是用户可随时修改的平台属性；提交时快照意味着改完显示名后，旧答题记录仍显示旧名，直到用户再次练习才刷新；lifetime 排行会长期展示陈旧名字。结算快照则把"姓名"这种易变 PII 冻结进不可变事件，反而鼓励不更新。
- 跨服务数据重复：QuizCraft 存储了它不需要的账户 PII（display_name 属于 platform-core 的账户域）；未来改名、注销都要处理同步。
- 唯一可取之处是"历史结算快照定格当时名字"，但当前结算事件无任何消费者，不值得为此引入漂移与 PII 成本。

**结论：路径 1。** 单一事实来源（`users.display_name`）、无漂移、游客天然 `user_id=null → 匿名学习者`、profile 机制干净移除；代价是 gateway 对 platform-core 新增一个只读依赖，用缓存 + 降级消化。

### 2.2 游客上榜（B）的关键设计

- **匿名身份归属**：attempts 无 per-attempt 匿名列，但 `sessions.actor_key`（`guest:<uuid>`，创建时写入、不可变）是稳定归属键。排行查询对每次 attempt 取 `COALESCE(a.user_id, s.actor_key)` 作为**身份键**（登录者=user_id，游客=guest 主体 UUID），经 `LEFT JOIN quizcraft_practice_sessions s ON s.id=a.session_id` 实现——**零迁移**，且历史游客答题（老数据）也能归属。session 被 claim 前后语义自洽：claim 前提交的答案按游客计，claim 后提交的按 user_id 计（写入时真相）。
  - 备选：给 attempts 加 `guest_subject uuid` 列 + 部分索引（`WHERE correct AND guest_subject IS NOT NULL`），避免 JOIN；代价是 000012 迁移 + 写路径改动。v1 推荐零迁移 JOIN；若压测发现排行查询成为瓶颈再上列。
- **去重**：attempts 层 `UNIQUE(session_id, question_id)` 已保证"同一会话同一题只计一次"；结算按 `UNIQUE(period_start,period_end,scope,bank_id)` + `LockIdempotency` 幂等（现为每周一次，天然无"同日多次结算"问题；若未来引入日结，沿用同一幂等键即可）。
- **防刷上限**：公开榜 `LIMIT 100` 不变，游客与登录者同池竞争、同一规则计分（公平性优先，不做游客配额）；anti-abuse 杠杆为**可配置上榜门槛 `min_correct_answers`（默认 1，运营可调至 3）**，抬高刷榜成本；游客 cookie 单浏览器单份（Set-Cookie 覆盖）+ 90 天寿命，旋转 cookie 需真实答题成本；对游客的残余刷榜风险接受并记录（见 §8）。
- **未设置/历史遗留的匿名答题**：`user_id` 与身份键均为 NULL 的行（000012 之前、cookie 不可考的极端残留）直接排除出榜，避免混入不可归属的计数。

### 2.3 移除 Ranking Profile 机制（C）

- `PATCH /api/v1/ranking-profile` 路由与 handler（`updateRankingProfile`/`normalizeRankingNickname`/`looksLikeRankingIdentifier`/`validSystemAvatar`/`rankingProfileRequest`）整体删除；`UpsertRankingProfile`/`LockRankingProfileMutation` SQL 与生成代码删除。
- `quizcraft_ranking_profiles` 表：**冻结不删**（推荐）。删除需新增 000012 down/up 且牵动 `migration_artifacts.go` 基线清单与指纹；表极小、无消费者，冻结成本为零、可逆。`migration_test.go` 中"迁移前后 profiles 计数不变"的断言在新机制下依然成立（不再写入）。
- OpenAPI：从 quizcraft.yaml 删除 `/api/v1/ranking-profile` 路径与 `RankingProfileUpdate` schema，Ranking tag 描述去掉 "private ranking profile controls"。
- 语义变化：**无 opt-in/opt-out**。凡产生正确答题的登录者/游客都进入公开榜（按身份键聚合），原先"profile 不可见者缺席"彻底取消。

### 2.4 契约变更（D）

- **quizcraft.yaml（内部契约）**：`RankingPage.entries` 增加 `user_id`（`type: [string, 'null'], format: uuid`），标注 `x-internal: true` + 描述"internal-only：Portal Gateway 必须对外剥离（ADR-0036）"；同时条目移除 `nickname`/`system_avatar`（改由 gateway 合成），`required: [rank, user_id, correct_answer_count]`。
- **gateway 对外响应结构不变**：`{rank, nickname, system_avatar, correct_answer_count}`，`nickname = display_name | 匿名学习者`。
  - ⚠️ **需要拍板的子决策——`system_avatar`**：现状对外契约与 Portal UI 都渲染 `system_avatar`（portal-gateway.yaml:1773-1784 必填）。平台身份没有头像字段，两条路：
    - 子决策 A（推荐）：保留字段，gateway 按身份键哈希稳定派生（SHA-256(identity_key) 首字节 mod 4 → 四档系统头像），零 UI 改动、非用户可控、无 PII；
    - 子决策 B：删除 `system_avatar`，对外条目即 `{rank, nickname, correct_answer_count}`（对应任务描述中的写法），需同步改 portal-gateway.yaml 与 Portal UI。
    - 若产品倾向 B，改动清单 §5 中 gateway/契约/UI 条目相应收敛，其余设计不变。
- **portal-gateway.yaml**：对外 schema 不变（或按子决策 B 删除 avatar）；若 B，需把 `required` 改为 `[rank, nickname, correct_answer_count]`。
- **gateway `validateRanking` 调整**：内部阶段改为接受 `user_id`（uuid 或 null）且**必填**（缺 user_id 视为契约违反 → ErrInvalidRanking，杜绝旧 core/新 gateway 混排把所有人当游客），rank 递增、count 单调不减、`user_id` 无重复、entries ≤ 100；对外阶段去掉 nickname 形状/avatar 校验（nickname 由网关自产，不信任上游）；`looksLikeRankingIdentifier` 在网关侧不再需要（昵称来源改为平台 display_name，校验职责移至平台侧或直接信任）。

### 2.5 platform-core 新接口（E）

- **路径**：`POST /api/v1/users/display-names`（只读但走 POST，避免查询串携带 ids 过长与日志泄露）。
- **认证**：复用网关现有 `PLATFORM_CLIENT_ID/PLATFORM_SECRET/PLATFORM_KEY_ID` + `serviceauth.Signer.Sign`（5 行 HMAC），platform-core `authenticateServiceRequest` 原生支持（GET/POST、nonce 一次性、5min 时钟窗）；**不需要六段 actor 绑定**（该接口是产品无关的身份解析，不绑浏览器会话）。
- **请求**：`{"user_ids": [uuid × 1..100]}`（body ≤ 16KB，`MaxBytesReader` 对齐现有模式）。
- **响应**：`{"request_id": "...", "data": {"display_names": [{"user_id": uuid, "display_name": string|null}]}}`（与 platform-core 现有 `writeSuccess` 信封一致；`display_name` 为 NULL 表示未设置）。
- **行为**：只返回 `display_name`，**绝不返回 email/status 等敏感字段**；未知/不存在 user_id 返回 `display_name: null`（不做 404，便于网关批量降级）；单条超长/畸形由 CHECK 约束兜底（1-80）。
- **上限**：批量 100；超限 400 `invalid_batch_size`。
- **缓存建议**：网关进程内 TTL（10min / 2048 条 / singleflight）；不设主动失效（display_name 修改无推送通道，TTL 过期即可，排行非强一致场景）。
- **幂等/审计**：只读接口无幂等键；`requestAudit` 中间件自动审计（record 服务方 clientID/keyID、不记录 body 明细），复用现有 nonce 防重放。

---

## 3. 数据流图（ASCII）

```
【提交路径】 浏览器 / Portal UI
   ├─ 登录者：__Host-quizcraft_session → actor{userID, key:"user:<id>"}
   └─ 游客：  quizcraft_anonymous cookie → actor{key:"guest:<uuid>"}（90 天）
        │
        ▼  POST /api/v1/practice/sessions/{id}/answers
quizcraft_practice_attempts(+user_id)  ← 游客 user_id=NULL，归属走 sessions.actor_key

【排行读路径】 浏览器 GET /api/v1/rankings/overall?period=weekly|lifetime
        │
        ▼  portal-gateway（无浏览器身份，X-Actor-User-Id=anonymous，5 行 HMAC）
   practice.Client.ranking()
        │
        ▼  GET /api/v1/rankings/overall（five-part Portal read）
   Go core writeRanking
        │  新内部契约：entries[{rank, user_id(可空), correct_answer_count}]
        │  聚合身份键 = COALESCE(a.user_id, s.actor_key)，LIMIT 100
        ▼
   gateway
   ├─ 收集 user_ids（排除 null）
   ├─ 查 display-name 缓存 ──miss──▶ POST /api/v1/users/display-names（5 行 HMAC）
   │                                    platform-core：SELECT id, display_name FROM users WHERE id = ANY($1)
   ├─ nickname = display_name | 匿名学习者（user_id=null 或 display_name 为空）
   ├─ system_avatar = 身份键哈希派生（子决策 A）｜或删除（子决策 B）
   └─ **剥离 user_id**，写入对外条目 {rank, nickname, system_avatar, correct_answer_count}
        │
        ▼  Portal UI

【结算路径】 cmd/settleranking（每周一次，幂等）
   ListOverallRankingWindow / ListBankRankingWindow（含身份键，含游客）
        ▼
   quizcraft_ranking_settlement_events.standings = [{user_id(可空), rank, correct_answer_count}]
   （不可变事件；无消费端点；显示时由消费方按 §2.1 规则解析名字）
```

---

## 4. 契约草案（JSON schema 片段）

### 4.1 quizcraft.yaml —— 排行条目（内部契约，user_id internal-only）

```yaml
RankingPage:
  type: object
  additionalProperties: false
  required: [scope, period, metric, entries]
  properties:
    scope: { type: string, enum: [overall, bank] }
    bank_id: { type: string, format: uuid }
    period: { $ref: '#/components/schemas/RankingPeriod' }
    metric: { type: string, const: correct_answer_count }
    entries:
      type: array
      maxItems: 100
      items:
        type: object
        additionalProperties: false
        required: [rank, user_id, correct_answer_count]
        properties:
          rank: { type: integer, minimum: 1 }
          user_id:
            type: [string, 'null']
            format: uuid
            x-internal: true
            description: >
              Internal-only stable actor key; null for guest learners.
              Portal Gateway MUST strip this field before any external
              response (ADR-0036 privacy contract).
          guest_key:
            type: [string, 'null']
            x-internal: true
            description: >
              Internal-only stable anonymous identity key (the session's
              immutable actor_key text) for guest learners; null for
              signed-in learners. Portal Gateway uses it only to derive a
              stable 游客x display label and MUST never expose it before
              any external response (ADR-0038).
          correct_answer_count: { type: integer, minimum: 0 }
```

> 删除：`/api/v1/ranking-profile` 路径、`RankingProfileUpdate` schema；`RankingPage.entries` 移除 `nickname`/`system_avatar`。

### 4.2 platform-core —— 新增批量 display-name 接口

```yaml
/api/v1/users/display-names:
  post:
    operationId: resolveUserDisplayNames
    security: [{ serviceBasic: [], serviceSignature: [], serviceReplay: [] }]
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            additionalProperties: false
            required: [user_ids]
            properties:
              user_ids:
                type: array
                minItems: 1
                maxItems: 100
                uniqueItems: true
                items: { type: string, format: uuid }
    responses:
      '200':
        description: display_name is null when unset or unknown.
        content:
          application/json:
            schema:
              type: object
              required: [request_id, data]
              properties:
                request_id: { type: string }
                data:
                  type: object
                  additionalProperties: false
                  required: [display_names]
                  properties:
                    display_names:
                      type: array
                      items:
                        type: object
                        additionalProperties: false
                        required: [user_id, display_name]
                        properties:
                          user_id: { type: string, format: uuid }
                          display_name: { type: [string, 'null'], maxLength: 80 }
      '400': { $ref: '#/components/responses/BadRequest' }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '409': { $ref: '#/components/responses/ServiceReplay' }
```

### 4.3 gateway 对外响应（不变，子决策 A 保留 avatar）

```yaml
QuizCraftRankingResponse:
  type: object
  required: [request_id, data]
  properties:
    request_id: { type: string }
    data:
      type: object
      additionalProperties: false
      required: [scope, period, metric, entries]
      properties:
        scope: { type: string, enum: [overall, bank] }
        bank_id: { type: string, format: uuid }
        period: { type: string, enum: [weekly, lifetime] }
        metric: { type: string, const: correct_answer_count }
        entries:
          type: array
          items:
            type: object
            additionalProperties: false
            required: [rank, nickname, system_avatar, correct_answer_count]
            properties:
              rank: { type: integer, minimum: 1 }
              nickname: { type: string, minLength: 1, maxLength: 80,
                          description: users.display_name or the neutral 匿名学习者 label. }
              system_avatar: { type: string, enum: [scholar-blue, coder-green, reader-amber, owl-purple],
                               description: Deterministically derived from the internal identity key; not user-controlled. }
              correct_answer_count: { type: integer, minimum: 0 }
```

---

## 5. 文件级改动清单

### 5.1 QuizCraft Go core（`products/quizcraft/go-service`）

| 文件 | 改动 |
|---|---|
| `db/queries/practice.sql` | 重写 4 条排行查询（ListOverallRanking / ListBankRanking / ListOverallRankingWindow / ListBankRankingWindow）：去 profile JOIN；`LEFT JOIN quizcraft_practice_sessions` 取身份键 `COALESCE(a.user_id, s.actor_key)`；`WHERE a.correct AND COALESCE(a.user_id, s.actor_key) IS NOT NULL`；`GROUP BY 身份键`；输出 `user_id`（可空）+ rank + count；`ORDER BY count DESC, 身份键`；删除 UpsertRankingProfile / LockRankingProfileMutation |
| `internal/store/practice.sql.go`、`models.go` | sqlc 重新生成；Row 类型相应增减 |
| `practice_http.go` | `writeRanking` 改输出 `{rank, user_id, correct_answer_count}`；删除 `updateRankingProfile` 与 `normalizeRankingNickname`/`looksLikeRankingIdentifier`/`validSystemAvatar`/`rankingProfileRequest` 及 `writes.Patch("/api/v1/ranking-profile")` 注册（304-325 区块）；游客归属无需改动提交路径（JOIN 方案零写路径改动） |
| `ranking_settlement.go` | 随 window 查询输出变化，standings 序列化 `{user_id(可空), rank, correct_answer_count}` |
| `cmd/settleranking/main.go` | 无改动（纯 CLI 调用点） |
| `db/migrations/` | **v1 无新迁移**（JOIN 方案）；备选：如走 attempts.guest_subject 列则加 000012 up/down |
| `tests/ranking_test.go` | 重写（见 §7） |
| `db/migrations/000004_ranking.up.sql` | 不动（profiles 表冻结；部分索引 `WHERE correct AND user_id IS NOT NULL` 可保留，若压测需要再扩 guest 分支） |

### 5.2 platform-core（`services/platform-core`）

| 文件 | 改动 |
|---|---|
| `internal/contract/` | 新增路由常量 `DisplayNamesRoute = "/api/v1/users/display-names"` 与请求/响应类型（沿用现有信封风格） |
| `internal/store/` | 新增批量查询：`SELECT id, display_name FROM users WHERE id = ANY($1::uuid[])` |
| `internal/httpapi/handler.go` | 新增 `resolveUserDisplayNames` handler：`authenticateServiceRequest`（5 行 HMAC）→ 批量上限 100 / body ≤16KB → 查询 → 返回 display_name（null 表示未设置/未知） |
| `tests/` | 新增 display-names 端点测试（认证、上限、null display_name、未知 id、重放 nonce） |

### 5.3 portal-gateway（`services/portal-gateway`）

| 文件 | 改动 |
|---|---|
| `internal/practice/contract_generated.go` | 依 quizcraft.yaml 重新生成（RankingEntry 增 `UserID`、删 Nickname/SystemAvatar；契约 SHA 变更属预期） |
| `internal/practice/client.go` | `ranking()` 解码后：收集 user_ids → display-name 解析（缓存 + singleflight）→ 组装对外条目（nickname、avatar 派生）→ 剥离 user_id；`validateRanking` 内部版改为接受并校验 user_id（必填、可空、无重复）；删除 `looksLikeRankingIdentifier`/`validRankingAvatar`（对外昵称自产，不再校验上游形状） |
| `internal/practice/display_names.go`（新增） | 进程内 TTL 缓存（10min / 2048 条 / singleflight）；platform-core 失败 → 降级 `匿名学习者` + 日志/指标 |
| `internal/platformcore/client.go` | 新增 `DisplayNames(ctx, userIDs []string)`（POST + 5 行签名，对齐 ExchangeCode 写法） |
| `internal/httpapi/quizcraft_rankings.go` | 基本不变；可补充 platform-core 降级错误路径的注释/日志 |
| `internal/httpapi/quizcraft_dark_test.go` | 重写隐私测试（见 §7） |
| `internal/practice/client_test.go` | mock Core 排行响应改为新内部契约；新增 display-names mock |
| `internal/config/config.go` | 无新环境变量（复用 PLATFORM_CORE_URL + Platform 凭据） |

### 5.4 契约 / 文档

- `packages/api-contracts/openapi/quizcraft.yaml`：§4.1 改动 + 删 ranking-profile 路径/schema + Ranking tag 描述。
- `packages/api-contracts/openapi/portal-gateway.yaml`：对外不变（或按子决策 B 删 avatar）。
- `docs/adr/0038-ranking-identity-from-platform-profile.md`（新增，见 §6）；`docs/adr/0036-…md` 隐私条款修订注明。
- 本文档 `docs/research/ranking-platform-identity.md`。

### 5.5 迁移顺序

1. **Go core**：practice.sql 重写 + sqlc 再生成 + writeRanking/route/handler 删除 + settlement 序列化 → core 单元/集成测试绿（先于一切，纯内部行为变化，旧 gateway 会因 decode 失败而 502，属预期断崖，需在同窗口内完成第 2-4 步）。
2. **契约**：quizcraft.yaml 改 + gateway contract_generated.go 再生成（SHA 变化进入同一提交）。
3. **platform-core**：新端点 + 测试（无依赖，可并行）。
4. **gateway**：client 解码/映射/缓存 + platformcore.DisplayNames + 对外不变。
5. **测试**：go-service ranking_test 重写、gateway dark/privacy 测试重写、新端点测试。
6. **ADR/文档**。

> 部署纪律：core 先于 gateway（新 gateway 严格校验 user_id 必填，旧 core 响应无 user_id 会 502 而非误标游客）；第 1-4 步必须同一发布窗口完成，避免 core/gateway 契约漂移窗口。

---

## 6. ADR 草案要点

### 6.1 建议：新增 ADR-0038（amend ADR-0036）

```
status: accepted
amends: 0036

# Ranking identity derives from the platform profile

## 背景
ADR-0036 规定公开排行只含 rank/nickname/system_avatar/correct_answer_count，
无可见 ranking profile 者缺席。现废除用户主动上传的 ranking profile 机制，
排行直接复用 platform-core 唯一身份 users.display_name，游客亦可上榜。

## 决策
- 昵称来源：公开排行 nickname = platform-core users.display_name（NULL 或 1-80
  字符）；未设置（NULL/空/未知 id）→ 中性标签「匿名学习者」。display_name 由
  Portal Gateway 经新批量接口 POST /api/v1/users/display-names（5 行 HMAC，
  复用 PLATFORM_CORE_URL 凭据）在读取时实时解析，进程内 TTL 缓存（10min）。
- 移除 profile 机制：PATCH /api/v1/ranking-profile 路由与 handler 删除；
  quizcraft_ranking_profiles 表冻结不删（基线清单兼容，可逆）；无 opt-in/opt-out，
  凡有正确答题者均上榜。
- 游客上榜：匿名练习者（quizcraft_anonymous 身份）按 sessions.actor_key
  归属计入公开榜，显示「匿名学习者」；同一会话同一题只计一次
  （UNIQUE(session_id,question_id)），结算幂等 UNIQUE(period,scope,bank)。
- 隐私契约（0036 §2 不变）：对外响应永不出现 user_id/UUID。Go core→gateway
  内部契约携带 user_id（可空），gateway 必须剥离；网关校验 user_id 必填，
  杜绝旧契约混排。
- system_avatar：由内部身份键确定性派生（或按产品决策删除，见研究文档 §2.4）。

## 后果
- Gateway 排行读路径新增 platform-core 只读依赖；故障降级为「匿名学习者」。
- display_name 重命名经 TTL 缓存延迟生效（≤10min），排行非强一致。
- settlement 历史事件保持不可变；新事件 standings 形状为
  {user_id(可空), rank, correct_answer_count}，名字在展示时解析。
```

### 6.2 ADR-0036 修订点（第 2 条隐私契约）

- 「Learners without a visible profile are absent」→ **删除**（无 profile 概念，游客与登录者均上榜）。
- 「empty nicknames render 匿名学习者」→ 改为「未设置 display_name（或游客）→ 匿名学习者」。
- 新增一句：内部契约允许 user_id，网关剥离义务 + 对外契约不变。

---

## 7. 测试计划

### 7.1 Go core（`products/quizcraft/go-service/tests/ranking_test.go`）

| 用例 | 处置 |
|---|---|
| `TestRankingHTTPCountsNewSessionsOnceAndProtectsPublicIdentity` | 改：去掉 profile PATCH 前置；断言内部响应含正确 `user_id`（且不等于对外泄露——core 内部契约本就有 user_id，断言其正确性与可空性）；并列 rank 语义保留；opt-out 断言删除，改为"未设 display_name 的登录者仍上榜（user_id 有值、nickname 由网关合成）" |
| `TestRankingProfileRejectsOmittedNickname…` / `TestRankingProfileRejectsIdentifierShapedNicknames` / `TestRankingProfileMutationsAreSerializedPerUser` | **删除**（机制移除） |
| `TestRankingHTTPStaysDarkUntilPortalGatewayAndUsesNeutralAnonymousProfile` | 改：新增游客作答用例——游客（无 cookie→带 cookie）答题后出现在内部排行，`user_id=null`；dark（未配置 catalogClientID → 404）与鉴权用例保留 |
| `TestRankingSettlementFactsAreImmutableAndRewardFree` | 改：standings 断言改为包含 user_id（登录者）与 null（游客）；"fully opted-out bank = []" 改为"无答题的 bank = []"；不可变 trigger 断言保留 |

新增：游客跨会话去重（同一 guest 身份同一题多会话计数）、游客+登录者混合聚合、身份键 tie-break 稳定性。

### 7.2 portal-gateway

- `internal/practice/client_test.go`：mock Core 响应改新契约（含 user_id/null）；新增 display-names mock（缓存命中/未命中/降级）。
- `internal/httpapi/quizcraft_dark_test.go`：`TestRouterRankingContractNeverLeaksUserID` **重写为**：
  - Core 内部响应携带 `user_id` → 网关接受（不再 502），对外响应**无 user_id**、nickname=display_name（来自 mock platform-core）；
  - `user_id=null` → 对外 nickname=`匿名学习者`；
  - Core 响应缺 `user_id` 字段 → 502（契约违反）；
  - platform-core 不可用 → 对外仍 200、nickname 全部降级为 `匿名学习者`、且无 user_id。
- 新增：display-name 缓存 TTL 行为测试（单测级别）。

### 7.3 platform-core

- 新端点测试：5 行 HMAC 认证成功/失败、nonce 重放 409、批量 >100 → 400、未知 id → display_name null、未设置 display_name → null、响应不包含 email/status 字段。

### 7.4 契约漂移

- `contract_generated.go` 重新生成后，现有 `QuizCraftCatalogContractSHA256`/`QuizCraftRankingContractSHA256` 断言随生成物更新；gateway 对 core 的 decode 失败（旧 core）有显式 502 用例。

---

## 8. 风险清单

| # | 风险 | 评估与缓解 |
|---|---|---|
| R1 | **display_name 空置率高**：大量用户未设显示名 → 榜上密集「匿名学习者」 | 产品接受（中性标签本来就是目标）；可另立"引导设置显示名"运营项（非本设计范围） |
| R2 | **昵称重名**：display_name 不唯一，同名多条 | 仅展示问题；排序/并列由 rank 与身份键 tie-break 决定，稳定可复现 |
| R3 | **防刷/刷票**：游客可旋转 cookie 刷榜 | 上榜门槛 `min_correct_answers`（可配置）+ 单浏览器单 cookie + 90 天寿命 + 榜单 LIMIT 100；游客稳定编号由 `guest_key`（`sessions.actor_key` 文本身份键，x-internal、不落库不对外）实时派生，**cookie 轮换/清除后编号随之变化**（同一 cookie 内跨周稳定）；旋转 cookie 需真实答题成本；残余风险接受并在 ADR 记录 |
| R4 | **游客同日/同周期重复结算去重** | attempts `UNIQUE(session_id,question_id)` 单会话去重；结算 `UNIQUE(period_start,period_end,scope,bank_id)` + advisory lock 幂等（现每周一次）；若引入日结沿用同键 |
| R5 | **settlement 历史数据兼容**：现有 3 个事件 standings 形状为 `{user_id, rank, nickname, system_avatar, correct_answer_count}`，新形状改变 | 事件不可变（trigger），**不回写**；当前无任何端点消费 settlement 事件，兼容成本为零；未来消费方需同时容忍两形状或按事件创建时间取版本 |
| R6 | **游客归属边界**：claim 前/后的答案归属（写入时真相）导致同一用户名字同时出现在游客与登录条目 | 语义自洽（claim 前答题按游客身份计）；如不可接受，可在 claim 时回写 attempts.user_id（破坏 attempts 不可变 trigger，需先解除 trigger——不建议） |
| R7 | **portal-command 无 cookie 游客**：five-part 命令每次请求可能新发 guest 主体 → 身份碎片化 | 归属键取 sessions.actor_key（创建时写入、不可变）天然缓解；真正无 cookie 的命令路径本就受 session_owner_mismatch 约束（现状如此），不新增风险面 |
| R8 | **platform-core 依赖**：排行读路径新增一跳 | 缓存 + 失败降级为「匿名学习者」保可用性；日志/指标暴露降级率 |
| R9 | **缓存漂移**：display_name 改名 ≤10min 延迟 | 非强一致场景，接受；如需强一致再引入版本号/推送（不做） |
| R10 | **契约漂移窗口**：core 先上线 → 旧 gateway decode 502 | 部署纪律：core+契约+gateway 同发布窗口；新 gateway 对缺 user_id 显式 502 而非误标游客 |
| R11 | **内部契约 user_id 泄露风险**：任何中间层/日志把内部响应打到浏览器 | 网关对外组装显式剥离 + dark 测试断言外部无 user_id 键；ADR-0036 隐私条款保持为硬约束 |
| R12 | **冻结的 profiles 表残留 / 匿名标识持久化**：遗留脏数据/未来误读；`guest_key` 是否落库 | 无读路径即无风险；`guest_key` 是 x-internal 匿名标识，**不落库、不持久化**（「游客x」编号每次由身份键实时派生，身份键即不可变 `sessions.actor_key`），与冻结表一样无需迁移；如需彻底清理 profiles 表，单独 000012 迁移 + 更新 migration_artifacts.go 基线（本期不做） |

---

## 9. 结论

- **推荐路径**：路径 1 —— Go core 内部契约携带 `user_id`（可空），Portal Gateway 经新增 `POST /api/v1/users/display-names`（5 行 HMAC，复用现有 `PLATFORM_CORE_URL` 凭据）批量解析 `users.display_name`，对外剥离 user_id；游客按 `sessions.actor_key` 零迁移归属并显示「匿名学习者」。
- **一句话理由**：display_name 是平台唯一事实来源，读取时实时解析既消灭昵称漂移与 PII 重复存储，又让游客（user_id=null）与未设显示名者自然落到中性标签，同时把隐私边界收在 gateway 一道显式剥离 + 契约校验的硬关卡上（ADR-0036 不变）。
- **产出文件**：`docs/research/ranking-platform-identity.md`

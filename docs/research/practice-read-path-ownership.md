# 决策 D1：Portal `/practice` 读路径最终形态（研究结论）

> 状态：**已定结论（研究完成）** · 对应决策：`docs/migrations/ALL_NEW_STACK_CUTOVER.md` 决策表 D1
> 方法：只读核查主源证据（代码 / ADR / compose / 现状文档），每条结论附证据路径:行号。
> 结论一句话：**选 A——Portal `/practice` 全部读路径收敛为「Gateway 精确路由 → QuizCraft Go core」的契约读，portal-api 直读降级并移除。**
>
> 2026-08-19 更新（服务器实测）：**生产已实测切流**——Gateway → Go core 生效中（catalog/rankings 均 200），portal-api 直读为返回空目录的幽灵路径（`/api/v1/practice/banks` 200-空列表）；本研究结论的方案 A 与生产事实一致，已落地为 ADR-0036（`docs/adr/0036-portal-practice-read-path-owner-go-core.md`）。第 1 节现状表格中标「已过时」的行即 2026-08-19 前的事实基线。

---

## 1. 现状梳理：谁在读、谁在写、契约差异、开关与默认值

### 1.1 三路径并存的事实基线

| 路径 | 读（数据从哪来） | 写（会话/作答） | 生产状态 |
|---|---|---|---|
| quizcraft web-app（superhuazai.me） | React 前端直接调 Go core 契约（`OpenAPI.BASE = VITE_QUIZCRAFT_GO_API_BASE_URL`，`products/quizcraft/web-app/src/api/quizcraftShadowClient.ts:100-105`），银行目录/排行/收藏/反馈全部走 Go 契约 | Go core（`writes` 路由，`go-service/practice_http.go:303-326`，`requireWritesEnabled` 门禁） | ⚠️ 已过时（2026-08-19）：FastAPI 已停服（10086 无监听），Go core 已接管生产（quizcraft-go.service 8-14 起，writes/portal commands 开启） |
| go-service（:10089 影子） | 自身直读 Go 契约表（`quizcraft_*` 表，PostgreSQL 唯一事实源，`go-service/README.md:22`） | Go core；Portal 私有命令路由默认关（`practice_http.go:331-339`） | ⚠️ 已过时（2026-08-19）：go-service 已是**生产服务**（quizcraft-go.service，非影子）；`VITE_QUIZCRAFT_GO_WRITES` 现为 web-app 侧切流开关 |
| Portal `/practice`（henukit.cn 栈） | **portal-api 直读 quizcraft 库的 Go 契约表**（`services/portal-api/internal/practice/db.go`），经 Gateway wildcard 代理 | Gateway 暗命令（默认 503）；portal-api 无任何写端点 | ⚠️ 部分过时（2026-08-19）：portal-api 直读仍在生产但为**幽灵路径**——`/api/v1/practice/banks` 200 空列表（docker quizcraft 库无 Go 表），待部署 M1 代码（ADR-0036）移除 |

### 1.2 Portal 读路径的默认链路（生产现状）

浏览器 → Portal Gateway →（`GET /api/v1/practice/*` wildcard，`services/portal-gateway/internal/httpapi/handler.go:286`）→ `proxyToPortalAPI`（handler.go:1542-1589，GET-only）→ portal-api 路由：

- `GET /api/v1/practice/banks`（router.go:103）
- `GET /api/v1/practice/schools`（router.go:106）
- `GET /api/v1/practice/lists/{id}`（router.go:109）→ `GetQuestions` **返回正确答案**（db.go:148-154 解码 answer，179-189 组装 `Answer: answerIdx`）
- `GET /api/v1/practice/leaderboard`（router.go:112）→ `GetLeaderboard`（db.go:257-301）
- `GET /api/v1/practice/stats`（router.go:115）→ live 模式恒 503（router.go:594-604，无真实统计源）

portal-api 直读 SQL 全部硬编码 Go 契约表名：`quizcraft_banks` / `quizcraft_bank_versions` / `quizcraft_bank_version_questions` / `quizcraft_questions` / `quizcraft_question_versions` / `quizcraft_question_stats`（db.go:25-31, 100-111, 216-222, 260-263）与 `quizcraft_ranking_settlement_events`（db.go:260）。

### 1.3 直读路径的两个实质缺陷（切流前必须解决的证据）

**缺陷 1 — 排行 standings 泄漏用户 UUID。**
- portal-api 按 `{user_id, nickname, score}` 解码 settlement standings，且 **昵称为空时直接用 `user_id` 当显示名**：`db.go:278-282`（decode 结构）、`db.go:290-291`（`if name == "" { name = s.UserID }`）。
- 而 Go 侧 settlement 的 standings 确实**含 user_id**（作为不可变审计事实写入）：`ranking_settlement.go:32-37` 序列化 `ListOverallRankingWindow` 的行（`internal/store/practice.sql.go:800-811` 返回 `user_id`），测试也断言「settlement lacks stable actor evidence」需要 standings 含 userID（`go-service/tests/ranking_test.go:400-402`）。
- 同时新 standings 字段是 `correct_answer_count` 而非 `score`：直读解码 `score` 恒为 0，排行数字全错。**双重损坏：UUID 泄漏 + 排行数值失真。**
- 仓库已点名此风险：`docs/migrations/ALL_NEW_STACK_CUTOVER.md:46`「portal 直读把 quizcraft_ranking_settlement_events 的 standings 当昵称展示（可能泄漏 user UUID）」。

**缺陷 2 — 题目答案随直读下发，违背 ADR-0018 的答案边界。**
ADR-0018 规定「Portal 只从创建的 session 渲染题目内容，只在作答响应后渲染正确性/答案/解析」（`docs/adr/0018-...md:38-39`）；而 portal-api `GetQuestions` 把 `answer` 与 `analysis` 一并返回（db.go:130-189）。尽管当前 Portal UI 未渲染该字段，端点本身已把答案键暴露给任何能访问 `/api/v1/practice/lists/{id}` 的浏览器。

### 1.4 契约差异：直读 vs Go core 读契约

| 维度 | portal-api 直读（现状） | Go core Portal 读契约（V2 reads 备好） |
|---|---|---|
| 目录 | `GetBanks` 返回 `{id,name,subject,question_count}`（db.go:215-252）；`GetSchools` 用 bank_key 拼 school/major 层级（db.go:23-96，无官方层级） | `GET /api/v1/banks` → `{bank_id, bank_version_id, bank_key, name, content_sha256, question_count, chapters}`（`practice_http.go:955-979`；`quizcraft.yaml:806-816`） |
| 排行 | settlement 表 last event，忽略 period 参数（db.go:258 `_ = period`），解码 `{user_id,nickname,score}` | `GET /api/v1/rankings/overall`、`/banks/{bank_id}/rankings`，`{rank,nickname,system_avatar,correct_answer_count}`（`practice_http.go:410-450`；yaml:970-980），**无 user_id 字段**；只有 visible profile 上榜（`practice.sql.go:505-516, 800-811` JOIN `quizcraft_ranking_profiles p ... p.visible`），无 profile 缺席（README.md:9），昵称空 → `匿名学习者`（`practice_http.go:512-540`），32 位 hex/含 @ 昵称被拒防 UUID 伪装（`practice_http.go:542-561`） |
| 统计/收藏/反馈状态 | 无真实源（stats 恒 503） | actor-bound 六段 HMAC 读：`GET /api/v1/stats`、`/portal/practice/favorites`、`/portal/practice/banks/{bank_id}/favorites`、`/portal/practice/feedback/{feedback_id}/status`（`practice_http.go:298-301`） |
| 认证 | 无（Gateway wildcard 无鉴权） | 五段 HMAC `authenticatePortalRead`（`go-service/summary_http.go:121-129`，`portal.practice.read` 权限）；actor-bound 六段 `authenticatePortalPersonalStats`（summary_http.go:131-140，`X-Actor-User-Id` 为第六段，`practice_http.go:1334-1344` portalActorUserID 拒绝 guest/nil/重复） |

**关键事实：Go core 的 Portal 读契约（目录+排行+统计+收藏+反馈状态）在服务端和 Gateway 客户端都已完整实现**，只是默认关闭；web-app 前端已证明 Go core 契约是可用的事实源（`quizcraftShadowClient.ts:127-227`）。

### 1.5 开关与默认值全表（谁控制什么）

| 开关 | 位置 | 默认 | 作用 |
|---|---|---|---|
| `PORTAL_ENABLE_QUIZCRAFT_CATALOG` | `portal-gateway/internal/config/config.go:97` | `0` | Gateway 注册 `/api/v1/practice/catalog` 真实 handler（handler.go:220-225；`quizCraftCatalogPath` handler.go:68） |
| `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG` | `apps/portal/src/lib/api/env.ts:38-40`；Dockerfile:27-34 | `0` | 浏览器才请求/渲染 V2 catalog（`page-client.tsx:33,164-181`） |
| `PORTAL_ENABLE_QUIZCRAFT_V2_READS` | config.go:42,148-190 | `0` | Gateway 注册 `/api/v1/rankings/overall`、`/banks/{bank_id}/rankings`（handler.go:226-231）+ 初始化 `quizCraft` V2 读客户端 |
| `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS` | env.ts:47-49 | `0` | 浏览器显示排行榜 tab / 请求 stats（`practice-nav.tsx:62-63`；`personal-stats.ts:52`） |
| `PORTAL_PRACTICE_COMMANDS_ENABLED` | config.go:110 | `0` | Gateway 命令客户端非 nil（handler.go:139-150）；否则 session/answer/feedback/favorites 路由恒 503（handler.go:887-890） |
| `QUIZCRAFT_WRITES_ENABLED` | go-service README.md:129；`practice_http.go:56` | `0` | Go core 写路由门禁（`requireWritesEnabled`，practice_http.go:944-953） |
| `QUIZCRAFT_PORTAL_COMMANDS_ENABLED` | practice_http.go:46,331-339 | `0` | Go core 注册 `/api/v1/portal/practice/*` 私有写路由 |
| `VITE_QUIZCRAFT_GO_WRITES` | `web-app/src/api/quizcraftRollout.ts:1-7` | 未设 | web-app 原子切流：1 时全部读写走 Go（README.md:129） |
| 已退役浏览器管理开关 | 不再配置 | 未设 | 不得恢复独立管理前端 |

Gateway 侧三个服务端开关 + 浏览器侧两个烘焙开关，共 **5 个开关控制 Portal 的 QuizCraft 读**，切流时必须两两对齐（服务端 + 浏览器），且 ADR-0018 明示「仅靠改 Portal UI 无法启用边界，必须服务端 gate 与独立密钥对一起配置」（ADR-0018:53-55）。

### 1.6 `PRACTICE_SERVICE_URL` 四路复用（最大接线雷）

`PRACTICE_SERVICE_URL` 是 `mustEnv`（config.go:94），同时被 4 处消费，语义互相冲突：

1. **命令客户端 baseURL**：`practice.NewCommandClient(cfg.PracticeURL, ...)`（handler.go:142，写）。
2. **目录读客户端 baseURL**：`practice.NewClient(cfg.PracticeURL, ...)`（handler.go:154-157，读）。
3. **生产 compose 默认值 = portal-api**：`docker-compose.henukit.yml:393` `PRACTICE_SERVICE_URL: ${PRACTICE_SERVICE_URL:-http://portal-api:8085}`；`.env.henukit.example:72` 同。即默认语义是「portal-api 直读代理地址」。
4. **prebuilt compose 强制 = QuizCraft Core**：`docker-compose.henukit.prebuilt.yml:204` `PRACTICE_SERVICE_URL: ${QUIZCRAFT_CORE_URL:?...}`。即 prebuilt 语义是「Go core 地址」。

而 V2 读客户端又用**独立的** `QUIZCRAFT_CORE_URL`（config.go:43-44,180；`QUIZCRAFT_PORTAL_CATALOG_*` 凭据，config.go:182-187）。于是同一个变量在默认 compose 里表达「portal-api」，在 prebuilt 里表达「Core」——中间态（目录/命令走 Core、公共读仍走 portal-api）**无法用这组变量表达**。切流接线若只改 `PRACTICE_SERVICE_URL`，会把命令/目录客户端指向 portal-api（无此路由）或把全部读隐式指向 Core（未完成验证）。仓库两份文档点名：`ALL_NEW_STACK_CUTOVER.md:10`「PRACTICE_SERVICE_URL 四路复用是最大接线雷」、`:33`「无论选哪个，都要出一份接线矩阵文档裁决」。

### 1.7 生产现状与 ADR 边界

- 生产 `.env.henukit.prod` 未设置 `QUIZCRAFT_CORE_URL`、`PORTAL_ENABLE_QUIZCRAFT_*` → 暗命令与 V2 reads 全部默认关闭（`CURRENT_PRODUCTION_STATE.md:47-48,60`）。
- ADR-0013（portal-gateway-go）：「Portal Gateway 默认只读代理」「Gateway 不连接产品数据库」「V2 目录预接入路径 `/api/v1/practice/catalog` 默认被 wildcard 短路 404」（`docs/adr/0013-portal-gateway-go.md:14,17,22`）。
- ADR-0018：「Gateway 恰好转发两个默认关闭的写命令」；读保持默认只读（`0018:8`）；收藏/排行/反馈在例外之外，需各自显式决策（`0018:50-52`）；#161 只准备暗路径、#166 是唯一切流窗口（`0018:34-37`）。
- QuizCraft CONTEXT.md：Go core **Owns** Practice / Favorites / Ranking / Feedback / Workshop 契约（`products/quizcraft/CONTEXT.md:33-37`）。
- 执行规格：公开排行只暴露受控昵称与系统头像、**不暴露账户标识**（`docs/development/henukit-console-executable-spec.md:147`）；QuizCraft 数据以稳定 bank/question 身份与不可变版本建模，运行时 JSON 兜底不是生产数据源（:141）。

---

## 2. 方案对比

**A：读也走 Gateway → Go core**（Portal UI 换 V2 catalog/rankings 数据源，portal-api 直读降级并移除）
**B：portal-api 直读 DB 正式转正为 practice 读路径 owner**（补 ADR 划界，Go core 只做写）
**A′（第三方案）：portal-api 直读保留但契约化**——portal-api 不再直写 SQL，改为实现/转发 quizcraft.yaml 同一份读契约（等价于把 portal-api 变成 Go core 读契约的第二个实现/代理），写仍归 Go core。

| 维度 | A（推荐） | B | A′ |
|---|---|---|---|
| **数据一致性** | ✅ 单一读实现（Go core 契约），读与写同一事实源；目录/排行/统计都从不可变 attempt/version 派生 | ❌ 双读来源长期并存：直读 SQL 与 Go 契约各自演化，settlement standings 与 ranking profile 两套排行口径 | ⚠️ 读契约单一，但仍有**两个读实现**（Go core + portal-api 代理），需契约漂移测试持续背书；多一跳服务 |
| **隐私（UUID/答案）** | ✅ 排行响应无 user_id（practice_http.go:432,441；yaml:970-980），昵称空→匿名学习者，profile 不可见即缺席；答案只经 session/answer 边界下发（ADR-0018:38-39） | ❌ 保留 `nickname 空 → user_id 当显示名`（db.go:290-291）与答案随题目下发（db.go:148-189）两个已知缺陷；要修就得给直读补过滤逻辑，等于在 portal-api 里重建 Go core 已做的隐私契约 | ✅ 可修（按契约剥掉 answer、standings 改 correct_answer_count），但要复制 Go core 的昵称归一化/防 UUID 昵称/可见性逻辑，容易再漂移 |
| **运维接线（四路复用）** | ✅ 可借机收敛 `PRACTICE_SERVICE_URL` 为「Core 地址」单一语义，删除 wildcard→portal-api 的 practice 段，出接线矩阵 | ❌ 必须永久维持 `PRACTICE_SERVICE_URL`（portal-api 语义）与 `QUIZCRAFT_CORE_URL`（Core 语义）并行，四路复用成为长期事实；命令/目录读与公共读指向两个目标 | ⚠️ 直读端点仍走 wildcard→portal-api，`PRACTICE_SERVICE_URL` 的 portal-api 语义继续存在，与命令客户端（Core 语义）冲突依旧 |
| **改动量** | 中：Gateway 路由微调 + 接线矩阵 + 开关迁移 + portal-api 删读（UI 的 V2 catalog/stats 已实现，`page-client.tsx:164-181`、`personal-stats.ts`、`client.ts:548-567`） | 小（表面）：只补 ADR；但要修 UUID/答案缺陷、养双读源测试，长期维护成本高 | 中：portal-api 重写读层为契约代理 + 契约漂移 CI + 凭据分发（portal-api 需要 Go core 读凭据，凭据多一份副本） |
| **与 ADR-0018/0013 一致性** | ✅ 对齐 ADR-0013-portal-gateway-go（Gateway 是默认只读产品代理、不连产品库）、ADR-0018（写走 Gateway 暗命令、读保持 Gateway 侧）、CONTEXT.md（QuizCraft owns 契约）、规格 :147 隐私 | ❌ 与规格 :147 相悖（除非专门修直读）；把「读 owner」放在 Gateway 之外，与 ADR-0013「Gateway 不连接产品数据库、只读代理」的架构方向相反；ADR-0018 的 #161/#166 暗路径将部分作废 | ⚠️ 与 ADR-0013 方向冲突（读代理应位于 Gateway，而非 portal-api），需再补一份 amends |
| **风险** | 中：切流窗口内 UI 数据源切换 + 开关对齐失败会 503（fail-closed，可接受）；移除直读前需确认无其他消费方 | 高：双读源漂移迟早产生用户可见不一致；UUID 泄漏是隐私事故候选；与既有 ADR 方向相悖需推翻 #161/#166 的部分准备 | 中：契约漂移测试是永久负担；读链路多一跳增加故障面 |

**结论矩阵：A 在数据一致性、隐私、接线、ADR 一致性四项全绿，改动量可控；B 只赢在表面改动量；A′ 是 B 的加固版但引入了第二读实现与凭据副本，不解决四路复用。**

---

## 3. 明确推荐：**A**（读走 Gateway → Go core，portal-api 直读降级并移除）

理由（每条可辩护）：

1. **对齐既有 ADR 方向，无需推翻任何已接受决策**：ADR-0013-portal-gateway-go 已定「Gateway 是默认只读产品代理、不连接产品数据库」，ADR-0018 已把写收敛到 Gateway 暗命令并要求 #166 唯一切流窗口。A 只是把「读」补进同一方向（Gateway 精确路由 → Core 契约），与 #161/#166 的暗路径准备完全兼容；B 则需要为 portal-api 直读另立一份 ADR，与「Gateway 只读代理」架构相反。
2. **单一事实来源**：读与写最终都由 Go core 从同一 PostgreSQL 事实（不可变 bank/question 版本、Scored Attempt、Ranking Profile）派生；portal-api 直读是第二条 SQL 实现，任何契约演进都要改两处，A 消灭这个长期负债。web-app 已证明 Go core 契约可独立支撑完整读面。
3. **一次性解决 UUID 泄漏**：Go core 排行契约在**响应层就没有 user_id**，且 profile 可见性、昵称归一化、`匿名学习者`、32 位 hex 昵称拦截全部内置并有测试断言「响应不含 user_id」（`go-service/tests/ranking_test.go:96,252,262`）；portal-api 的 `db.go:290-291` 昵称回退是缺陷本体，A 直接删掉它，而不是在 portal-api 里再造一套过滤。
4. **消除 PRACTICE_SERVICE_URL 四路复用陷阱**：A 的落地把该变量收敛为单一语义「QuizCraft Core」，删掉 wildcard→portal-api 的 practice 代理段，与 `QUIZCRAFT_CORE_URL` 的关系用接线矩阵一次定死（见 §4.4），不再长期维护两种读目标。
5. **改动量被前端已就绪的事实抵消**：Portal UI 的 V2 目录、排行、统计数据源与失败语义已实现（`page-client.tsx:164-181`、`client.ts:548-567`、`personal-stats.ts`、`practice-nav.tsx:62-63`），剩余工作是网关路由收敛、开关迁移与 portal-api 删读，属于切流窗口内的确定性改动，而非从零开发。

---

## 4. A 方案落地改动清单

### 4.1 Gateway 路由（services/portal-gateway/internal/httpapi/handler.go）

1. 把 `/api/v1/practice/catalog`、`/api/v1/rankings/overall`、`/api/v1/rankings/banks/{bank_id}`、`/api/v1/practice/stats`、favorites/feedback 状态读注册为**常态精确路由**（不再依赖 `PORTAL_ENABLE_QUIZCRAFT_*` 才注册），V2 读门禁收敛为一个总开关（如 `PORTAL_ENABLE_QUIZCRAFT_V2_READS`）或直接并入命令门禁。
2. 从 `r.Get("/api/v1/practice/*", h.proxyToPortalAPI)`（handler.go:286）的代理面中**移除 practice 段**：`/api/v1/practice/banks`、`/schools`、`/lists/*`、`/leaderboard` 不再代理到 portal-api，改为映射到 Core 契约路径或显式 404/迁移提示。
3. 暗命令路由（handler.go:236-250）保持现有语义，仅受 `PORTAL_PRACTICE_COMMANDS_ENABLED` 控制。

### 4.2 Portal UI 数据源（apps/portal）

1. `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG` / `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS` 与 `PORTAL_ENABLE_QUIZCRAFT_*` 在切流构建中**同设为 1**（`scripts/ops/henukit-release-images.sh:129` 已示范），之后逐步把两个浏览器开关并入单一构建开关并最终删除。
2. `/practice` 目录页删除 legacy schools/banks→gateway 缓存→mock 的回退链（`page-client.tsx:182-263`），只保留 V2 catalog 严格路径（失败即 error，已有实现）。
3. 排行榜页迁移到 `/api/v1/rankings/overall`（`client.ts:553-559` 已实现），下线 `/api/v1/practice/leaderboard` 消费。
4. 保留 `/practice/quiz` 的 session 命令流不变（已按 ADR-0018 实现）。

### 4.3 portal-api 移除直读（services/portal-api）

1. 删除 `internal/practice/db.go`（`GetSchools/GetQuestions/GetBanks/GetLeaderboard`）与 router.go 中 5 个 practice 路由（router.go:103-117）。
2. 从 `NewRouter` 移除 `QUIZCRAFT_DATABASE_URL` 连接（router.go:36-39,49-52）。
3. 确认无其他消费方（gateway wildcard 已删）后，从 compose/.env 移除 `QUIZCRAFT_DATABASE_URL` 接线。
4. `internal/practice/mock.go` 若不再被 UI 引用一并删除（`mockAllowed` 仅限本地开发，`env.ts:28-31`）。

### 4.4 接线矩阵（新文档或并入 ADR）

| 变量 | 切流后语义 | 说明 |
|---|---|---|
| `PRACTICE_SERVICE_URL` | **QuizCraft Core 服务地址（唯一语义）** | 命令 + 目录读共用；默认 compose 的 `http://portal-api:8085` 值删除（`docker-compose.henukit.yml:393`、`.env.henukit.example:72`），prebuilt 的强制方式（`.prebuilt.yml:204`）保留 |
| `QUIZCRAFT_CORE_URL` | 与 `PRACTICE_SERVICE_URL` 二选一（建议合并为后者，或明确定义前者=V2 读、后者=命令/目录读的职责分离） | 当前两变量并存是四路复用根因之一 |
| `PORTAL_ENABLE_QUIZCRAFT_CATALOG` / `_V2_READS` | 切流后与命令门禁同设 1，最终并入 `PORTAL_PRACTICE_COMMANDS_ENABLED` 等价的总门 | 服务端与浏览器端开关必须两两对齐（ADR-0018:53-55） |
| `QUIZCRAFT_PORTAL_CATALOG_*` / `PRACTICE_COMMAND_*` 凭据 | 读凭据（catalog key ring）与写凭据（command key ring）**强制不同**（`practice_http.go:199-201,228-237` 已强制） | 保持不变 |

### 4.5 契约与测试

1. 排行榜隐私契约测试已存在（`go-service/tests/ranking_test.go:96,252,262`），切流后把「响应不含 user_id、nickname 空→匿名学习者」纳入 Gateway 侧契约测试（`services/portal-gateway/internal/httpapi/quizcraft_dark_test.go` 扩展）。
2. Gateway V2 读客户端 `client.go` 已有严格校验（`validateRanking` 检查 `correct_answer_count`、拒绝 32 位 hex 昵称，client.go:465-511）——保留并作为回归。
3. 删除/归档 portal-api 的 practice 相关测试（`services/portal-api/internal/httpapi/router_test.go` 中对应用例）。
4. Browser gate 双视口（desktop + 390px）覆盖练习/收藏/反馈/排行（`ALL_NEW_STACK_CUTOVER.md:40`）。

### 4.6 开关迁移顺序（切流窗口内）

1. 服务器核验 quizcraft 库表结构与 `quizcraft_migration_events` 版本（`CURRENT_PRODUCTION_STATE.md:81`）。
2. `migrate`/`reconcile` 全量对账、`quizcraft_v2` 快照、`run_id` 固化（README.md:62-125）。
3. 接线矩阵落文档 → compose 变量收敛（§4.4）。
4. #166 窗口：三个服务端开关 + 两个浏览器开关同设 1，发布单个切流 bundle（对齐 ADR-0018:34-37 与 README.md:129 的原子发布方式）。
5. Browser gate 通过后：portal-api 移除直读（§4.3），FastAPI 停服脱离 Nginx → 只读冷备 7 天 → 人工批准移除（ADR-0013-cutover:13）。

---

## 5. 新 ADR 草案要点（可直接转成 `docs/adr/0036-xxx.md`）

标题建议：**0036-portal-practice-read-path-owner-go-core.md**（status: proposed，amends: 0013-portal-gateway-go, 0018）

1. **读路径收敛**：Portal `/practice` 的所有读（目录 `/api/v1/practice/catalog`、排行 `/api/v1/rankings/*`、个人统计/收藏/反馈状态的 actor-bound 读）一律由 Portal Gateway 精确路由转发到 QuizCraft Go core 的 Portal 读契约；portal-api 直读 quizcraft 库路径（`internal/practice/db.go`）降级为过渡并在切流后删除。amends ADR-0013-portal-gateway-go 的「默认只读代理」读侧细节，读入方向不变。
2. **隐私契约强制**：公开排行响应只含 `rank / nickname / system_avatar / correct_answer_count`，禁止出现用户 UUID；profile 不可见者不入榜，昵称留空用中性标签「匿名学习者」；题目答案与解析只在 session 作答响应后下发。切流前，直读阶段的 settlement standings 展示同样遵守（禁止 user_id 回退昵称）。
3. **接线单一化**：`PRACTICE_SERVICE_URL` 收敛为「QuizCraft Core 服务地址」的单一语义，废弃其指向 portal-api 的默认值；发布接线矩阵裁决其与 `QUIZCRAFT_CORE_URL`、`PORTAL_ENABLE_QUIZCRAFT_CATALOG`/`PORTAL_ENABLE_QUIZCRAFT_V2_READS`、浏览器烘焙开关的关系，作为 #166 切流的配置门禁。
4. **切换门禁**：#166 是唯一切流窗口；三个服务端开关与两个浏览器开关必须同设 1 并两两对齐；Browser gate（desktop + 390px）验证练习/收藏/反馈/排行后，portal-api 才可移除直读；读失败为 honest 503/error，绝无 mock/legacy 兜底。
5. **凭据边界不变**：目录/排行读凭据（catalog key ring）与命令写凭据（command key ring）强制不同且不可共享；Gateway 不持有产品库连接（ADR-0013:22 不变），QuizCraft owns Practice/Favorites/Ranking/Feedback/Workshop 契约（CONTEXT.md:33-37 不变）。

---

## 附：证据索引（本节所有行号均已在上文引用处标注，汇总如下）

- Gateway 路由/客户端：`services/portal-gateway/internal/httpapi/handler.go:68,139-174,220-250,285-286,1542-1589`；`internal/config/config.go:33-56,94-110,148-190`；`internal/httpapi/quizcraft_catalog.go:14-53`；`internal/httpapi/quizcraft_rankings.go:44-75`；`internal/practice/client.go:80-193,212-380,465-511`；`internal/practice/contract_generated.go:6-18`
- portal-api 直读：`services/portal-api/internal/practice/db.go:23-96,99-196,215-252,257-301`；`internal/httpapi/router.go:36-52,103-117,481-618`
- Go core 契约：`products/quizcraft/go-service/practice_http.go:290-339,410-450,512-561,955-979,1334-1344`；`summary_http.go:121-140`；`ranking_settlement.go:17-62`；`internal/store/practice.sql.go:505-516,800-811`
- 契约形状：`packages/api-contracts/openapi/quizcraft.yaml:83-127,191-212,427-479,806-816,965-1005`
- 前端开关：`products/quizcraft/web-app/src/api/quizcraftRollout.ts:1-7`；`quizcraftShadowClient.ts:100-105,127-227`；`client.ts:212-287`；`apps/portal/src/lib/api/env.ts:38-49`；`apps/portal/src/app/practice/page-client.tsx:33,161-263`；`apps/portal/src/lib/practice/personal-stats.ts:48-95`；`apps/portal/src/lib/gateway-init.ts:32-44`
- 现状/接线：`docs/operations/CURRENT_PRODUCTION_STATE.md:41-60`；`docs/migrations/ALL_NEW_STACK_CUTOVER.md:10,30-33,46`；`docker-compose.henukit.yml:393-412,465-468`；`docker-compose.henukit.prebuilt.yml:204`；`.env.henukit.example:72-83`
- ADR/规格：`docs/adr/0013-portal-gateway-go.md:14-22`；`docs/adr/0018-portal-quizcraft-practice-command-boundary.md:8,34-55`；`docs/adr/0013-quizcraft-maintenance-window-full-cutover.md:7-13`；`docs/development/henukit-console-executable-spec.md:141-152`；`products/quizcraft/CONTEXT.md:33-47`

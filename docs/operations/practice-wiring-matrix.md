# Practice 接线矩阵（ADR-0036 / #166 切流配置门禁）

> 状态：**ADR-0036 已接受的实施接线**。本文是 #166 切流窗口的配置门禁：
> 任何部署若要启用 Portal `/practice` 的 QuizCraft 读/写，必须按本表一次性对齐
> 服务端开关、浏览器烘焙开关与地址变量，两两配对，禁止部分启用。
> 对应决策：`docs/adr/0036-portal-practice-read-path-owner-go-core.md`；
> 研究依据：`docs/research/practice-read-path-ownership.md` §1.5 / §1.6 / §4.4。

## 1. 一句话接线图

```
浏览器 ── /api/v1/practice/catalog、/api/v1/rankings/*、/api/v1/practice/stats、
          /api/v1/practice/favorites*、/api/v1/practice/feedback/{id}/status（读）
       ── /api/v1/practice/sessions|answers|feedback、favorites 写（命令）
            │
            ▼
      Portal Gateway（精确路由，ADR-0036）
            │ 读写走 QuizCraft Core 地址（PRACTICE_SERVICE_URL 单一语义）
            ▼
      QuizCraft Go core（:10089，quizcraft 库唯一直读方；方案 2 已容器化为 compose 服务 `quizcraft`）
```

- Portal Gateway 的 `PORTAL_ENABLE_QUIZCRAFT_CATALOG` / `PORTAL_ENABLE_QUIZCRAFT_V2_READS`
  只决定**读客户端是否存在**，不再决定路由是否注册；路由常态注册，客户端缺省时
  诚实 404（公共读）或 503（actor-bound 读）。
- portal-api 不再直读 quizcraft 库（`internal/practice/` 已删），也不再接收
  `/api/v1/practice/{banks,schools,lists,leaderboard,stats}` 的代理。
- Gateway 不持有任何产品数据库连接（ADR-0013 不变）。

## 2. 变量总表（#166 切流必须同设）

| 变量 | 默认（fail-closed） | 切流值（#166） | 语义 / 位置 |
|---|---|---|---|
| `PRACTICE_SERVICE_URL` | compose 默认 `http://portal-api:8085`（本地占位） | **QuizCraft Core 地址**（方案 2 容器化后：`http://quizcraft:10089`；旧 env 值 `http://host.docker.internal:10089` 仅过渡兼容） | **唯一语义 = QuizCraft Core 服务地址**：命令客户端 + catalog 读客户端共用。`mustEnv`（`portal-gateway/internal/config/config.go`）。生产 prebuilt compose 已强制（`docker-compose.henukit.prebuilt.yml`） |
| `QUIZCRAFT_CORE_URL` | 空 | Core 地址（可与 `PRACTICE_SERVICE_URL` 相同或不同） | V2 读客户端（rankings/stats/favorites/feedback）baseURL。与 `PRACTICE_SERVICE_URL` 的关系：两者职责分离，V2 读走 `QUIZCRAFT_CORE_URL` + `QUIZCRAFT_PORTAL_CATALOG_*` 凭据，命令/目录读走 `PRACTICE_SERVICE_URL` + `PRACTICE_*`/`PRACTICE_COMMAND_*` 凭据。**合并建议**：后续可把 `QUIZCRAFT_CORE_URL` 并入 `PRACTICE_SERVICE_URL`，本窗口保持并存但两两同设 |
| `PORTAL_ENABLE_QUIZCRAFT_CATALOG` | `0` | `1` | Gateway catalog 读客户端（`/api/v1/practice/catalog`）。**独立于 V2 读**：目录读凭据 `PRACTICE_*` 与 V2 读凭据 `QUIZCRAFT_PORTAL_CATALOG_*` 不同源，保持独立开关以免误并 |
| `PORTAL_ENABLE_QUIZCRAFT_V2_READS` | `0` | `1` | **V2 读总门禁**：rankings（`/api/v1/rankings/*`）、stats、favorites 读、feedback 状态读共用一个客户端，一开关全开/全关 |
| `PORTAL_PRACTICE_COMMANDS_ENABLED` | `0` | `1` | **命令（写）门禁**：session/answer/feedback/favorites 写。与读门禁**必须独立**——命令凭据 `PRACTICE_COMMAND_*` 与读凭据强制不同，读并入命令门禁会把读写可用性错误耦合 |
| `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG` | `0` | `1`（构建时烘焙） | 浏览器目录页是否请求/渲染 V2 catalog（`apps/portal/src/lib/api/env.ts`） |
| `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS` | `0` | `1`（构建时烘焙） | 浏览器排行榜 tab / stats 请求（`personal-stats.ts`、`practice-nav.tsx`） |
| `NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY` | `0`（dev）/ `1`（prod） | `1` | 强制真实 Gateway、禁 mock（生产必须 `1`） |

## 3. 默认值与烘焙的关系（消除歧义）

- **仓库默认全部为 0/空（fail-closed）**：compose（`docker-compose.henukit.yml`）、
  `.env.henukit.example`、网关 `config.go`、浏览器 `env.ts` 的默认一致。
- **`scripts/ops/henukit-release-images.sh` 把两个浏览器开关烘焙为 1**：该清单描述的
  是 **#166 切流发布构建**。烘焙 1 与网关默认 0 的表面不一致是**有意的**：
  用该脚本产出的发布镜像**必须**在同一个发布 bundle 里把三个服务端开关
  （`PORTAL_ENABLE_QUIZCRAFT_CATALOG`、`PORTAL_ENABLE_QUIZCRAFT_V2_READS`、
  `PORTAL_PRACTICE_COMMANDS_ENABLED`）与 `PRACTICE_SERVICE_URL`/`QUIZCRAFT_CORE_URL`
  同设，否则浏览器渲染读面而 Gateway 返回诚实 404/503（绝不 mock/legacy 兜底）。
- 本地/预发布 compose 构建参数默认 0，属正常关闭状态；**不允许**「浏览器开、
  网关关」之外的任何部分启用组合作为生产长期状态。

## 4. 凭据对（强制不同，不可共享）

| 凭据组 | 用途 | 变量 |
|---|---|---|
| catalog key ring（读） | catalog 读客户端（`PRACTICE_SERVICE_URL`） | `PRACTICE_CLIENT_ID/SECRET/KEY_ID` |
| V2 读 key ring（读） | rankings/stats/favorites/feedback 读客户端（`QUIZCRAFT_CORE_URL`） | `QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID/SECRET/KEY_ID` |
| command key ring（写） | session/answer/feedback/favorites 写客户端 | `PRACTICE_COMMAND_CLIENT_ID/SECRET/KEY_ID` |

读凭据（catalog / V2）与写凭据（command）**强制不同**（Go core `practice_http.go`
已强制校验）。Gateway 只持有服务凭据，不持有产品库连接。

## 5. 路由与数据源（切流后契约）

| 浏览器端点 | 数据源 | 未启用时（fail-closed） |
|---|---|---|
| `GET /api/v1/practice/catalog` | Core 目录契约（catalog 客户端） | 404 |
| `GET /api/v1/rankings/overall`、`/api/v1/banks/{bank_id}/rankings` | Core 排行契约（V2 客户端） | 404 |
| `GET /api/v1/practice/stats` | Core 个人统计（V2 客户端） | 503 |
| `GET /api/v1/practice/favorites`、`/banks/{bank_id}/favorites`、`/feedback/{feedback_id}/status` | Core actor-bound 读（V2 客户端） | 503 |
| `POST /api/v1/practice/sessions`、`.../answers`、`/feedback`、favorites 写 | Core 命令（命令客户端） | 503 |
| `GET /api/v1/practice/banks`、`/schools`、`/lists/{id}`、`/leaderboard` | **已下线**（ADR-0036，portal-api 直读删除） | 404 + 迁移提示 |

排行隐私契约：公开排行响应只含 `rank / nickname / system_avatar / correct_answer_count`，
无 `user_id`；昵称空 → 匿名学习者（Core 归一化）；网关侧 `DisallowUnknownFields`
拒绝任何含 `user_id` 的 Core 排行响应（Gateway 契约测试已覆盖）。

## 6. #166 切流检查单（配置门禁）

1. 服务器核验 quizcraft 库表结构与 `quizcraft_migration_events` 版本（`docs/operations/CURRENT_PRODUCTION_STATE.md`）。
2. `migrate`/`reconcile` 全量对账、`quizcraft_v2` 快照、`run_id` 固化（`products/quizcraft/go-service/README.md`）。
3. 本矩阵核对：上述变量两两对齐（服务端 × 浏览器 × 地址）。
4. 发布单个切流 bundle：构建脚本烘焙 1 的 Portal 镜像 + 服务端开关同设 1 + `PRACTICE_SERVICE_URL`/`QUIZCRAFT_CORE_URL` 指向 Core。
5. Browser gate（desktop + 390px）验证练习/收藏/反馈/排行通过后，才允许移除
   portal-api 残留直读（已删）并停服 FastAPI（ADR-0013-cutover）。
6. 读失败为诚实 503/404，禁止任何 mock/legacy 兜底（ADR-0036 强制）。

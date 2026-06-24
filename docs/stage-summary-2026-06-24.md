# 本阶段总结：V2 MVP 收敛与支付硬化

日期：2026-06-24

本文档对照原始「一站式学习平台 V2 全量重构版」计划，说明当前阶段实际做了什么、哪些还没有做、哪些仍只是 mock / MVP / 预留边界。结论以当前仓库代码、测试和文档为准，不把未联调或未上线能力写成生产可用。

## 1. 当前阶段定位

当前处于 V2 MVP 收敛与上线前硬化阶段。

本阶段不是继续大范围扩功能，而是把已经形成的 V2 monorepo、Go API、Web、Admin、Worker、课程资料、课程包、支付权限边界和文档状态收敛清楚。旧版 Next.js + Prisma 只保留在 `legacy/v1-next-prisma` 作为参考，不再作为运行依赖。

当前项目还不能声明为生产上线版本。主要原因是：真实微信商户环境尚未端到端联调，完整 E2E / 移动端截图回归 / 生产部署硬化仍不足，会员积分、真实 AI、搜索排行榜和社交关系模块仍未完全闭环。

## 2. 本阶段完成了什么

### 2.1 工程骨架

- 建立 V2 monorepo：`apps/web`、`apps/admin`、`services/api`、`services/worker`、`docs`、`infra`、`scripts`、`legacy`。
- 将旧版 Next.js + Prisma 代码归档到 `legacy/v1-next-prisma`。
- 核心业务后端转移到 Go API；Next.js 主站不再承担核心业务 API。
- 添加 Docker Compose 本地开发配置和统一 `.env.example`。
- README 和核心文档已覆盖架构、API、数据库、开发、部署、安全、AI 工作流、积分会员、后台使用说明和路线图。

### 2.2 Go API 与数据库

- Gin/GORM API 服务骨架已经可运行。
- 已实现配置读取、PostgreSQL/GORM、Redis 基础连接、统一响应、统一错误、中间件、CORS、Recover、请求日志、基础限流、安全响应头和生产 CORS 配置校验。
- 新增 `go run ./cmd/preflight -env-file ../../.env.production` 生产配置预检，用于上线前检查 `APP_ENV`、CORS、自动迁移、固定验证码、数据库/Redis 占位符、JWT key、微信 Native live 配置、notify URL、证书/私钥路径和上传目录。
- 已实现 health/version 接口。
- 已建立 V2 SQL migration 与 GORM model 基础。
- Seed 可以生成演示学校、学院、专业、课程、资料、课程包、题目、社区内容、AI mock task 和演示账号。

### 2.3 登录与权限

- 已实现邮箱验证码登录、JWT cookie/token 和用户角色。
- 已实现普通用户、创作者、审核员、管理员、超级管理员等角色边界。
- 已实现冻结用户写操作限制。
- Web `/me` 可维护学校、专业、年级绑定。
- 管理后台用户管理由服务端鉴权，不依赖前端隐藏按钮。

### 2.4 课程资料与下载保护

- 已实现学校 / 学院 / 专业 / 课程基础 API。
- 已实现课程资料 public / admin API。
- 已实现资料上传基础防护：文件名处理、后缀限制、MIME / 内容校验、大小限制、路径穿越防护。
- 已实现资料下载权限：`free`、`login_required`、`paid`，并预留 `member_only`。
- paid 资料必须经过 Go API 服务端权限判断，不能只靠前端隐藏按钮。
- 成功下载会写入服务端审计日志；被拒绝下载、不安全路径和缺失文件不会被记为成功下载。
- PDF 下载会生成临时水印副本，源文件不会被覆盖；非 PDF 保持原样下载。
- 公开资料列表/详情、课程资料列表和公开课程包内资料使用脱敏 DTO，不暴露 storage key、创建人或审核元数据。

### 2.5 课程包、订单与权益

- 已实现课程包 catalog、详情、包内资料绑定和包级授权解锁。
- 公开课程包接口只返回 published 课程包与 published 资料，避免泄露 draft / pending / rejected / archived 资料 item。
- 订单创建读取服务端课程包价格，客户端不能传金额改价。
- 已实现用户作用域订单查询和只读订单状态查询。
- 管理后台订单页是只读，不允许人工标记 paid 或直接发放支付权益。
- 已实现微信 Native 关单基础接口：
  - `POST /api/v1/payments/wechat/close` 关闭 pending/paying 订单。
  - 用户只能关闭自己的订单，admin/super_admin 可以关闭任意 pending/paying 订单。
  - paid/closed 订单不能关闭。
  - closed 订单不会被后续课程包下单复用。
- 已实现微信 Native 订单过期收敛：
  - Native 下单会写入 `orders.expires_at`。
  - 状态查询、重复下单复用、Native 支付创建和 admin 订单列表会先把过期 pending/paying 订单置为 `expired`。
  - expired 订单不能继续拉起二维码，不能通过 close 接口关闭，也不会被新课程包订单复用。
- Admin 订单查询可展示并筛选 `risk_flag`，用于支付异常排查；当前还不是自动告警或自动对账系统。
- Payment incident ledger has been added for rejected WeChat callback anomalies (`order_not_found`, `amount_mismatch`, `transaction_conflict`). Admins can mark incidents `resolved` or `ignored`; this writes operation logs only and never marks orders paid or grants entitlement. Vue Admin Dashboard now surfaces the open incident count as a basic operator prompt, and newly opened incidents can optionally emit a signed best-effort webhook without raw notify payloads.
- Payment reconciliation now has an admin-only, read-only local report for order/payment-record/order-grant/risk-flag/open-incident consistency issues. It is not a live WeChat merchant settlement reconciler and does not mutate orders, incidents, payment records, or entitlements.
- 已实现资料 manifest 导入基础版：
  - `data/material-manifest.example.json` 提供示例。
  - `go run ./cmd/import-materials -dry-run <manifest.json>` 可预检已经准备好的课程资料，不写入数据库。
  - `go run ./cmd/import-materials <manifest.json>` 可导入已经准备好的课程资料。
  - 导入会 upsert 学校、学院、专业、课程、课程包和资料。
  - 导入会幂等绑定课程包资料 item。
  - 文件必须真实存在并位于 `LOCAL_UPLOAD_DIR` 内。
  - 路径穿越和缺失文件会拒绝并回滚事务。
  - UTF-8 BOM manifest 可以正常导入，避免 Windows 工具写出的 JSON 被误拒。
- 已补自动化 manifest delivery smoke：
  - 测试夹具通过 importer 导入临时挂载文件。
  - 通过 HTTP API 验证公开课程包详情不泄露 storage key。
  - 验证 free、login_required、paid 未授权拒绝、课程包授权解锁 paid 和下载日志。
  - 外部 `cmd/smoke` 支持 `-grant-package-access`，可在目标环境用 fresh student/admin 测试账号验证“未授权 paid 403 -> admin 手动课程包授权 -> paid 下载 200”的内测交付链路。
  - Web workspace 新增 Playwright browser delivery smoke，可打开真实 Web/Admin 会话验证同一条手动交付路径。
  - 已用本地临时 Postgres、临时 `LOCAL_UPLOAD_DIR` 和临时 paid `.txt` 课程包跑通一次 browser delivery smoke；这证明测试脚本和服务链路可执行，但不替代真实 PDF 内测资料验收。
  - 真实内测资料仍需要在目标环境挂载后单独跑导入和下载验收。

### 2.6 微信 Native 支付硬化

支付方向已从原始 V2 plan 中的「易支付」调整为「微信支付 Native」。易支付不是当前默认方案。

已完成：

- 开发/测试环境 `WECHAT_PAY_MODE=mock` 可生成 mock Native `codeUrl`。
- mock 下单只把订单置为 `paying`，不会标记 paid，也不会发放 entitlement。
- 生产环境禁止 mock 支付。
- 开发/测试环境 mock notify 必须带 HMAC 头，且校验订单号和金额。
- mock notify 成功后会把订单置为 `paid`，写入 `payment_records`，并幂等发放一次课程包 entitlement。
- `cmd/smoke` 已新增开发/测试专用 `-mock-wechat-pay` 验收链路，可创建/复用订单、请求 mock Native codeUrl、发送签名 mock notify、验证订单 `paid`、验证 entitlement 和 paid 下载。
- Web workspace 新增 opt-in browser mock-payment smoke，可验证真实 Web 课程包页二维码展示、后端签名 mock notify、订单 paid、entitlement 和 paid 下载；它只适用于 development/test，不替代真实商户 E2E。
- live Native 下单会使用商户私钥签名请求，并在保存 `code_url` 前校验微信响应签名。
- 已实现微信支付 API v3 密码学基础件：请求签名、Authorization header、平台证书/公钥解析、notify 原文验签、AES-256-GCM resource 解密。
- 本阶段新增 live notify handler：
  - 校验微信回调 HTTP 头签名。
  - 解密官方回调 `resource`。
  - 校验 `appid` / `mchid`。
  - 校验 `out_trade_no` 与本地订单。
  - 校验回调金额与本地订单整数分一致。
  - `SUCCESS` 后更新订单为 `paid`。
  - 写入 payment record。
  - 幂等发放课程包 entitlement。
  - 重复 SUCCESS 回调不会重复发放 entitlement。
  - 复用同一 transaction id 指向不同订单会被拒绝。

仍未完成：

- 真实微信商户参数环境端到端联调。
- 真实微信商户环境下的关单端到端验证。
- 退款流程。
- 证书轮换自动化。
- 支付运营告警、异常人工处理台账。

因此当前只能说「微信 Native 代码链路已具备联调基础」，不能说「真实支付已经上线可收款」。

### 2.7 Web 主站

- 已有首页、登录、课程列表、课程详情、资料详情、课程包列表/详情、课程刷题、Wiki、Blog、Forum、错题本、个人中心、通知等基础页面。
- 课程包详情页可创建/复用 pending 订单、请求 Native codeUrl、渲染二维码、轮询只读订单状态。
- 前端不会伪造支付成功，也不会在前端发放 entitlement。
- 页面仍需要系统性的移动端截图回归。

### 2.8 Vue Admin

- 已有登录、路由守卫、Dashboard、用户管理、积分管理、会员管理、权益授权、课程管理、资料上传、资料状态流转、课程包管理、订单只读查询、下载审计、内容审核、AI draft 审核、举报处理、Analytics 和 Operation logs。
- 高风险操作由 Go API 服务端鉴权，并写 operation logs。
- 会员/积分已有基础后台；系统配置、兑换和支付驱动的会员运营后台仍不完整。

### 2.9 刷题、错题、AI 与内容社区

- 题目列表/详情 public DTO 不返回答案。
- 已支持基础题型、提交判题、quiz attempt、错题记录和基础薄弱点统计。
- AI 当前是 mock task / worker / draft review 流程，生成内容不会自动发布。
- Wiki、Blog、Forum 已有 review-first 基础流程。
- 举报与通知基础流程已实现。

## 3. 尚未完成的内容

- 真实微信支付商户环境端到端联调。
- 订单关闭、退款、支付告警和证书轮换。
- 完整资料导入运营流程、真实 PDF 部署挂载说明和导入后 E2E 验收。
- 真实 LLM、RAG、AI 发布到题库/资料/Wiki 的正式流程。
- 会员套餐购买、积分兑换 AI 次数、会员权益中间件完整闭环。
- Moment 动态与关注 / 互关好友 / 屏蔽的 Go API 基础已补；Web `/moments` 基础动态流、发布、图片上传/预览、点赞、评论、关注和屏蔽入口已补；动态图片读取已通过 Go API 复用动态可见性和屏蔽规则；公开用户主页聚合 API 和 Web `/users/[id]` 已补；视频和云存储仍未完成。
- 搜索、排行榜、通知扩展和更完整的运营统计。
- 完整 E2E、移动端截图回归、浏览器自动化验收；当前已有 delivery、quiz wrong-question、admin material-review、admin blog-review、admin wiki-review、admin wiki-proposal-review、admin forum-review、admin forum-reply-review、admin ai-draft-review 和 mobile public-page 多条 opt-in/target smoke，但还不是完整回归套件。
- 生产部署硬化：HTTPS、反代、日志、监控、备份、密钥轮换。

## 4. 与原始 plan 对照

| 原始阶段 | 原始目标 | 当前状态 | 说明 |
| --- | --- | --- | --- |
| Stage 0：清理与工程骨架 | monorepo、旧代码归档、Docker、env、README | 完成 | V2 monorepo 已建立，旧代码归档，Docker/env/README 存在。 |
| Stage 1：Go API 基础框架 | Gin、配置、PostgreSQL、Redis、GORM、中间件、Health | 完成 | API 服务骨架和基础接口已实现。 |
| Stage 2：全新数据库 Schema | V2 schema、migration、seed | 部分完成 | migration/model/seed 已有；部分领域业务仍未填满。 |
| Stage 3：认证与权限 | 邮箱验证码、JWT RS256、角色、冻结、管理/审核/创作者权限 | 部分完成 | 登录、JWT、角色和冻结边界已有；真实邮件发送和生产密钥配置仍是部署工作。 |
| Stage 4：组织架构与课程资料 | 组织/课程 CRUD、资料上传下载、水印、权限 | 部分完成 / 强 MVP | 核心资料链路已可验证；manifest dry-run 预检、manifest 导入、manifest-to-paid-download 测试、API manual-grant smoke 和浏览器 delivery smoke 已完成；真实内测资料导入验收、OSS/S3 与完整运营流程未完成。 |
| Stage 5：刷题系统 | 多题型、提交、错题本、薄弱点 | 部分完成 | 基础题型、提交、错题、薄弱点已有；复杂评分和练习 session 需增强。 |
| Stage 6：AI 基础设施与 Worker | Redis Streams、LLM、AI task、draft review | 部分完成 | mock worker/draft review 已有；真实 LLM/RAG 未完成。 |
| Stage 7：积分与会员 | 积分流水、规则、会员、兑换、权益 | 部分完成 | 积分流水、用户积分页、admin 积分规则维护、公开会员套餐、用户会员页、admin 手动赠送/撤销会员已做；购买、兑换、AI 权益扣减仍未完整闭环。 |
| Stage 8：支付系统 | 原 plan 写易支付 | 方向调整 / 部分完成 | 已改为微信 Native；mock 下单、mock notify、live 下单、live notify、基础关单、订单过期收敛、risk_flag 可见性、payment incident 人工处理台账、Dashboard 未处理数量提醒和可选 webhook 提醒已有；真实商户 E2E、退款、证书轮换和自动对账未完成。 |
| Stage 9：Wiki 共创体系 | 创作者申请、Wiki、协作编辑、历史、审核 | 部分完成 | Wiki 公开页、修订提案、审核、历史和 stale 防护已有；创作者申请流不完整。 |
| Stage 10：博客、动态、帖子区 | Blog、Moment、Forum、关系系统 | 部分完成 | Blog/Forum 基础和审核已有；Moment 与关系系统 Go API 基础已补，Web `/moments` 基础动态流、图片上传/预览、动态图片服务端可见性鉴权、`/users/[id]` 公开用户主页聚合和 `/me/relations` 关系管理已补；视频和云存储仍未完成。 |
| Stage 11：通知、举报、搜索、排行榜 | 通知、举报、搜索、排行榜 | 部分完成 | 通知、举报、Admin 处理已有；基础公开搜索 API 和 Web 搜索页已做；排行榜未做。 |
| Stage 12：Next.js 主站 | 主站完整页面 | 部分完成 | 课程、资料、课程包、Wiki、Blog、Forum、动态、用户主页、错题、通知等核心页已有；AI/会员/积分/排行榜不足。 |
| Stage 13：Vue 3 管理后台 | 完整运营后台 | 部分完成 | 用户、课程、资料、课程包、订单、支付异常台账、积分管理、会员管理、审核、举报、日志、AI draft 已有；系统配置、兑换、支付驱动会员运营仍不足。 |
| Stage 14：Docker Compose | 本地一键启动 | 部分完成 | Compose 配置存在；全链路仍依赖本地 env、seed 和文件挂载。 |
| Stage 15：Seed 数据与演示账号 | 演示组织、课程、资料、题目、内容、账号 | 部分完成 | Seed 覆盖核心演示数据；manifest 导入示例和命令已完成；真实资料文件不提交。 |
| Stage 16：测试与质量 | 后端、前端、Docker、支付、审核等测试 | 部分完成 | Go tests 和部分 build/lint 已持续运行；已补 delivery、quiz wrong-question、admin material-review、admin blog-review、admin wiki-review、admin wiki-proposal-review、admin forum-review、admin forum-reply-review、admin ai-draft-review、mobile public-page browser smoke；仍缺完整 E2E 和截图回归。 |
| Stage 17：文档 | 架构、API、数据库、开发、部署、安全文档 | 部分完成 | 核心文档存在；仍需随实现持续更新。 |

## 5. 本阶段新增/修改的关键文件

- `services/api/internal/payment/wechat_notify.go`
- `services/api/internal/payment/wechat_native_client.go`
- `services/api/internal/payment/handler.go`
- `services/api/internal/server/router.go`
- `services/api/tests/wechat_native_test.go`
- `README.md`
- `docs/api.md`
- `docs/security.md`
- `docs/roadmap.md`
- `docs/phase-summary-v2-mvp.md`
- `docs/stage-summary-2026-06-24.md`

## 6. 本阶段验证命令

本阶段至少需要持续运行：

```bash
cd services/api
../../.tools/go/bin/go.exe test ./...

cd ../..
docker compose -f docker-compose.dev.yml config --quiet
git diff --check
```

本阶段已重点覆盖微信 Native 相关单测：

- mock notify 成功支付并幂等授权。
- live notify 官方签名校验、resource 解密、appid/mchid/金额校验、支付成功和幂等授权。
- 微信 Native 关单权限、状态流转和 closed 订单不复用。
- 微信 Native 过期订单状态收敛、expired 订单不可继续支付/关单且不可被新下单复用。
- Admin 订单风险筛选、risk_flag 展示、payment incident 人工处理台账、Dashboard 未处理数量提醒和可选 webhook 提醒。
- 签名错误、解密失败、appid 不匹配、金额不匹配、transaction id 冲突不会发放权益。

## 7. 当前风险

- 真实微信商户端到端链路未联调，不能直接对外收款。
- 当前 live notify 代码链路依赖部署环境正确配置平台证书目录、API v3 key、商户私钥和 notify URL。
- 自动证书轮换、支付异常分级升级路由和支付对账未做，生产运维风险仍高。
- 真实课程资料不应提交到 Git，需要通过部署挂载、后台上传或 manifest 导入命令入库。
- AI 仍是 mock 为主，不能宣传为真实智能学习闭环。
- 缺少完整 E2E 和移动端截图回归，界面体验风险仍需单独验收；quiz wrong-question、admin material-review、admin blog-review、admin wiki-review、admin wiki-proposal-review、admin forum-review、admin forum-reply-review 和 admin ai-draft-review smoke 只能覆盖关键单路径，不等于完整刷题/审核/AI 回归。

## 8. 下一阶段最小任务

1. 用真实微信商户参数联调 Native 下单、notify 回调和关单。
2. 增加真正的支付异常分级升级路由、自动对账和支付运维说明；人工处理台账、Dashboard 数量提醒与 webhook 基础提醒已有。
3. 用真实内测资料跑一次 manifest 导入到本地/测试库，确认包绑定和 paid 下载权限；自动化 smoke 已覆盖测试夹具链路，但不能替代真实资料验收。
4. 继续补 E2E smoke：当前已有 delivery、quiz wrong-question、admin material-review、admin blog-review、admin wiki-review、admin wiki-proposal-review、admin forum-review、admin forum-reply-review、admin ai-draft-review、mock-payment 和 mobile public-page；下一步补举报处理、更多刷题题型和真实 AI/支付联调 smoke。
5. 做移动端截图回归，优先覆盖 390px 宽度。

## 9. 结论

V2 已从纯骨架推进到可本地验证的强 MVP：课程资料、下载权限、课程包、刷题、审核、通知、举报、Admin、Worker 和微信 Native 支付代码链路都有基础实现。

但它仍是内测准备状态，不是正式上线状态。最优先的上线前缺口是微信真实商户 E2E、支付运维硬化、真实资料导入验收、完整 E2E 和生产部署安全边界。

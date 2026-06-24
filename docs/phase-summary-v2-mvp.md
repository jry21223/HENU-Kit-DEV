# V2 MVP 本阶段总结

日期：2026-06-24

本文档用于对照原始「一站式学习平台 V2 全量重构版」计划，说明当前阶段实际完成了什么、还有哪些没有完成、哪些内容只是 MVP / mock / 预留边界。本文以仓库当前代码和可运行检查为准，不把未接通的能力写成已完成。

## 1. 当前阶段

当前处于 V2 MVP 实现与收敛修复阶段。

项目已经从旧版 Next.js + Prisma 形态切换到新的 monorepo：

- `apps/web`：Next.js 主站。
- `apps/admin`：Vue 3 管理后台。
- `services/api`：Go Gin/GORM 单体 API。
- `services/worker`：Go Worker。
- `legacy/v1-next-prisma`：旧版代码归档，仅作参考。

本阶段不是上线阶段，也不是生产支付联调完成阶段。当前目标是让 V2 主要产品闭环具备可本地验证的基础形态，并持续把高风险边界收紧。

## 2. 本阶段做了什么

### 2.1 工程与架构

- 建立 V2 monorepo 结构。
- 旧版 Next.js + Prisma 代码已归档到 `legacy/v1-next-prisma`。
- 新业务后端放到 Go API，不再把核心业务写在 Next.js Route Handler / Server Action 中。
- 添加 Docker Compose 本地开发配置。
- 添加 `.env.example`，不提交真实密钥、证书、课程 PDF。
- README、API、数据库、路线图、安全、AI 工作流等文档已建立。

### 2.2 Go API 基础

- 实现 Gin 服务启动。
- 实现配置读取、PostgreSQL/GORM、Redis 基础连接。
- 实现统一响应结构、统一错误、中间件、CORS、Recover、请求日志、基础限流。
- 实现健康检查与版本接口：
  - `GET /healthz`
  - `GET /api/v1/healthz`
  - `GET /api/v1/version`

### 2.3 数据库与 Seed

- 建立 V2 SQL migration 与 GORM models。
- Seed 脚本可以生成演示学校、学院、专业、课程、资料、课程包、题目、社区内容、AI mock task 和演示账号。
- 演示账号包括：
  - `admin@example.com`
  - `reviewer@example.com`
  - `creator@example.com`
  - `user@example.com`
- 开发环境验证码可用固定值 `123456`，生产环境不能依赖固定验证码。

### 2.4 登录、用户与权限

- 实现邮箱验证码登录基础流程。
- 实现 JWT cookie/token。
- 实现角色与权限边界：
  - 普通用户
  - 创作者
  - 审核员
  - 管理员
  - 超级管理员
- 实现冻结用户的写操作限制。
- Web `/me` 可维护当前账号学校、专业、年级绑定。

### 2.5 课程、资料、课程包

- 实现学校 / 学院 / 专业 / 课程基础 API。
- 实现资料 public / admin API。
- 实现资料上传基础防护：
  - 文件名处理
  - 后缀限制
  - MIME / 内容校验
  - 大小限制
  - 路径穿越防护
- 实现资料下载权限：
  - `free`
  - `login_required`
  - `paid`
  - `member_only` 边界预留
- 成功下载写入服务端下载审计日志。
- 被拒绝下载、危险路径、缺失文件不会写成成功下载。
- PDF 下载会生成临时水印副本，不覆盖源文件。
- 非 PDF 文件保持原样下载。
- 实现课程包 catalog、课程包详情、包内资料绑定、包级授权解锁。
- 公开课程包详情只返回 published 包与 published 资料，不能泄露 draft / pending / rejected / archived 资料 item。
- 实现资料 manifest 导入基础版：
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

### 2.6 微信 Native 支付方向收敛

- 当前支付方向已经从原计划中的易支付改为微信支付 Native。
- 已实现 Go API `POST /api/v1/payments/wechat/native` 的开发/测试 mock 边界：
  - development/test 下 `WECHAT_PAY_MODE=mock` 可返回 mock `codeUrl`。
  - mock 下单只把订单推进到 `paying`。
  - mock 下单不会标记 `paid`。
  - mock 下单不会发放 entitlement。
  - production 环境禁止 mock 支付。
  - live 模式会做配置检查，并已接入真实微信 Native 下单请求签名与微信响应验签代码路径。
- 已实现开发/测试环境 mock notify 边界：
  - `POST /api/v1/payments/wechat/notify` 不要求用户登录，模拟微信服务器回调入口。
  - mock notify 必须带 `X-WeChat-Mock-Signature` HMAC 头。
  - mock notify 必须校验订单号和金额。
  - 签名错误、金额不一致、订单不存在、缺少 mock secret 时不会更新订单，也不会发放 entitlement。
  - 成功 mock notify 会把订单置为 `paid`，写入 `payment_records`，并幂等发放一次课程包 entitlement。
  - 这只是本地联调 harness，不能代替生产微信官方回调。
- 已实现并单测微信支付 API v3 live 所需密码学基础件：
  - RSA 请求签名。
  - Authorization header 组装。
  - 微信 notify 原文验签。
  - 商户私钥解析。
  - 平台公钥/证书公钥解析。
  - AES-256-GCM resource 解密。
- live Native 下单已接入：
  - 用商户私钥签名 `POST /v3/pay/transactions/native`。
  - 请求金额来自服务端订单整数分。
  - 响应必须通过平台证书/公钥验签后才保存 `code_url`。
  - 当前仍未做真实商户环境端到端联调。
- live 官方 notify handler 已接入：
  - 校验微信回调 HTTP 头签名。
  - 解密 AES-256-GCM `resource`。
  - 校验 appid/mchid、订单号和整数分金额。
  - `SUCCESS` 后把订单置为 `paid`，写入 `payment_records`，并幂等发放课程包 entitlement。
  - 重复 SUCCESS 回调不会重复授权，transaction id 指向不同订单会被拒绝。
  - 当前仍未做真实商户环境端到端联调、证书轮换和支付告警。
- 微信 Native 关单基础接口已接入：
  - `POST /api/v1/payments/wechat/close` 关闭 pending/paying 订单。
  - 用户只能关闭自己的订单，admin/super_admin 可以关闭任意 pending/paying 订单。
  - paid/closed 订单不能关闭。
  - closed 订单不会被后续课程包下单复用。
  - live paying 订单会先调用微信关单接口，再更新本地状态；真实商户环境仍未做端到端验证。
- 微信 Native 订单过期收敛已接入：
  - Native 下单会写入 `orders.expires_at`。
  - 状态查询、重复下单复用、Native 支付创建和 admin 订单列表会先把过期 pending/paying 订单置为 `expired`。
  - expired 订单不能继续拉起二维码，不能通过 close 接口关闭，也不会被新课程包订单复用。
- Admin 订单查询可展示并筛选 `risk_flag`，用于支付异常排查；当前还不是自动告警或自动对账系统。
- Payment incident ledger has been added for rejected WeChat callback anomalies (`order_not_found`, `amount_mismatch`, `transaction_conflict`). Admins can mark incidents `resolved` or `ignored`; this writes operation logs only and never marks orders paid or grants entitlement. Vue Admin Dashboard now surfaces the open incident count as a basic operator prompt, and newly opened incidents can optionally emit a signed best-effort webhook without raw notify payloads.
- Payment reconciliation now has an admin-only, read-only local report for order/payment-record/order-grant/risk-flag/open-incident consistency issues. It is not a live WeChat merchant settlement reconciler and does not mutate orders, incidents, payment records, or entitlements.
- Web 课程包详情页现在可以：
  - 创建或复用待支付课程包订单。
  - 调用 Native 下单接口获取服务端返回的 `codeUrl`。
  - 在前端本地渲染微信 Native 二维码。
  - 每 3 秒轮询只读订单状态。
  - 不在前端伪造支付成功。
  - 不在前端发放 entitlement。

### 2.7 刷题、错题本与薄弱点

- 实现题目列表/详情 public DTO，题目接口不返回答案。
- 支持基础题型结构：
  - 单选
  - 多选
  - 判断
  - 填空
  - 简答基础存储
- 实现提交判题基础逻辑。
- 实现 quiz attempt。
- 实现错题记录。
- Web `/me/wrong-questions` 可查看当前用户自己的错题和基础薄弱课程统计。
- 错题删除只影响当前用户记录，不影响题库。

### 2.8 Wiki、博客、论坛、举报、通知

- Wiki：
  - 公开列表/详情只展示 published 内容。
  - 创作者/admin 可提交 Wiki。
  - Web Wiki 详情页有创作者/admin 修订提案表单。
  - 修订提案必须进入审核，不会直接改公开内容。
  - Admin 支持 Wiki entry / proposal 审核。
  - 支持 stale base version 防护。
- Blog：
  - 公开列表/详情只展示 published 内容。
  - 后端 review-first 流程已建立。
  - Web 详情页支持举报。
- Forum：
  - 公开列表/详情只展示 published 公开帖子。
  - 登录用户可提交待审核帖子和回复。
  - 支持普通帖、问答帖、悬赏帖基础形态。
  - 悬赏帖支持积分冻结、拒绝退款、最佳答案结算基础逻辑。
  - Web `/me/forum` 可追踪和重新提交自己的帖子/回复。
- 举报：
  - 支持资料、Wiki、博客、论坛内容举报。
  - Admin `/reports` 可处理举报。
  - 重复 pending 举报会去重。
- 通知：
  - Web `/me/notifications` 可查看当前用户通知。
  - 内容审核、举报处理等会创建用户通知。

### 2.9 AI 与 Worker

- Worker 骨架已存在。
- 支持 mock AI task 流程。
- Worker 可处理 pending AI task，并生成待审核 draft。
- Admin 可审核 AI draft。
- AI draft 不会自动发布为正式内容。
- 真实 LLM、RAG、AI 生成内容发布到正式资源仍未完成。

### 2.10 Vue Admin

已实现或具备基础形态：

- 登录与路由守卫。
- Dashboard。
- 用户管理。
- 手动 access grants。
- 学校 / 课程 / 资料管理。
- 资料上传与状态流转。
- 课程包管理、包内资料绑定/解绑。
- 订单只读查看。
- 下载审计。
- Wiki / Wiki proposal / Blog / Forum / Forum reply 审核。
- AI draft 审核。
- 举报处理。
- Analytics 基础页。
- Operation logs 浏览、筛选、导出。

Admin 页面不会直接授予支付成功，也不会绕过 Go API 权限。

## 3. 本阶段没有做完什么

### 3.1 真实微信 Native 支付仍未完成生产联调

目前已有 mock Native codeUrl、前端二维码展示、开发/测试环境带 HMAC 的 mock notify 闭环、带响应验签的 live Native 下单代码路径，以及 live 官方 notify 验签、解密、金额校验和幂等授权代码路径。

还没有完成：

- 真实微信商户参数环境端到端联调。
- 真实微信商户环境下的关单端到端验证。
- 退款流程。
- 证书轮换自动化。
- 支付异常告警和人工处理台账。

因此当前只能声明为“微信 Native 联调准备中”，不能声明为“真实支付已上线可收款”。

### 3.2 会员与积分产品未完整闭环

已有积分 ledger 和论坛悬赏相关积分行为，但还没有形成完整商业产品：

- 会员套餐购买/兑换 UI 不完整。
- AI 权益扣减/折扣中间件未完整贯通。
- 积分兑换 AI 次数、兑换套卷权限未完整实现。
- 会员到期通知、会员权益统计仍待补。

### 3.3 AI 仍是 mock / 审核流基础版

还没有完成：

- 真实 OpenAI-compatible LLM 接入验证。
- RAG。
- 基于课程资料的知识库检索。
- AI 针对性出题完整闭环。
- AI 套卷生成完整闭环。
- AI 草稿发布到题库/资料/Wiki 的正式流程。

AI 内容不自动发布，这是安全边界，不是缺陷。

### 3.4 社交与内容模块仍不完整

未完成或仅有基础边界：

- Moment 动态与关注 / 互关好友 / 屏蔽的 Go API 基础已补；Web `/moments` 基础动态流、发布、点赞、评论、关注和屏蔽入口已补，用户主页聚合和真实媒体上传仍未完成。
- 关系列表管理页与更丰富的互关好友 UX。
- 用户主页聚合。
- 搜索。
- 排行榜。
- 博客积分激励完整结算。
- Forum 复杂楼层、精华、置顶、更多运营能力。

### 3.5 E2E 和上线硬化不足

目前主要验证方式是单元/集成测试、typecheck、build 和局部人工检查。

还需要补：

- 登录 -> 浏览课程包 -> 创建订单 -> 管理授权 -> 下载 paid 资料的 E2E。
- 刷题 -> 错题本 -> 薄弱点的 E2E。
- Admin 审核流 E2E。
- 移动端截图回归。
- Docker 全链路 smoke。
- 生产部署脚本、备份、监控、HTTPS、反向代理、安全 headers。

## 4. 原始 Plan 对照

状态说明：

- 完成：仓库已有对应代码，并通过本地检查或直接可验证。
- 部分完成：已有可用基础，但原计划要求更完整。
- 未开始：没有形成有效实现。
- 方向调整：原计划内容被后续产品方向替换。

| 原始阶段 | 原始目标 | 当前状态 | 对照说明 |
| --- | --- | --- | --- |
| Stage 0：清理与工程骨架 | monorepo、旧代码归档、Docker、env、README | 完成 | V2 monorepo 已建立，旧代码已归档，基础文档和 Docker Compose 存在。 |
| Stage 1：Go API 基础框架 | Gin、配置、PostgreSQL、Redis、GORM、中间件、Health | 完成 | API 服务骨架和基础接口已实现，Go tests 可运行。 |
| Stage 2：全新数据库 Schema | V2 schema、migration、seed | 部分完成 | 迁移、models、seed 已有；部分领域表已有但业务行为仍未全部填满。 |
| Stage 3：认证与权限 | 邮箱验证码、JWT RS256、角色、冻结、管理员/审核/创作者权限 | 部分完成 | 登录、JWT、角色和冻结边界存在；真实邮件发送和生产密钥配置仍是部署工作。 |
| Stage 4：组织架构与课程资料 | 组织/课程 CRUD、资料上传下载、水印、权限 | 部分完成 / 强 MVP | 组织、课程、资料、上传、下载权限、水印、审计、manifest dry-run 预检、manifest 导入、manifest-to-paid-download 测试、API manual-grant smoke 和浏览器 delivery smoke 已实现；真实内测资料导入验收和 OSS/S3 等生产存储仍未完成。 |
| Stage 5：刷题系统 | 多题型、提交、错题本、薄弱点 | 部分完成 | 基础题型、提交、错题、Web 错题本存在；练习 session、复杂评分仍需增强。 |
| Stage 6：AI 基础设施与 Worker | Redis Streams、LLM、AI task、draft review | 部分完成 | mock task、worker、draft review 存在；真实 LLM、RAG、发布流未完成。 |
| Stage 7：积分与会员 | 积分流水、规则、会员、兑换、权益 | 部分完成 | 积分流水、用户积分页、admin 积分规则维护、公开会员套餐、用户会员页、admin 手动赠送/撤销会员已做；购买、兑换、AI 权益扣减仍未完整闭环。 |
| Stage 8：支付系统 | 原文为易支付，后续改为微信 Native | 方向调整 / 部分完成 | 易支付不是当前目标。已做微信 Native mock 下单、Web 二维码、只读轮询、开发/测试 mock notify 支付成功、幂等授权闭环、带请求签名/响应验签的 live Native 下单代码路径、live 官方 notify 验签/解密/金额校验/幂等授权代码路径、基础关单接口、订单过期收敛、risk_flag 可见性、payment incident 人工处理台账、Dashboard 未处理数量提醒和可选 webhook 提醒；真实微信商户端到端联调、退款、证书轮换和自动对账未完成。 |
| Stage 9：Wiki 共创体系 | 创作者申请、Wiki、协作编辑、历史、审核 | 部分完成 / 强 MVP | Wiki 公开页、修订提案、审核、历史、stale 防护已做；创作者申请流未完整完成。 |
| Stage 10：博客、动态、帖子区 | Blog、Moment、Forum、关系系统 | 部分完成 | Blog、Forum 基础和审核已做；Moment、关系系统 Go API 基础已补，Web `/moments` 基础动态流已做；用户主页聚合和真实媒体上传仍未做。 |
| Stage 11：通知、举报、搜索、排行榜 | 通知、举报、搜索、排行榜 | 部分完成 | 通知、举报、Admin 处理已做；基础公开搜索 API 和 Web 搜索页已做；排行榜未做。 |
| Stage 12：Next.js 主站 | 主站完整页面 | 部分完成 | 课程、资料、课程包、Wiki、Blog、Forum、动态、错题、通知等核心页存在；AI、会员、积分、排行榜等页不足。 |
| Stage 13：Vue 3 管理后台 | 管理员后台完整运营能力 | 部分完成 | 用户、课程、资料、课程包、订单、支付异常台账、积分管理、会员管理、审核、举报、日志、AI draft 等已做；系统配置、兑换、支付驱动会员运营仍不足。 |
| Stage 14：Docker Compose | 本地一键启动 | 部分完成 / 可用基础 | Compose 配置存在并可校验；全链路启动仍依赖本地 env、seed、文件挂载。 |
| Stage 15：Seed 数据与演示账号 | 演示组织、课程、资料、题目、内容、账号 | 部分完成 | Seed command 存在并覆盖核心演示数据；manifest 导入示例和命令已完成；真实资料文件不提交，需要部署挂载、后台上传或 manifest 导入。 |
| Stage 16：测试与质量 | 后端、前端、Docker、支付、审核等测试 | 部分完成 | Go tests、Web/Admin lint/build 已持续运行；缺少完整 E2E 和浏览器截图回归。 |
| Stage 17：文档 | 架构、API、数据库、开发、部署、安全文档 | 部分完成 | 核心文档存在；支付、部署、会员、AI 等需随着实现继续更新。 |

## 5. 当前可验证能力清单

- V2 monorepo 结构清楚。
- Go API 可测试。
- Web / Admin 可 lint 和 build。
- Docker Compose 配置可校验。
- 登录、角色、基础权限可用。
- 课程、资料、课程包可浏览。
- paid 资料下载必须经过服务端权限判断。
- 成功下载有审计记录。
- PDF 水印不覆盖原文件。
- 题目接口不泄露答案。
- 用户可刷题并记录错题。
- 用户只能看自己的错题和通知。
- 公开内容接口过滤未发布内容。
- Admin 审核动作保留服务端权限边界。
- AI 生成内容只进入 draft/review，不自动发布。
- 微信 Native mock 下单不会发放权益；只有带 HMAC 的开发/测试 mock notify 可以把订单置为 paid 并幂等发放课程包 entitlement。
- 微信 Native 关单只允许 pending/paying -> closed，不能关闭 paid 订单或撤销已发放权益。
- 微信 Native 过期 pending/paying 订单会被服务端置为 expired，不能继续拉起支付、不能关闭、不能被重复下单复用。
- Web 二维码只是展示层，支付成功和解锁必须以后端为准。

## 6. 本阶段验证命令

本阶段应持续使用以下命令验证：

```bash
cd apps/web
npm run lint
npm run build

cd ../../apps/admin
npm run lint
npm run build

cd ../../services/api
go test ./...

cd ../..
docker compose -f docker-compose.dev.yml config --quiet
git diff --check
```

说明：

- Admin build 可能出现 Vite chunk size warning，这是体积提醒，不是失败。
- PowerShell 可能因为终端编码显示中文乱码，不代表文件内容损坏。

## 7. 当前风险

- 真实微信支付尚未接通，不能对外收款上线。
- 生产环境 entitlement 自动交付的代码边界已经包含微信官方回调验签、解密、金额校验、appid/mchid 校验和幂等授权，但必须等真实商户端到端联调、证书轮换和支付告警完成后再开放真实收款。
- 当前真实 PDF 挂载、导入后验收和运营交接仍需部署流程配合。
- 真实 AI 成本、内容质量、审核发布链路都未硬化。
- 缺少浏览器级 E2E 与移动端截图回归。
- 生产部署还缺 HTTPS、反代、安全 headers、日志/监控、备份策略。

## 8. 下一阶段建议

最小下一步建议按这个顺序推进：

1. 继续微信 Native 支付硬化：
   - live 配置校验；
   - 真实商户参数联调 Native 下单；
   - 真实商户端到端 notify 联调；
   - appid/mchid 校验；
   - 真实商户环境关单验证；
   - live 支付链路端到端测试。

2. 补一组 E2E smoke：
   - 登录；
   - 浏览课程包；
   - 创建订单；
   - 管理员手动授权；
   - 下载 paid 资料；
   - 刷题生成错题；
   - Admin 审核内容。

3. 完成资料导入与课程包绑定的运营验收：
   - 使用真实内测资料跑 manifest 导入；
   - 核对课程包绑定；
   - 核对 paid 下载权限；
   - 补真实资料部署挂载说明。
   - 自动化 smoke 已覆盖测试夹具链路，但不能替代真实资料验收。

4. 再补会员/积分和 AI 真实接入。

## 9. 结论

当前仓库已经不是纯骨架，V2 MVP 的核心技术路径已经形成：Go API 作为唯一业务后端，Web/Admin 作为前端入口，Worker 处理异步任务，课程资料、刷题、审核、举报、通知、课程包和支付 mock 联调边界都已有可验证基础。

但当前仍不能声明为生产可上线版本。最大缺口是微信 Native 真实商户端到端联调、关单验证、退款/告警等支付运维硬化，其次是完整 E2E、会员/积分、真实 AI、搜索/排行榜/社交关系和生产部署硬化。

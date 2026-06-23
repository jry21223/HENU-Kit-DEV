# V2 MVP 本阶段总结

日期：2026-06-23

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

### 2.6 微信 Native 支付方向收敛

- 当前支付方向已经从原计划中的易支付改为微信支付 Native。
- 已实现 Go API `POST /api/v1/payments/wechat/native` 的开发/测试 mock 边界：
  - development/test 下 `WECHAT_PAY_MODE=mock` 可返回 mock `codeUrl`。
  - mock 下单只把订单推进到 `paying`。
  - mock 下单不会标记 `paid`。
  - mock 下单不会发放 entitlement。
  - production 环境禁止 mock 支付。
  - live 模式会做配置检查，但真实微信 API 调用还没有完成。
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

### 3.1 真实微信 Native 支付未完成

目前只有 mock Native codeUrl 和前端二维码展示。

还没有完成：

- 真实微信 Native 下单 HTTP 调用。
- 商户私钥签名。
- 平台证书验签。
- 支付回调签名验证。
- 回调 resource 解密。
- 金额校验。
- appid / mchid 校验。
- `paid` 状态服务端转换。
- 支付成功自动发放 entitlement。
- 重复回调幂等授权。
- 关单接口。

因此当前不能声明为“真实支付可用”。

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

- 动态 moments。
- 关注 / 互关好友 / 屏蔽。
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
| Stage 4：组织架构与课程资料 | 组织/课程 CRUD、资料上传下载、水印、权限 | 部分完成 / 强 MVP | 组织、课程、资料、上传、下载权限、水印、审计已实现；OSS/S3 等生产存储仍未接。 |
| Stage 5：刷题系统 | 多题型、提交、错题本、薄弱点 | 部分完成 | 基础题型、提交、错题、Web 错题本存在；练习 session、复杂评分仍需增强。 |
| Stage 6：AI 基础设施与 Worker | Redis Streams、LLM、AI task、draft review | 部分完成 | mock task、worker、draft review 存在；真实 LLM、RAG、发布流未完成。 |
| Stage 7：积分与会员 | 积分流水、规则、会员、兑换、权益 | 部分完成 | 积分在论坛悬赏等场景已使用；会员产品和兑换链路未完整闭环。 |
| Stage 8：支付系统 | 原文为易支付，后续改为微信 Native | 方向调整 / 部分完成 | 易支付不是当前目标。已做微信 Native mock 下单、Web 二维码、只读轮询；真实微信支付未完成。 |
| Stage 9：Wiki 共创体系 | 创作者申请、Wiki、协作编辑、历史、审核 | 部分完成 / 强 MVP | Wiki 公开页、修订提案、审核、历史、stale 防护已做；创作者申请流未完整完成。 |
| Stage 10：博客、动态、帖子区 | Blog、Moment、Forum、关系系统 | 部分完成 | Blog、Forum 基础和审核已做；Moment、关系系统、用户主页未做。 |
| Stage 11：通知、举报、搜索、排行榜 | 通知、举报、搜索、排行榜 | 部分完成 | 通知、举报、Admin 处理已做；搜索和排行榜未做。 |
| Stage 12：Next.js 主站 | 主站完整页面 | 部分完成 | 课程、资料、课程包、Wiki、Blog、Forum、错题、通知等核心页存在；AI、会员、积分、动态、排行榜等页不足。 |
| Stage 13：Vue 3 管理后台 | 管理员后台完整运营能力 | 部分完成 | 用户、课程、资料、课程包、订单、审核、举报、日志、AI draft 等已做；会员/积分/系统配置仍不足。 |
| Stage 14：Docker Compose | 本地一键启动 | 部分完成 / 可用基础 | Compose 配置存在并可校验；全链路启动仍依赖本地 env、seed、文件挂载。 |
| Stage 15：Seed 数据与演示账号 | 演示组织、课程、资料、题目、内容、账号 | 部分完成 | Seed command 存在并覆盖核心演示数据；真实资料文件不提交，需要部署挂载或后台上传。 |
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
- 微信 Native mock 下单不会发放权益。
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
- entitlement 自动交付必须等支付回调验签、解密、金额校验、幂等都完成后再开放。
- 当前资料导入和真实 PDF 挂载仍需部署流程配合。
- 真实 AI 成本、内容质量、审核发布链路都未硬化。
- 缺少浏览器级 E2E 与移动端截图回归。
- 生产部署还缺 HTTPS、反代、安全 headers、日志/监控、备份策略。

## 8. 下一阶段建议

最小下一步建议按这个顺序推进：

1. 继续微信 Native 支付硬化：
   - live 配置校验；
   - 真实 Native 下单；
   - 回调验签；
   - resource 解密；
   - 金额与订单校验；
   - 幂等 paid 转换；
   - entitlement 发放。

2. 补一组 E2E smoke：
   - 登录；
   - 浏览课程包；
   - 创建订单；
   - 管理员手动授权；
   - 下载 paid 资料；
   - 刷题生成错题；
   - Admin 审核内容。

3. 完成资料导入与课程包绑定的运营流程：
   - manifest 导入；
   - 文件路径安全；
   - 重复导入保护；
   - 真实资料部署挂载说明。

4. 再补会员/积分和 AI 真实接入。

## 9. 结论

当前仓库已经不是纯骨架，V2 MVP 的核心技术路径已经形成：Go API 作为唯一业务后端，Web/Admin 作为前端入口，Worker 处理异步任务，课程资料、刷题、审核、举报、通知、课程包和支付 mock 边界都已有可验证基础。

但当前仍不能声明为生产可上线版本。最大缺口是微信 Native 真实支付与支付后 entitlement 自动交付，其次是完整 E2E、会员/积分、真实 AI、搜索/排行榜/社交关系和生产部署硬化。

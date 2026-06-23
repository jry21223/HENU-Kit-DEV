# 一站式学习平台 V2

面向高校学生的课程级学习平台，围绕课程资料、在线刷题、AI 辅助、Wiki 共创、社区讨论、积分会员和前台销售建立完整学习闭环。

V2 是绿地重构版本。旧版 Next.js + Prisma 实现已归档到 `legacy/v1-next-prisma`，仅作为功能参考，不作为运行依赖。

## 1. 项目概览

平台按「学校 -> 学院 -> 专业 -> 课程」组织内容，第一阶段重点服务河南大学软件学院相关课程。当前目标不是简单资料下载站，而是可运营、可商业化、可扩展的学习平台。

核心方向：

- 让学生快速找到自己学校、专业、课程对应的复习资料。
- 支持课程刷题、错题本、薄弱点统计和 AI 针对性强化。
- 用 Wiki、博客、帖子、动态沉淀学生原创内容和学习经验。
- 用积分和会员体系建立创作者激励与 AI 成本控制。
- 用 LangBot 前台销售承接咨询、介绍权益、引导注册和购买。
- 用独立管理后台完成内容审核、AI 草稿审核、用户运营和系统配置。

## 2. 当前可验证状态

- V2 monorepo 骨架已建立。
- Go API 已实现 health/version、邮箱验证码登录、JWT cookie/token、角色中间件、学校/课程/资料/课程包接口、组织/课程/资料/课程包 admin CRUD、包内资料绑定、上传防护、资料下载权限、课程包 pending 订单、刷题提交、错题记录和基础薄弱点统计。
- 成功资料下载会写入服务端审计日志，失败鉴权、不安全路径和缺失文件不会记为成功下载。
- PDF 下载会在服务端生成临时轻水印副本，水印包含用户标识、资料 ID 和下载时间；源文件不会被覆盖。非 PDF 文件保持原样下载。
- 用户可以查看自己的成功下载记录；管理员可以查看全量下载审计日志。
- 课程包 catalog API 已实现，`material_access_grants.package_id` 可以在服务端解锁 published 课程包内的 paid 资料。
- Go API 与 Worker 已实现 mock AI task 流：用户创建任务，worker 完成 pending task，并把生成结果保存为待审核 draft。
- Next.js Web 已有首页、课程列表、课程详情、课程包列表/详情与解锁状态展示、资料详情、课程刷题、Wiki 只读列表/详情、Blog 只读列表/详情、论坛列表/详情、发帖、回复提交、最佳答案操作入口和学生邮箱登录页面。
- Next.js Web 已有个人中心 `/me`，登录用户可以维护学校、专业和年级绑定，在 `/me/wrong-questions` 查看错题与薄弱课程，在 `/me/forum` 追踪、修改和重新提交自己的论坛帖子/回复，并在 `/me/notifications` 查看审核通知。
- Vue Admin 已有邮箱登录、路由守卫、仪表盘、用户管理、权益授权、课程包管理、课程管理、资料上传、资料状态流转、下载审计页面和 reviewer 可访问的 AI 草稿审核页；AI 草稿通过/驳回会记录审核意见。
- 目标运行栈为 Go API、Go Worker、Next.js Web、Vue Admin、PostgreSQL 和 Redis。
- 微信支付 Native 是目标支付方案；当前仍是本地 mock 边界，未完成真实商户联调。
- AI 当前使用 mock LLM；AI 生成内容不会绕过审核自动发布。
- 当前没有生产数据迁移要求。

## 3. 产品形态

| 产品端 | 面向对象 | 说明 |
| --- | --- | --- |
| Web 主站 | 学生、创作者、普通访客 | 找资料、刷题、AI 问答、Wiki、博客、帖子、动态、会员购买 |
| Admin 管理后台 | 管理员、审核员、运营人员 | 内容审核、用户管理、AI 工作流、积分会员、系统配置 |
| LangBot 前台销售 | 潜在用户、QQ/微信群访客、客服入口 | 自动答疑、介绍套餐、引导注册、发放购买链接、售后分流 |
| Go API 服务 | Web、Admin、LangBot、Worker | 统一业务 API、鉴权、数据读写、支付回调、权限校验 |

## 4. 目录结构

```txt
apps/web                 Next.js Web 主站
apps/admin               Vue 3 管理后台
services/api             Go Gin/GORM 单体 API
services/worker          Go Redis Streams Worker
final-review-sales-agent LangBot 销售插件原型
integrations             外部集成
legacy/v1-next-prisma    V1 归档参考
infra                    Docker 和 nginx 支撑文件
docs                     V2 文档
scripts                  seed 和开发脚本
uploads                  本地运行时存储，除 .gitkeep 外忽略
```

## 5. 快速启动

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up --build
```

默认本地端口：

- Web: `http://localhost:3000`
- Admin: `http://localhost:5173`
- API: `http://localhost:8080/api/v1/healthz`

## 6. 本地检查

```bash
docker compose -f docker-compose.dev.yml config
npm install
npm run build
npm audit --audit-level=low
```

如果本机已安装 Go：

```bash
cd services/api && go test ./...
cd ../worker && go test ./...
```

本仓库也支持使用 `.tools/` 下的便携 Go 工具链进行检查；`.tools/` 已忽略，不能提交。

当前测试覆盖重点：

- 邮箱验证码登录和角色拒绝。
- Web 登录表单 build/type 覆盖。
- 资料详情不泄露 `storage_key`。
- free/login_required/paid 资料下载权限。
- 成功下载审计日志，以及拒绝下载不产生日志。
- PDF 动态轻水印、非 PDF 原样下载、原 PDF 文件不被覆盖。
- `/me/downloads` 用户隔离和 `/admin/downloads` 管理员权限。
- `/auth/me` 个人资料更新、学校/专业绑定校验和专业-学校匹配校验。
- 资料默认 draft 入库、admin 全量可见、公开端只展示 published、非法状态拒绝。
- Admin material metadata PATCH rejects direct file-field mutation; file replacement remains an upload flow.
- 课程包授权解锁包内 paid 资料。
- 后台课程包 CRUD、包内资料绑定/解绑、重复绑定保护，以及公开课程包详情不泄露未发布资料 item。
- Web 课程详情页展示课程包价格、包含资料和支付联调状态；`/packages` 展示已发布课程包列表，`/packages/[id]` 展示课程包详情、包内 published 资料、当前账号 entitlement 状态，并可创建不发放权益的 pending 课程包订单。Vue Admin `/orders` 可以只读查询订单状态，不能标记支付成功或发放权益。
- Web `/wiki` and `/wiki/[id]` expose only published public Wiki entries through the Go API; draft, pending, rejected, and private review metadata stay hidden.
- Web `/blog` and `/blog/[id]` expose only published public Blog posts through the Go API; public responses use a DTO that hides review metadata, and the detail page can submit a `blog_post` report.
- Web 论坛页展示已发布公开帖子，支持登录用户提交待审核普通/问答/悬赏帖；详情页支持登录用户提交待审核回复，并允许楼主/admin 触发服务端最佳答案选择。
- Web `/me/forum` 展示当前用户自己的论坛帖子和回复，包括待审、已发布、已驳回状态以及自己的审核说明；可修改 draft/pending/needs_changes/rejected 内容并重新提交审核，公开论坛页仍只展示 published 内容。
- Web `/me/notifications` 展示当前用户自己的通知、未读数、逐条已读和全部已读操作。
- `/me/notifications` 用户隔离、已读幂等、全部已读，以及论坛、资料、Wiki、博客、AI 草稿审核通知生成。
- 举报 API、Web 资料/Wiki/博客/论坛举报入口和 Vue Admin `/reports` 支持登录用户提交公开内容举报、reviewer/admin 处理举报、处理结果通知举报人。
- 不安全 storage key 返回 404。
- admin-only 组织/课程/资料变更。
- 上传文件名、后缀、内容和大小限制。
- Vue Admin dashboard、课程管理和资料管理 build/type 覆盖。
- 题目列表/详情不泄露答案。
- 刷题提交、错题用户隔离和 quiz attempt。
- Web `/me/wrong-questions` 展示当前用户自己的错题、薄弱课程统计和移出错题本操作；题目详情使用不含答案的公开 question DTO。
- AI task 所有权、reviewer/admin 可见性、AI 草稿审核权限边界、审核意见持久化和 worker draft 生成幂等。

## 7. Seed 数据

PostgreSQL 可用后执行：

```bash
cd services/api
go run ./cmd/seed
```

seed 会创建示例组织/课程、资料、课程包、题目、社区内容、积分/会员示例、mock AI task 和演示账号：

- `admin@example.com`
- `reviewer@example.com`
- `creator@example.com`
- `user@example.com`

开发环境默认验证码是 `123456`。生产环境必须配置真实验证码发送，不能依赖固定验证码。

seed 资料记录使用 `uploads/materials/...` 本地 storage key。真实 PDF 不会提交到仓库，应通过部署挂载或后台上传提供。

## 8. 安全边界

- 不提交 `.env`、JWT 私钥、微信支付密钥、LLM API Key 或真实课程 PDF。
- `uploads/` 是运行时存储，除占位文件外被忽略。
- 生产环境不能使用固定验证码或 mock 支付。
- paid 资料下载必须经过 Go API 服务端鉴权，不能只靠前端隐藏按钮。
- 当前 paid 资料支持直接 material grant 和 published 课程包 grant。
- 公开课程包接口只能返回 `published` 课程包，并且包内 `items` 与 `materials` 都必须过滤到 `published` 资料；即使后台把 draft/pending/archived 资料预先绑定到包里，公开响应也不能泄露这些资料 ID。
- PDF 水印由 Go API 下载接口动态生成临时文件；如果 PDF 处理失败，下载会返回错误而不是静默直出未水印文件。
- AI 生成内容必须先进入 draft/review 流程，不能自动发布为正式内容。

## 9. 开发入口

- 架构设计：`docs/architecture.md`
- API 文档：`docs/api.md`
- 数据库说明：`docs/database.md`
- AI 工作流：`docs/ai-workflow.md`
- 部署说明：`docs/deployment.md`
- Go API：`services/api/internal`
- Worker：`services/worker`
- Web：`apps/web`
- Admin：`apps/admin`

## 10. Current Admin Notes

- Vue Admin includes `/downloads` for successful material download audit logs.
- Vue Admin includes `/users` for admin-only user listing, role updates, and active/frozen status changes. The Go API prevents self role/status changes and restricts `super_admin` edits/grants to `super_admin` users.
- Vue Admin includes `/access-grants` for admin-only manual material/package access grants used in internal testing or after-sales delivery; it does not create payment orders or mark orders as paid.
- Vue Admin includes `/orders` for admin-only, read-only order inspection with buyer, package, amount, provider, status, and entitlement visibility.
- Vue Admin includes `/packages` for admin-only course package CRUD, integer-cent pricing, `draft/published/archived` status control, and package-material binding without exposing raw file storage keys.
- Vue Admin includes admin-only all-status course listing and a course edit dialog.
- Vue Admin includes material status operations for `draft`, `pending`, `published`, `rejected`, and `archived`.
- Vue Admin includes material metadata editing; the actual storage key remains hidden, and the Go API rejects direct file-field mutation through metadata PATCH.
- Vue Admin includes `/material-reviews` for reviewer/admin material approve/reject review, review reason capture, and one-way pending review checks.
- Vue Admin includes `/wiki-reviews` for reviewer/admin wiki entry approve/reject review, review reason capture, and one-way review checks.
- Vue Admin includes `/wiki-proposal-reviews` for reviewer/admin wiki edit proposal approve/reject review, stale base-version protection, and live entry version/history updates.
- Vue Admin includes `/blog-reviews` for reviewer/admin blog post approve/reject review, review reason capture, and one-way review checks.
- Vue Admin includes `/forum-reviews` for reviewer/admin forum post approve/reject review, review reason capture, and one-way review checks.
- Vue Admin includes `/forum-reply-reviews` for reviewer/admin forum reply approve/reject review, review reason capture, and one-way review checks.
- Vue Admin includes `/ai/drafts` for reviewer/admin AI task visibility, draft approve/reject review, and review reason capture.
- Vue Admin includes `/analytics` for read-only successful-download trends, top materials, access breakdown, course demand, and report handling distribution.
- Vue Admin includes operation-log time filtering, CSV export, and a read-only retention policy panel.
- The download audit page reads `GET /api/v1/admin/downloads` and still depends on Go API server-side admin authorization.
- The user management page reads `GET /api/v1/admin/users` and writes `PATCH /api/v1/admin/users/:id`; it does not edit email, password credentials, membership, or points balance.
- The access-grants page reads/writes `GET/POST/DELETE /api/v1/admin/access-grants`; manual grants use `manual_admin`, are limited to published paid/member-only materials or published packages, and are revoked server-side.
- The package-management page reads/writes `GET/POST/PATCH/DELETE /api/v1/admin/packages`, `GET/POST /api/v1/admin/packages/:id/items`, and `DELETE /api/v1/admin/packages/:id/items/:itemId`; item binding supports `resourceType=material`, treats duplicate bindings idempotently, and removes bindings without deleting the underlying material.
- Admin material and download pages do not grant paid access, mutate download logs, or expose material `storage_key`.
- Admin analytics are based on successful server-side download logs and current report records; denied download attempts, page visits, search intent, and payment conversion are not included yet.
- Material review is one-way for the MVP: only pending materials can be approved or rejected through reviewer endpoints, rejected materials stay hidden from public pages, and rejection requires a review reason.
- Wiki submission is review-first for the MVP: creator/admin users can submit entries, public wiki APIs expose only published public entries, public responses hide review metadata, and rejected entries stay hidden.
- Wiki edit proposals are review-first for the MVP: creator/admin users can propose edits to published public entries, reviewer queues compare base/current/proposed content, public content stays unchanged until approval, stale base versions return `409 proposal_stale`, and successful approval updates the live entry, increments its version, and writes `wiki_edit_histories`.
- Blog submission is review-first for the MVP: logged-in users can submit posts, public blog APIs expose only published posts through a public DTO, and rejected posts stay hidden.
- Forum submission is review-first for the MVP: logged-in users can submit normal/question/reward posts, public forum APIs expose only published public posts under published boards, and rejected posts stay hidden.
- Forum reward posts now freeze author points at submission, stay hidden until review, keep points escrowed after approval, refund points automatically on rejection, and settle escrowed points to the selected best-answer author through `POST /api/v1/forum/replies/:id/mark-best`.
- Forum replies are review-first for the MVP: replies can only target published public posts, approved replies increment the parent post comment count once, and rejected replies stay hidden.
- User-scoped forum tracking and resubmission are available through `GET/PATCH /api/v1/me/forum-posts`, `GET/PATCH /api/v1/me/forum-replies`, and Web `/me/forum`; they return only the current user's submissions, do not expose reviewer ids or other users' hidden content, and reset editable submissions back to `pending`.
- Reward-post resubmission re-freezes the original reward points after a prior rejection/refund; insufficient points keep the post rejected and do not reopen it for review.
- User notifications are available through `GET /api/v1/me/notifications`, `POST /api/v1/me/notifications/:id/read`, `POST /api/v1/me/notifications/read-all`, and Web `/me/notifications`; users only see/read their own notifications.
- Forum post/reply review creates a `forum_review` notification for the author in the same transaction as the review update and operation log.
- Material, wiki entry/proposal, blog post, and AI draft review creates a `content_review` notification for the original author/editor/task owner in the same transaction as the review update and operation log.
- Vue Admin Wiki proposal review marks stale proposals and blocks stale approval in the UI; the Go API still enforces the final stale-version rejection.
- Basic report APIs, Web material/wiki/blog/forum report buttons, and Vue Admin `/reports` are available through `POST /api/v1/reports`, `GET /api/v1/admin/reports`, `POST /api/v1/admin/reports/:id/resolve`, and `POST /api/v1/admin/reports/:id/reject`; duplicate pending reports are de-duplicated per reporter/target, and handled reports notify the reporter with `report_result`.
- Payment and membership notifications remain later work.
- AI draft review is one-way for the MVP: repeat review of approved/rejected drafts is rejected, and review does not publish generated content automatically.
- Go API writes server-side `operation_logs` for user management, access grants, organization, course, course package and package-item binding, material, upload/status/archive, material review, wiki entry/proposal review, blog review, forum post/reply review, forum best-answer selection, and AI draft review mutations; Vue Admin includes a read-only operation-log browser.
- Operation log export is admin-only, filter-aware, and capped by `OPERATION_LOG_EXPORT_LIMIT`; automatic operation-log deletion is not enabled in the MVP.
- Real AI publish-to-resource flows remain later work.
- Web `/me` updates profile binding through `PATCH /api/v1/auth/me`; school and major ids are validated by the Go API.
- Web `/me` also reads `GET /api/v1/me/entitlements` to show active direct material grants, published course package grants, and unlocked material counts for the current user.
- Web `/me/forum` reads and patches `GET/PATCH /api/v1/me/forum-posts` and `GET/PATCH /api/v1/me/forum-replies` with httpOnly-cookie credentials to show and resubmit the current user's discussion review state.
- Web `/me/notifications` reads and patches notification read state with httpOnly-cookie credentials.

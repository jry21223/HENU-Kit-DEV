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
- Next.js Web 已有首页、课程列表、课程详情、课程包列表/详情与解锁状态展示、资料详情、课程刷题、Wiki 列表/详情与创作者修订提案、Blog 只读列表/详情、论坛列表/详情、发帖、回复提交、最佳答案操作入口和学生邮箱登录页面。
- Next.js Web 已有个人中心 `/me`，登录用户可以维护学校、专业和年级绑定，在 `/me/wrong-questions` 查看错题与薄弱课程，在 `/me/forum` 追踪、修改和重新提交自己的论坛帖子/回复，在 `/moments` 查看/发布带图片的学习动态，在 `/users/[id]` 查看公开用户主页，在 `/me/relations` 管理关注/粉丝/互关好友，并在 `/me/notifications` 查看审核通知。
- Vue Admin 已有邮箱登录、路由守卫、仪表盘、用户管理、权益授权、课程包管理、课程管理、资料上传、资料状态流转、下载审计页面和 reviewer 可访问的 AI 草稿审核页；AI 草稿通过/驳回会记录审核意见。
- 目标运行栈为 Go API、Go Worker、Next.js Web、Vue Admin、PostgreSQL 和 Redis。
- 微信支付 Native 是目标支付方案；当前支持开发/测试环境 mock Native codeUrl、订单过期收敛和带 HMAC 的 mock notify 闭环，生产环境禁止 mock；live Native 下单已接入签名请求和微信响应验签，live notify handler 已实现官方回调验签、resource 解密、appid/mchid/金额校验和幂等授权代码路径，但真实商户环境端到端联调仍未完成。
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
- API liveness: `http://localhost:8080/api/v1/healthz`
- API readiness: `http://localhost:8080/api/v1/readyz`

## 6. 本地检查

```bash
docker compose -f docker-compose.dev.yml config
npm install
npm run build
npm audit --audit-level=low
```

Production-like deployment examples are provided but still require operator review:

```bash
cp .env.production.example .env.production
docker compose --env-file .env.production -f docker-compose.prod.example.yml config --quiet
cd services/api && go run ./cmd/preflight -env-file ../../.env.production
```

See `docs/deployment.md` before using this for any paid internal test. The example expects secrets and certificates to be mounted from ignored `secrets/` and `certs/` directories.
- Production preflight is a deploy gate, not a substitute for merchant or browser smoke tests. It verifies dangerous configuration before the stack is opened to paid traffic.
- Internal smoke runbook: `docs/internal-smoke.md`
- Mock WeChat payment smoke: `go run ./cmd/smoke -mock-wechat-pay -mock-wechat-secret <local-fake-secret>` in development/test only; the API must run with `WECHAT_PAY_MODE=mock` and the same fake `WECHAT_PAY_API_V3_KEY`. It signs a mock notify, verifies backend `paid` status, entitlement, and paid download. It is not a real merchant E2E check.
- Browser mock-payment smoke: `npm --workspace @final-review/web run test:e2e:mock-payment` with `E2E_MOCK_PAYMENT_SMOKE=1` verifies the Web package QR flow and signed backend mock notify unlock path in development/test only.
- Browser delivery smoke: `npm --workspace @final-review/web run test:e2e:delivery` with `E2E_DELIVERY_SMOKE=1` opens Web/Admin, verifies paid denial before entitlement, creates an admin package grant, and verifies paid download after the grant.
- Quiz wrong-question smoke: `npm --workspace @final-review/web run test:e2e:quiz` with `E2E_QUIZ_SMOKE=1` logs in through Web, submits an intentionally wrong answer, verifies Go API wrong-question persistence, and checks `/me/wrong-questions`.
- Admin review smoke: `npm --workspace @final-review/web run test:e2e:review` with `E2E_REVIEW_SMOKE=1` creates a pending Blog post, approves it through Vue Admin, and verifies it becomes public on Web.
- Admin forum-review smoke: `npm --workspace @final-review/web run test:e2e:forum-review` with `E2E_FORUM_REVIEW_SMOKE=1` creates a pending Forum post, approves it through Vue Admin, and verifies it becomes public only after review.
- Mobile public-page smoke: `npm --workspace @final-review/web run test:e2e:mobile` checks core public pages at a 390px viewport for 5xx failures, document-level horizontal overflow, and basic mobile control target sizes.

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
- Web 课程详情页展示课程包价格、包含资料和支付联调状态；`/packages` 展示已发布课程包列表，`/packages/[id]` 展示课程包详情、包内 published 资料、当前账号 entitlement 状态，并可创建 pending 课程包订单。Go API 可在开发/测试环境为订单生成 mock WeChat Native codeUrl 并把订单置为 `paying`，同时写入 `expiresAt`；过期的 pending/paying 微信订单会被服务端收敛为 `expired`，不能继续拉起支付或被新下单复用。Web 会把 codeUrl 渲染成本地二维码；开发/测试环境可用带 HMAC 的 mock notify 把订单置为 `paid` 并幂等发放课程包 entitlement。Go API 支持关闭 pending/paying 微信 Native 订单，closed 订单不会被新下单复用。live Native 下单会用商户私钥签名并校验微信响应签名，live notify handler 会验签、解密、校验 appid/mchid/金额并幂等发放 entitlement；生产开放仍需要真实商户环境端到端联调、证书轮换和运营告警。Vue Admin `/orders` 只读查询订单状态，不能标记支付成功或发放权益。
- Browser mock-payment smoke 覆盖 Web 购买页二维码展示、签名 mock notify、订单 paid 状态、entitlement 和 paid 下载；它只能用于开发/测试，不替代真实微信商户端到端联调。
- Web `/wiki` and `/wiki/[id]` expose only published public Wiki entries through the Go API; draft, pending, rejected, and private review metadata stay hidden.
- Web `/blog` and `/blog/[id]` expose only published public Blog posts through the Go API; public responses use a DTO that hides review metadata, and the detail page can submit a `blog_post` report.
- Web 论坛页展示已发布公开帖子，支持登录用户提交待审核普通/问答/悬赏帖；详情页支持登录用户提交待审核回复，并允许楼主/admin 触发服务端最佳答案选择。
- Web `/me/forum` 展示当前用户自己的论坛帖子和回复，包括待审、已发布、已驳回状态以及自己的审核说明；可修改 draft/pending/needs_changes/rejected 内容并重新提交审核，公开论坛页仍只展示 published 内容。
- Web `/moments` 展示公开与互关可见学习动态；登录用户可发布 500 字以内动态、上传最多 9 张受控图片、设置公开/互关可见、点赞、评论、关注或屏蔽动态作者。动态图片通过 Go API 按关联动态可见性读取；视频、云存储和更细粒度媒体审计仍是后续工作。
- Web `/users/[id]` 展示公开用户主页，聚合当前访问者可见的动态、已发布 Blog、已发布论坛帖子和已发布论坛回复；Go API 不返回邮箱、审核字段或隐藏内容，互关动态和屏蔽关系由服务端判断。
- Web `/me/relations` 展示当前登录用户自己的关注、粉丝和互关好友列表，支持从服务端执行关注、取消关注和屏蔽；关系列表响应不返回邮箱。
- Web `/me/notifications` 展示当前用户自己的通知、未读数、逐条已读和全部已读操作。
- Web `/search` 和 Go API `/api/v1/search` 已提供基础公开搜索，覆盖课程、资料、课程包、Wiki、Blog 和论坛帖子。
- Web `/me/points` 和 `/me/membership` 已展示当前用户积分、积分流水、有效会员和公开会员套餐；Go API 已支持 admin 查询积分流水、维护积分规则、手动赠送/撤销会员并写操作日志。
- Vue Admin 已新增 `/points` 和 `/memberships`，用于积分流水查询、积分规则维护、会员手动发放和撤销。
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

## 8. 资料 Manifest 导入

已准备好的课程资料可以通过 manifest 导入，示例文件在 `data/material-manifest.example.json`。真实 PDF / DOCX / TXT 文件仍放在 `uploads/materials/...` 或部署挂载目录中，不提交到 Git。

```bash
cd services/api
go run ./cmd/import-materials -dry-run ../../data/material-manifest.example.json
go run ./cmd/import-materials ../../data/material-manifest.example.json
```

导入规则：

- manifest 中的 `uploads/materials/...` 会转换为内部 `storage_key=materials/...`。
- 文件必须真实存在，并且必须位于 `LOCAL_UPLOAD_DIR` 内。
- 危险路径如 `../../secret.pdf` 会被拒绝。
- 重复导入会更新已有资料并复用课程包绑定，不重复创建 material 或 package item。
- UTF-8 JSON manifests with a BOM are accepted, so Windows-generated manifest files should not fail solely because of byte-order marks.
- Automated smoke coverage exists in `TestMaterialManifestImportSmokeCoversPaidDownloadDelivery`: it imports mounted files through the manifest importer, verifies public package detail does not expose storage keys, checks free/login_required/paid download rules, grants the imported package, and verifies paid download audit logging.
- The import JSON includes a `report` block. Before importing real internal files, check `report.filesChecked`, `report.totalFileBytes`, `report.accessLevels`, `report.statuses`, `report.types`, `report.paidMaterials`, `report.packageItemLinks`, `report.packages`, and `report.duplicateFiles`.
- `-dry-run` uses the same validation/upsert/bind path inside a rolled-back transaction, so its `report` is the safest preflight acceptance artifact.
- `go run ./cmd/smoke ... -grant-package-access` can verify the internal manual-delivery path after import: paid download is denied before entitlement, an admin-only package grant is created, and paid download succeeds for the same test user.
- `npm --workspace @final-review/web run test:e2e:delivery` can verify the same manual-delivery path through real Web/Admin browser sessions when `E2E_DELIVERY_SMOKE=1` and fresh student/admin test accounts are configured.
- These smoke checks use fixture or operator-provided files only. Real course-file acceptance still requires running the import command against mounted internal materials in the target environment, then running the smoke with fresh student/admin test accounts.

## 9. 安全边界

- 不提交 `.env`、JWT 私钥、微信支付密钥、LLM API Key 或真实课程 PDF。
- `uploads/` 是运行时存储，除占位文件外被忽略。
- 生产环境不能使用固定验证码或 mock 支付。
- paid 资料下载必须经过 Go API 服务端鉴权，不能只靠前端隐藏按钮。
- 当前 paid 资料支持直接 material grant 和 published 课程包 grant。
- 公开课程包接口只能返回 `published` 课程包，并且包内 `items` 与 `materials` 都必须过滤到 `published` 资料；即使后台把 draft/pending/archived 资料预先绑定到包里，公开响应也不能泄露这些资料 ID。
- PDF 水印由 Go API 下载接口动态生成临时文件；如果 PDF 处理失败，下载会返回错误而不是静默直出未水印文件。
- AI 生成内容必须先进入 draft/review 流程，不能自动发布为正式内容。

- Go API sets baseline security headers on all responses and refuses unsafe CORS configuration: wildcard origins are rejected, and production requires exact HTTPS origins.

## 10. 开发入口

- 架构设计：`docs/architecture.md`
- API 文档：`docs/api.md`
- 数据库说明：`docs/database.md`
- 阶段总结：`docs/phase-summary-v2-mvp.md`
- 当前阶段对照总结：`docs/stage-summary-2026-06-24.md`
- AI 工作流：`docs/ai-workflow.md`
- 部署说明：`docs/deployment.md`
- Go API：`services/api/internal`
- Worker：`services/worker`
- Web：`apps/web`
- Admin：`apps/admin`

## 11. Current Admin Notes

- Vue Admin includes `/downloads` for successful material download audit logs.
- Vue Admin includes `/users` for admin-only user listing, role updates, and active/frozen status changes. The Go API prevents self role/status changes and restricts `super_admin` edits/grants to `super_admin` users.
- Vue Admin includes `/access-grants` for admin-only manual material/package access grants used in internal testing or after-sales delivery; it does not create payment orders or mark orders as paid.
- Vue Admin includes `/orders` for admin-only, read-only order inspection with buyer, package, amount, provider, status, and entitlement visibility.
- Vue Admin includes `/payment-reconciliation` for admin-only, read-only local checks across orders, payment records, order-source entitlements, risk flags, and open payment incidents. It is not a live WeChat merchant settlement reconciler and cannot mark orders paid or grant entitlement.
- Vue Admin includes `/payment-incidents` for admin-only WeChat callback anomaly triage, and `/dashboard` surfaces the current open-incident count. New incidents can optionally post a signed best-effort webhook through `PAYMENT_INCIDENT_WEBHOOK_URL`. Resolving or ignoring an incident records an operation note only; it does not mark orders paid or grant entitlement.
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
- Vue Admin includes `/media-assets` for admin-only moment image asset audit and stale unattached upload cleanup. Cleanup defaults to dry-run, never touches attached moment images, and writes an operation log.
- Vue Admin includes operation-log time filtering, CSV export, and a read-only retention policy panel.
- The download audit page reads `GET /api/v1/admin/downloads` and still depends on Go API server-side admin authorization.
- The user management page reads `GET /api/v1/admin/users` and writes `PATCH /api/v1/admin/users/:id`; it does not edit email, password credentials, membership, or points balance.
- The access-grants page reads/writes `GET/POST/DELETE /api/v1/admin/access-grants`; manual grants use `manual_admin`, are limited to published paid/member-only materials or published packages, and are revoked server-side.
- The package-management page reads/writes `GET/POST/PATCH/DELETE /api/v1/admin/packages`, `GET/POST /api/v1/admin/packages/:id/items`, and `DELETE /api/v1/admin/packages/:id/items/:itemId`; item binding supports `resourceType=material`, treats duplicate bindings idempotently, and removes bindings without deleting the underlying material.
- Admin material and download pages do not grant paid access, mutate download logs, or expose material `storage_key`.
- Admin analytics are based on successful server-side download logs and current report records; denied download attempts, page visits, search intent, and payment conversion are not included yet.
- Material review is one-way for the MVP: only pending materials can be approved or rejected through reviewer endpoints, rejected materials stay hidden from public pages, and rejection requires a review reason.
- Wiki submission is review-first for the MVP: creator/admin users can submit entries, public wiki APIs expose only published public entries, public responses hide review metadata, and rejected entries stay hidden.
- Wiki edit proposals are review-first for the MVP: creator/admin users can propose edits to published public entries from Web `/wiki/[id]`, reviewer queues compare base/current/proposed content, public content stays unchanged until approval, stale base versions return `409 proposal_stale`, and successful approval updates the live entry, increments its version, and writes `wiki_edit_histories`.
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

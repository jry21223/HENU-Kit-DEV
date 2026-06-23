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
- Go API 已实现 health/version、邮箱验证码登录、JWT cookie/token、角色中间件、学校/课程/资料/课程包接口、组织/课程/资料 admin CRUD、上传防护、资料下载权限、刷题提交、错题记录和基础薄弱点统计。
- 成功资料下载会写入服务端审计日志，失败鉴权、不安全路径和缺失文件不会记为成功下载。
- PDF 下载会在服务端生成临时轻水印副本，水印包含用户标识、资料 ID 和下载时间；源文件不会被覆盖。非 PDF 文件保持原样下载。
- 用户可以查看自己的成功下载记录；管理员可以查看全量下载审计日志。
- 课程包 catalog API 已实现，`material_access_grants.package_id` 可以在服务端解锁 published 课程包内的 paid 资料。
- Go API 与 Worker 已实现 mock AI task 流：用户创建任务，worker 完成 pending task，并把生成结果保存为待审核 draft。
- Next.js Web 已有首页、课程列表、课程详情、课程包展示、资料详情、课程刷题和学生邮箱登录页面。
- Next.js Web 已有个人中心 `/me`，登录用户可以维护学校、专业和年级绑定。
- Vue Admin 已有邮箱登录、路由守卫、仪表盘、课程管理、资料上传、资料状态流转、下载审计页面和 reviewer 可访问的 AI 草稿审核页；AI 草稿通过/驳回会记录审核意见。
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
- Web 课程详情页展示课程包价格、包含资料和支付联调状态。
- 不安全 storage key 返回 404。
- admin-only 组织/课程/资料变更。
- 上传文件名、后缀、内容和大小限制。
- Vue Admin dashboard、课程管理和资料管理 build/type 覆盖。
- 题目列表/详情不泄露答案。
- 刷题提交、错题用户隔离和 quiz attempt。
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
- Vue Admin includes admin-only all-status course listing and a course edit dialog.
- Vue Admin includes material status operations for `draft`, `pending`, `published`, and `archived`.
- Vue Admin includes material metadata editing; the actual storage key remains hidden, and the Go API rejects direct file-field mutation through metadata PATCH.
- Vue Admin includes `/ai/drafts` for reviewer/admin AI task visibility, draft approve/reject review, and review reason capture.
- Vue Admin includes `/analytics` for read-only successful-download trends, top materials, access breakdown, and course demand.
- The download audit page reads `GET /api/v1/admin/downloads` and still depends on Go API server-side admin authorization.
- Admin material and download pages do not grant paid access, mutate download logs, or expose material `storage_key`.
- Admin analytics are based on successful server-side download logs; denied download attempts, page visits, search intent, and payment conversion are not included yet.
- AI draft review is one-way for the MVP: repeat review of approved/rejected drafts is rejected, and review does not publish generated content automatically.
- Go API writes server-side `operation_logs` for organization, course, material, upload/status/archive, and AI draft review mutations; Vue Admin includes a read-only operation-log browser.
- Real AI publish-to-resource flows remain later work.
- Web `/me` updates profile binding through `PATCH /api/v1/auth/me`; school and major ids are validated by the Go API.

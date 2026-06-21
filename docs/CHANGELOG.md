# Changelog

## 2026-06-21 - Phase 16 WeChat Native Mock Implementation

### Changed

- Added WeChat Pay Native mock payment configuration and production mock blocking.
- Added WeChat Native order fields and a migration for integer-cent amounts, provider, out trade number, code URL, expiry, trade state, raw notify, and risk flag.
- Changed package order creation so it no longer returns EasyPay payment params.
- Added `/api/payments/wechat/native` and `/api/orders/[id]/status`.
- Soft-disabled legacy EasyPay notify/return routes with `410 Gone`.
- Updated the package purchase UI to create a WeChat Native mock payment and poll status.
- Replaced EasyPay unit tests with WeChat config, Native mock, and payment permission tests.

### Deferred

- Real WeChat Pay live API calls, request signing, callback verification/decryption, and callback-based entitlement grants are not implemented yet.

## 2026-06-21 - Phase 16 WeChat Native Direction Correction

### Changed

- Updated the payment direction from EasyPay to WeChat Pay Native.
- Rewrote README payment status to describe the current project as internal-test and WeChat Pay integration preparation, not production-ready payment.
- Added WeChat Pay Native environment variables to `.env.example`.
- Added WeChat Pay certificate and private-key ignore rules to `.gitignore`.
- Added `docs/WECHAT_PAY_NATIVE.md` with the target payment flow, mock/live mode rules, notify verification/decryption requirements, order state machine, and integration checklist.
- Added Phase 16 correction notes to API, database, roadmap, and self-check docs.

### Deferred

- Legacy EasyPay runtime code is still present and must be replaced in the next implementation round.
- Real WeChat Pay Native API calls, callback verification/decryption, database migration, QR payment UI, and payment tests are not implemented in this first correction round.

## 2026-06-21 - Phase 16

### Hardened

- 限定 mock fallback：仅非生产环境且无 `DATABASE_URL` 时使用 mock 数据。
- 加固验证码消费：验证码使用时通过条件 `updateMany` 再次检查未使用和未过期，降低并发重复消费风险。
- 加固上传校验：PDF/TXT 后缀和 MIME 必须匹配，继续限制空文件和 10MB 大小。
- 加固下载路径：新增 `src/lib/downloads.ts`，使用 `path.relative` 限定文件必须位于本地 `uploads/` 内。
- 下载响应增加 `X-Content-Type-Options: nosniff`。
- 加固 EasyPay 校验：签名验证同时检查商户号和 `sign_type`。
- README 重写为真实可验证状态描述，明确 demo 级能力和上线前待办。

### Tests

- 完善 `auth.test.ts`：邮箱域名、验证码哈希、验证码可用性、session 篡改和过期。
- 完善 `permissions.test.ts`：paid entitlement、未发布资料拒绝、下载路径穿越、mock fallback 生产环境禁用。
- 完善 `uploads.test.ts`：后缀/MIME 绕过、危险后缀、空文件、超大文件。
- 完善 `easypay.test.ts`：签名、商户号、`sign_type`、金额篡改和支付 URL。
- 完善 `questions.test.ts`：公开题目不泄露答案/解析、答案提交绕过尝试。
- 完善 `pdf-watermark.test.ts`：测试内生成 PDF，验证不覆盖原始字节且页数不变。

### Verified

- `npm run typecheck` 通过。
- `npm run lint` 通过。
- `npm test` 通过。

### Not Added

- 未新增产品功能。
- 未接真实邮件、真实 AI、真实支付商户生产配置。
- 未补完整 E2E、限流、审计日志、对象存储和病毒扫描。

## 2026-06-20 - Phase 0

### Added

- 初始化项目文档体系。
- 记录当前仓库为空、无既有技术栈、无可复用模块。
- 明确推荐 Next.js MVP 技术路线。
- 明确长期阶段路线图。
- 明确 API 规划、数据库规划和关键业务规则。
- 明确自我检验协议和 Phase 1 文件级 TODO。

### Not Added

- 未初始化 Next.js 项目。
- 未新增业务页面。
- 未新增 API。
- 未接入数据库。
- 未接入登录、支付、AI 或后台功能。

## 2026-06-20 - Phase 1

### Added

- 初始化 Next.js App Router、React、TypeScript、Tailwind CSS 项目。
- 新增首页、登录页、学校选择页、课程列表页、课程详情页、资料详情页、管理后台首页和 404 页面。
- 新增河南大学软件学院课程与资料 mock 数据。
- 新增课程卡片、资料卡片、权限标签、筛选表单和基础布局组件。
- 新增本地 `uploads/.gitkeep` 占位。

### Verified

- `npm install` 成功。
- `npm audit` 显示 0 vulnerabilities。
- `npm run typecheck` 通过。
- `npm run lint` 通过。
- `npm run build` 通过。

### Not Added

- 未接入数据库。
- 未创建 API。
- 未实现真实邮箱登录。
- 未实现真实下载权限。
- 未实现 admin 真实权限和后台维护能力。
- 未实现题库、错题本、课程包、支付、投稿审核、AI、PDF 水印和统计。

## 2026-06-20 - Phase 2

### Added

- 添加 Prisma、Prisma Client、`prisma.config.ts`、PostgreSQL Docker Compose 和 `.env.example`。
- 新增 `prisma/schema.prisma`，覆盖核心表和后续预留表。
- 新增 `prisma/seed.ts`，提供河南大学软件学院课程和资料示例数据。
- 新增 DB-first catalog/course/material service，无 `DATABASE_URL` 时 fallback 到 Phase 1 mock。
- 页面改为通过 service 获取课程和资料。

### Verified

- `npm install` 成功。
- `npm audit` 显示 0 vulnerabilities。
- `npm run db:generate` 通过。
- `npm run typecheck` 通过。
- `npm run lint` 通过。
- `npm run build` 通过。

### Verified

- Docker daemon 初始未运行，已启动 Docker Desktop 并恢复 Docker API。
- 官方 Docker Hub `postgres:17-alpine` 拉取超时，改用 `mirror.gcr.io/library/postgres:17-alpine`。
- PostgreSQL 容器健康检查通过。
- Windows 主机 Prisma schema engine 写库失败，Linux Node Docker 容器内 `npm run db:migrate -- --name init` 成功。
- `npm run db:seed` 成功，seed 结果为 1 schools、2 majors、6 courses、12 materials。
- DB-backed HTTP 检查通过。

### Not Added

- 未创建 API Route Handlers。
- 未实现邮箱验证码登录。
- 未实现下载权限、admin 后台写操作、题库、错题本、支付、投稿审核、AI、PDF 水印和统计。

## 2026-06-20 - Phase 3

### Added

- 新增学生邮箱验证码登录 API。
- 新增邮箱白名单、验证码、session 签名和当前用户工具。
- 新增登录页交互表单。
- 新增 header 登录态展示和退出按钮。
- 新增 auth 单元测试。

### Verified

- 非白名单邮箱被拒绝。
- 合法学生邮箱可发送验证码。
- API 响应不包含验证码。
- 正确验证码可登录并创建用户。
- 验证码重复使用失败。
- `/api/auth/me` 登录前后状态正确。
- `/api/auth/logout` 可清理 session。
- `npm test`、`npm run typecheck`、`npm run lint`、`npm run build` 通过。

### Not Added

- 未接真实邮件服务。
- 未实现发送频率限制。
- 未实现 admin 后台权限拦截。

## 2026-06-20 - Phase 4

### Added

- 新增学校、学院、专业、课程、课程资料和资料详情只读 API。
- API 返回字段做公开字段筛选。
- API 层复用 DB-first service，普通用户只返回 published 内容。

### Verified

- `/api/schools`、`/api/colleges`、`/api/majors`、`/api/courses`、`/api/courses/:id`、`/api/courses/:id/materials`、`/api/materials/:id` 验收通过。
- 无效课程和资料返回 404。
- DB-only 行为检查通过，未回退到 mock。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm run build` 通过。

### Not Added

- 未实现资料下载 API。
- 未实现 admin 写操作。

## 2026-06-20 - Phase 5

### Added

- 新增资料下载 API。
- 新增后端下载权限判断。
- seed 生成本地 mock PDF 文件。
- 资料详情页显示下载按钮或 locked 状态。
- 新增权限单元测试。

### Verified

- free 资料未登录可下载。
- login_required 资料未登录返回 401。
- 登录后 login_required 资料可下载。
- paid 资料返回 402，不能绕过。
- downloads 记录按成功下载增加。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未实现 paid entitlement。
- 未实现 PDF 水印。
- 未实现真实上传。

## 2026-06-20 - Phase 6

### Added

- 新增服务端 admin 角色检查。
- 保护 admin 后台页面。
- 新增课程和资料管理基础页面。
- 新增 admin courses/materials 写 API。
- 新增 admin 上传 API，限制 PDF/TXT 和 10MB。

### Verified

- 匿名访问 admin 写 API 返回 401。
- student 访问 admin 写 API 返回 403。
- admin 可创建并发布课程。
- admin 可创建并发布资料。
- student 上传被拒绝。
- admin PDF 上传成功。
- 危险类型上传被拒绝。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 后台 UI 表单仍是基础壳。
- 未实现 reviewer 审核。
- 未实现删除操作，使用状态归档路径。

## 2026-06-20 - Phase 7

### Added

- 新增单选题和判断题基础刷题能力。
- seed 新增知识点和 4 道 published 题目。
- 新增题库 service 和题目公开字段映射，题目列表不返回答案和解析。
- 新增 `GET /api/courses/:id/questions`、`GET /api/questions/:id`、`POST /api/questions/:id/submit`。
- 新增 `GET /api/users/me/wrong-questions` 基础错题列表接口。
- 新增 `/courses/:id/practice`、`/practice/:sessionId`、`/practice/result/:sessionId` 页面。
- 新增刷题客户端组件，提交后展示对错、正确答案和解析。
- 登录用户答错后写入 `wrong_questions`，重复答错不重复记录。
- 新增题库单元测试。

### Verified

- `GET /api/courses/discrete-math/questions` 返回题目且不包含 `answer` / `explanation`。
- 匿名用户可提交答案并看到解析，但不保存错题。
- 登录学生答错会保存错题。
- 同一用户重复答错同一题仍只有 1 条错题记录。
- `/api/users/me/wrong-questions` 未登录返回 401，登录后只返回当前用户错题。
- Browser 桌面和 390px 移动端核验通过，无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未新增持久化练习 session 表，结果页当前使用浏览器本地结果。
- 未实现完整错题本页面、筛选、移除、重新练习和薄弱知识点统计，留到 Phase 8。
- 未实现 admin 题目管理。
- 未实现多选题、填空题、计算题和证明题。

## 2026-06-20 - Phase 8

### Added

- 新增 `/me/wrong-questions` 错题本页面。
- 新增 `/me/stats` 薄弱知识点统计页面。
- 新增 `/me` 重定向到错题本。
- 顶部导航新增“我的复习”。
- 新增 `GET /api/me/wrong-questions`，支持课程和知识点筛选。
- 新增 `DELETE /api/me/wrong-questions/:id`，只允许删除当前用户自己的错题。
- 新增 `GET /api/me/weak-points`。
- 新增薄弱知识点统计纯函数和单元测试。

### Verified

- 登录用户可查看自己的错题。
- 用户 A 看不到、删不掉用户 B 的错题。
- 知识点筛选返回预期结果。
- 删除自己的错题返回 200，删除别人的错题返回 404。
- 薄弱知识点统计返回课程和知识点分布。
- Browser 桌面和 390px 移动端核验通过，无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未实现只练错题集合的专项 session。
- 未实现长期练习趋势统计。
- 未实现 admin 题目管理。

## 2026-06-20 - Phase 9

### Added

- seed 新增离散数学期末复习包。
- 新增 package/entitlement service。
- 新增公开课程包 API：`GET /api/packages`、`GET /api/packages/:id`。
- 新增 admin 课程包 API：`POST /api/admin/packages`、`PATCH /api/admin/packages/:id`。
- 新增 admin 手动授权 API：`POST /api/admin/entitlements`。
- 新增 `/packages`、`/packages/:id` 和 `/admin/packages` 页面。
- paid 下载 API 接入 entitlement 校验。
- 资料详情页根据授权状态展示已解锁或需要解锁。
- 权限测试覆盖 paid 未登录、未授权和已授权。

### Verified

- 未授权学生下载 paid 资料返回 402。
- student 调用 admin 授权 API 返回 403。
- admin 手动授权课程包返回 201。
- 授权后同一学生可以下载包内 paid 资料。
- admin 可以创建并发布课程包。
- 公开课程包详情可访问。
- Browser 桌面和 390px 移动端核验通过，无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未接真实支付。
- 未实现 entitlement 撤销和授权列表。
- 后台课程包 UI 仍是基础查看页，写操作通过 API 验收。

## 2026-06-20 - Phase 10

### Added

- 新增易支付兼容签名工具，支持参数排序、签名生成和常量时间签名校验。
- 新增订单 service，支持课程包订单创建、当前用户订单查询、支付通知结算和幂等授权发放。
- 新增订单 API：`GET /api/orders`、`POST /api/orders`、`GET /api/orders/:id`。
- 新增支付 API：`POST /api/payments/easypay/notify`、`GET /api/payments/easypay/notify`、`GET /api/payments/easypay/return`。
- 新增 `/me/orders` 订单页。
- 课程包详情页新增购买按钮，真实支付网关未配置时只允许本地回调验收。
- `.env.example` 新增 `EASYPAY_*` 配置。
- 新增易支付签名单元测试。

### Verified

- 未登录创建订单返回 401。
- 登录且邮箱已验证用户可以创建课程包订单。
- 坏签名、金额不匹配和非成功支付状态不会发放权限。
- 成功支付回调会把订单置为 paid，并发放 package entitlement。
- 重复成功回调保持幂等，entitlement 不重复。
- 支付成功后同一用户可下载包内 paid 资料。
- 同步返回 URL 验签后跳转订单页并完成结算。
- Browser 桌面和 390px 移动端核验课程包页和订单页无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未绑定真实商户号和真实支付网关。
- 未实现退款、取消、支付查询和后台订单管理。
- 未实现订单运营转化统计。

## 2026-06-20 - Phase 11

### Added

- 新增共享上传校验工具，限制 PDF/TXT 和 10MB，并收紧文件名清洗。
- 新增投稿规则工具，覆盖 reviewer/admin 审核角色、状态映射、审核动作和驳回说明校验。
- 新增 submission service，支持创建投稿、我的投稿列表、审核列表和审核通过生成资料。
- 新增 API：`POST /api/submissions`、`GET /api/me/submissions`、`GET /api/admin/submissions`、`PATCH /api/admin/submissions/:id/review`。
- 新增 `/me/submissions` 页面，支持学生提交资料和查看审核进度。
- 新增 `/admin/submissions` 页面，支持 reviewer/admin 查看、筛选和审核投稿。
- 管理后台首页新增可点击模块入口。
- 顶部导航新增资料共建入口。
- 新增上传和投稿规则单元测试。

### Verified

- 匿名投稿返回 401。
- 危险文件类型投稿返回 400。
- 登录学生投稿返回 201，状态为 pending。
- pending 投稿对应 material URL 前台返回 404。
- student 访问审核列表返回 403。
- reviewer/admin 审核通过返回 200，并生成可前台访问的 material。
- reviewer/admin 驳回返回 200，且不会生成 material。
- 学生我的投稿 API、admin 审核 API 和页面访问返回 200。
- Browser 桌面和 390px 移动端核验 `/me/submissions` 与 `/admin/submissions` 无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未实现投稿归档和删除。
- 未实现投稿文件的单独下载权限细化。
- 未实现贡献者角色升级、贡献统计和奖励。
- 未实现 AI 生成内容审核；Phase 12 已补充 AI 草稿任务基础版，独立审核队列仍待后续。

## 2026-06-20 - Phase 12

### Added

- 新增 AI job 规则工具，覆盖输出类型、状态映射和本地草稿内容生成。
- 新增 AI job service，支持任务列表、任务详情、成功生成 draft material 和失败状态记录。
- 新增 API：`GET /api/admin/ai-jobs`、`POST /api/admin/ai-jobs`、`GET /api/admin/ai-jobs/:id`。
- 新增 `/admin/ai-jobs` 后台页面，支持创建本地 AI 草稿任务、查看任务和查看可用来源资料。
- 管理后台首页新增 AI 任务入口。
- 新增 AI job 单元测试。

### Verified

- 匿名访问 admin AI jobs API 返回 401。
- student 访问 admin AI jobs API 返回 403。
- admin 创建成功任务返回 201，状态为 succeeded。
- 成功任务创建 draft material。
- draft material 前台资料 API 返回 404，不会自动发布。
- admin 可查看单个 AI job 详情。
- admin 创建模拟失败任务返回 201，状态为 failed 且包含 error。
- Browser 桌面和 390px 移动端核验 `/admin/ai-jobs` 无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未接真实 AI 模型。
- 未实现 PDF/Word 文本提取。
- 未实现公式保真处理。
- 未实现 AI job 重试、取消和异步 worker。
- 未实现 AI 生成结果的独立审核队列，当前生成 draft material 后仍需 admin 后续手动处理。

## 2026-06-20 - Phase 13

### Added

- 新增 `pdf-lib` 和 `@pdf-lib/fontkit`，用于 PDF 动态水印。
- 新增 `src/lib/pdf-watermark.ts`，支持水印文本构造、中文字体嵌入和每页斜向浅灰水印。
- 下载 API 集成 PDF 水印，PDF 响应头返回 `X-Watermark-Applied: true`。
- 非 PDF 下载保留原始内容，响应头返回 `X-Watermark-Applied: false`。
- `.env.example` 新增 `PDF_WATERMARK_FONT_PATH`。
- 新增 PDF 水印单元测试。

### Verified

- free PDF 匿名下载返回 200，应用水印。
- login_required PDF 未登录返回 401。
- 登录学生下载 login_required PDF 返回 200，应用水印。
- 不同用户/状态下载的水印 PDF 字节不同。
- 原 PDF 文件 hash 下载前后不变。
- TXT 非 PDF 下载返回 200，不应用水印，内容长度不变。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未实现对象存储临时水印文件。
- 未实现大 PDF 流式处理和超时保护。
- 未实现水印配置后台。
- 未实现对非 PDF 的转换或预览水印。

## 2026-06-20 - Phase 14

### Added

- 新增 analytics 聚合工具，支持热门课程、高下载资料和高错题知识点聚合。
- 新增 analytics service，统计用户、认证、课程、资料、下载、题目、错题和已支付订单。
- 新增 `GET /api/admin/analytics`。
- 新增 `/admin/analytics` 后台页面。
- 新增 analytics 单元测试。

### Verified

- 匿名访问 admin analytics API 返回 401。
- student 访问 admin analytics API 返回 403。
- admin 访问 admin analytics API 返回 200。
- analytics API 响应不包含用户邮箱明细。
- 返回热门课程、高下载资料和高错题知识点。
- Browser 桌面和 390px 移动端核验 `/admin/analytics` 无横向溢出。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未实现课程访问日志表，热门课程当前以课程相关下载量近似。
- 未实现分日趋势和图表。
- 未实现订单转化率完整漏斗。
- 未实现离线统计任务。

## 2026-06-20 - Phase 15

### Added

- 对首页、登录、学校、课程、课程详情、资料详情、刷题、结果、错题本、投稿、订单和统计页做 390px 移动端巡检。
- 课程筛选控件高度提升到 44px。
- 刷题单选控件增大，保持整行选项可点击。
- 清理首页、学校页、404 和 admin 首页过期阶段文案。

### Verified

- 12 个关键页面在 390px 视口无横向溢出。
- 可见按钮、select、textarea 等控件没有明显过小问题。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

### Not Added

- 未新增底部导航。
- 未做大规模 UI 重构。
- 未增加动画。

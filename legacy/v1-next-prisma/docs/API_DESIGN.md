# API Design

## Phase 16 Payment Direction Note

- Target payment provider: WeChat Pay Native (`wechat_native`).
- EasyPay routes are legacy code and must not be treated as the current default payment path.
- Mock WeChat Native order creation and order status polling are implemented. Real WeChat Pay live API integration is still deferred.
- Frontend polling must never grant paid access. Entitlements are granted only by server-side payment confirmation.
- Paid material download must always go through a server-side entitlement check.

Target payment APIs:

- `POST /api/orders`: create a local `pending` order for a published course package.
- `POST /api/payments/wechat/native`: create a WeChat Native QR code for the user's own pending/paying order.
- `GET /api/orders/:id/status`: return order state only; no entitlement side effects.
- `POST /api/payments/wechat/notify`: verify signature, decrypt resource, validate order and amount, then grant entitlement idempotently.
- `POST /api/payments/wechat/close`: close the user's own pending/paying order, with admin override.

## 设计原则

- API 必须服务当前 Phase，不提前实现后续复杂能力。
- 所有权限判断必须在服务端完成。
- 普通用户接口只返回必要字段，不泄露内部状态和敏感字段。
- draft、pending_review、archived 内容默认不返回给普通用户。
- paid 内容不能通过直接 URL 绕过下载 API。

## Auth

### POST `/api/auth/send-code`

请求：

- `email`

逻辑：

- 校验邮箱格式。
- 校验学校邮箱白名单。
- 生成 6 位验证码。
- 写入 `email_verifications`。
- 开发环境可在服务端日志输出验证码。
- 生产环境使用邮件服务发送，不暴露验证码到前端。

### POST `/api/auth/verify-code`

请求：

- `email`
- `code`
- `school_id`
- `major_id`
- `grade`

逻辑：

- 校验验证码存在、未过期、未使用。
- 创建或更新用户。
- 设置 `email_verified=true`。
- 建立 session。
- 将验证码标记为 used。

### GET `/api/auth/me`

返回当前登录用户的安全字段。

### POST `/api/auth/logout`

清理 session。

## School

### GET `/api/schools`

返回可用学校列表。

### GET `/api/colleges?schoolId=`

返回指定学校下的学院列表。

### GET `/api/majors?schoolId=&collegeId=`

返回指定学校和学院下的专业列表。

## Courses

### GET `/api/courses?schoolId=&majorId=&grade=`

返回 published 课程列表，支持学校、专业、年级筛选。

### GET `/api/courses/:id`

返回课程详情、考试范围和基础统计。

### GET `/api/courses/:courseId/materials`

返回课程下 published 资料列表。

## Materials

### GET `/api/materials/:id`

返回资料详情和预览内容。普通用户只能访问 published 资料。

### GET `/api/materials/:id/download`

逻辑：

- 查询资料。
- 检查资料状态是否 published。
- 检查 access_level。
- free 允许未登录下载。
- login_required 要求登录且邮箱验证。
- paid 要求 entitlement。
- 记录 downloads。
- PDF 文件动态生成水印副本，响应头 `X-Watermark-Applied: true`。
- 非 PDF 文件正常下载，响应头 `X-Watermark-Applied: false`。
- 不覆盖原始文件。

## Questions

Phase 7 已实现基础版。当前只支持单选题和判断题；多选、填空、计算、证明题保留枚举但不在前台开放完整流程。

### GET `/api/courses/:courseId/questions`

返回课程题目列表，不返回答案和解析。

### GET `/api/questions/:id`

返回单题详情。未提交前不返回答案。

### POST `/api/questions/:id/submit`

请求：

- `answer`

逻辑：

- 服务端判题。
- 返回对错和解析。
- 登录用户答错后写入错题本。
- 未登录用户可提交试做，但不写入错题。
- 同一用户同一题错题记录通过唯一约束去重。

## Wrong Questions

Phase 7 已实现基础错题列表：

### GET `/api/users/me/wrong-questions`

返回当前登录用户的基础错题列表，不返回答案和解析。未登录返回 401。

Phase 8 已实现以下用户 API：

### GET `/api/me/wrong-questions?courseId=&knowledgePointId=`

返回当前用户错题，支持按课程和知识点筛选，不返回答案和解析。未登录返回 401。

### DELETE `/api/me/wrong-questions/:id`

移除当前用户的一条错题记录。不能删除其他用户错题，不能删除题库原题。

### GET `/api/me/weak-points`

返回当前用户薄弱知识点统计，按课程和知识点聚合当前错题记录。

## Admin Courses

### POST `/api/admin/courses`

admin 创建课程。

### PATCH `/api/admin/courses/:id`

admin 编辑课程。

### PATCH `/api/admin/courses/:id/status`

admin 修改课程状态。

## Admin Materials

### POST `/api/admin/materials`

admin 创建资料记录。

### PATCH `/api/admin/materials/:id`

admin 编辑资料。

### POST `/api/admin/materials/upload`

admin 上传资料文件。必须限制类型和大小。

### PATCH `/api/admin/materials/:id/status`

admin 修改资料状态。

## Packages

Phase 9 已实现基础版。

### GET `/api/packages`

返回可查看的 published 课程包列表，并根据当前登录用户返回 `unlocked`。

### GET `/api/packages/:id`

返回课程包详情、包内资源和当前用户解锁状态。

### POST `/api/admin/packages`

admin 创建课程包。

### PATCH `/api/admin/packages/:id`

admin 编辑课程包。

### POST `/api/admin/entitlements`

admin 手动给用户发放 `package` 或 `material` 权限。Phase 9 用于支付前的手动授权，必须服务端校验 admin 角色。

## Orders And Payments

### POST `/api/orders`

创建课程包订单。目标支付方案为微信支付 Native，订单必须绑定当前登录且邮箱已验证的用户和已发布课程包。

请求：

- `packageId`

返回：

- `orderId`
- `status`
- 已拥有有效 entitlement 时返回 `already_owned`

约束：

- 未登录或邮箱未验证返回 401。
- package 必须存在且为 published。
- 订单金额必须从服务端 package 读取，前端不能传入金额。
- `payment_provider` 目标值为 `wechat_native`。
- 本轮仅完成方向纠偏，具体 WeChat Native 下单逻辑在下一轮实现。

### GET `/api/orders`

查询当前用户订单列表。

### GET `/api/orders/:id`

查询当前用户单个订单。只能查询当前用户自己的订单。

### POST `/api/payments/wechat/native`

为当前用户自己的 pending/paying 订单发起微信 Native 下单，返回 `codeUrl`、`expiresAt` 和 `status=paying`。

约束：

- 必须登录。
- 用户只能支付自己的订单，admin 不能代替用户发起付款。
- 生产环境禁止 mock。
- live 模式缺少微信商户配置时必须返回配置错误，不能 fallback 到 mock。

### GET `/api/orders/:id/status`

查询订单状态。该接口只返回状态，不允许触发授权。

### POST `/api/payments/wechat/notify`

微信支付服务器异步回调。必须验签、解密 resource、校验 `out_trade_no`、校验 `amount.total`、校验 appid/mchid，并幂等发放 entitlement。

### POST `/api/payments/wechat/close`

关闭当前用户自己的 pending/paying 订单。live 模式下需要调用微信关单接口。

### Legacy EasyPay Routes

以下接口为遗留路径，下一轮应删除或替换，不再作为默认支付方案：

- `POST /api/payments/easypay/notify`
- `GET /api/payments/easypay/return`

## Submissions

### POST `/api/submissions`

学生投稿，状态进入 pending。Phase 11 已实现。

请求类型：

- `multipart/form-data`

字段：

- `course_id`
- `title`
- `description`
- `file`

约束：

- 必须登录且邮箱已验证。
- 课程必须存在且已发布。
- 文件只允许 PDF/TXT，大小不超过 10MB。
- 创建后只生成 submission，不生成 material，不会直接发布。

### GET `/api/me/submissions`

当前用户投稿列表。只能返回当前登录用户自己的投稿。

### GET `/api/admin/submissions`

reviewer/admin 查看投稿。支持 `status=pending|approved|rejected|archived`。

### PATCH `/api/admin/submissions/:id/review`

reviewer/admin 审核投稿。

请求：

- `action=approve|reject`
- `review_comment`

约束：

- 只有 pending 投稿可以审核。
- 驳回必须填写 `review_comment`。
- approve 后由服务端创建 `material`，状态为 `published`，默认 `login_required`。
- student 不能访问该接口。

## AI Jobs

### POST `/api/admin/ai-jobs`

admin 创建 AI 生成任务。Phase 12 已实现本地草稿生成流程，不接真实 AI。

请求：

- `course_id`
- `output_type=knowledge_note|mock_paper|answer|quick_review|past_exam|other`
- `input_material_ids`，数组或逗号分隔字符串，可选
- `simulate_failure`，开发验收用，可选

约束：

- 仅 admin 可访问。
- 课程必须已发布。
- 来源资料必须属于该课程且已发布。
- 成功任务会生成 `draft` material，不会自动发布。
- 失败任务必须保存 `error`。

### GET `/api/admin/ai-jobs`

admin 查看 AI 任务列表。支持 `status=queued|running|succeeded|failed|cancelled`。

### GET `/api/admin/ai-jobs/:id`

admin 查看 AI 任务详情。

## Analytics

### GET `/api/admin/analytics`

admin 查看运营统计。Phase 14 已实现基础版。

返回：

- 用户数和邮箱认证数。
- 课程、资料、下载、题目、错题和已支付订单计数。
- 热门课程，当前以课程相关下载量近似。
- 高下载资料。
- 高错题知识点。

约束：

- 仅 admin 可访问。
- 不返回用户邮箱明细。
- 数据为空时返回空数组和 0 计数。

## 权限矩阵

未登录用户：

- 可以访问首页、课程列表、课程详情、资料简介和 free 下载。
- 不能下载 login_required 资料。
- 不能访问 paid 内容。
- 不能保存错题。
- 不能访问后台。

已登录学生：

- 可以下载 free 和 login_required 资料。
- 可以在线刷题和保存错题。
- 不能访问 admin 后台。

已授权用户：

- 可以访问已授权的 paid 内容。

reviewer：

- 当前可以审核投稿。
- AI 草稿审核权限后续细化。
- 不能管理用户和支付。

admin：

- 可以管理学校、专业、课程、资料、题库、用户、权限、订单和 AI 任务。

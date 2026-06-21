# Database Design

## 数据库原则

- 第一阶段先用 mock，Phase 2 再接 PostgreSQL。
- 所有核心业务表必须有 `created_at` 和 `updated_at`，历史或日志表按需使用事件时间字段。
- 内容表必须有 `status`，避免误发布。
- 权限相关表必须支持未来 paid 和 entitlement。
- 预留题库、错题、订单、AI、投稿等能力，但不提前实现业务流程。

## 核心表

### users

- `id`
- `email`
- `name`
- `school_id`
- `major_id`
- `grade`
- `role`
- `email_verified`
- `created_at`
- `updated_at`

### schools

- `id`
- `name`
- `slug`
- `email_domains`
- `status`
- `created_at`
- `updated_at`

### colleges

- `id`
- `school_id`
- `name`
- `created_at`
- `updated_at`

### majors

- `id`
- `school_id`
- `college_id`
- `name`
- `slug`
- `created_at`
- `updated_at`

### courses

- `id`
- `school_id`
- `college_id`
- `grades`
- `name`
- `slug`
- `description`
- `exam_scope`
- `status`
- `created_at`
- `updated_at`

说明：

- 实际 Phase 2 schema 使用 `grades` 字符串数组，避免同一课程因适用多个年级重复建表记录。
- 实际 Phase 2 schema 使用 `course_majors` 关联表，避免离散数学、高等数学A等公共课程因适用多个专业重复建课程记录。

### course_majors

- `course_id`
- `major_id`

### materials

- `id`
- `course_id`
- `title`
- `type`
- `description`
- `file_url`
- `file_name`
- `file_size`
- `preview_content`
- `access_level`
- `status`
- `created_by`
- `created_at`
- `updated_at`

### downloads

- `id`
- `user_id`
- `material_id`
- `ip`
- `user_agent`
- `downloaded_at`

### email_verifications

- `id`
- `email`
- `code`
- `expires_at`
- `used`
- `created_at`

## 题库与后续预留表

说明：

- `knowledge_points`、`questions`、`wrong_questions` 已在 Phase 7 启用基础版。
- 题目答案和解析仍存储在 `questions`，但公开题目 API 必须过滤，不得在列表和单题读取中返回。
- `packages`、`package_items`、`entitlements` 已在 Phase 9 启用基础版。
- `orders` 已在 Phase 10 启用基础版。
- `submissions` 已在 Phase 11 启用基础版。
- `ai_jobs` 已在 Phase 12 启用基础版。

### knowledge_points

- `id`
- `course_id`
- `parent_id`
- `title`
- `description`
- `sort_order`

### questions

- `id`
- `course_id`
- `knowledge_point_id`
- `type`
- `stem`
- `options`
- `answer`
- `explanation`
- `difficulty`
- `status`
- `created_at`
- `updated_at`

### wrong_questions

- `id`
- `user_id`
- `question_id`
- `created_at`

说明：

- Phase 7 使用 `user_id + question_id` 唯一约束避免重复错题。
- 错题查询必须按当前 session 用户过滤。

### orders

- `id`
- `user_id`
- `amount`
- `status`
- `product_type`
- `product_id`
- `created_at`
- `paid_at`

说明：

- Phase 10 已启用，用于课程复习包支付订单。
- `product_type=package` 表示购买课程复习包。
- 当前用本地订单 `id` 作为易支付 `out_trade_no`。
- 当前 schema 尚未保存第三方交易号；后续若要做支付查询、退款或对账，建议新增 `provider`、`provider_trade_no`、`raw_notify` 等字段。

### entitlements

- `id`
- `user_id`
- `resource_type`
- `resource_id`
- `source`
- `expires_at`
- `created_at`

说明：

- Phase 9 支持 `resource_type=package` 和 `resource_type=material`。
- paid 下载会检查直接资料授权或包含该资料的 package 授权。
- `expires_at` 为空表示长期有效。
- Phase 10 支付成功后使用 `source=easypay` 自动发放课程包 entitlement。

### packages

- `id`
- `title`
- `description`
- `school_id`
- `major_id`
- `grade`
- `price`
- `status`
- `created_at`
- `updated_at`

说明：

- Phase 9 已启用，用于课程复习包。
- 当前 seed 包含 `discrete-math-final-package`。

### package_items

- `id`
- `package_id`
- `resource_type`
- `resource_id`

说明：

- Phase 9 基础版主要使用 `resource_type=material`。

### submissions

- `id`
- `user_id`
- `course_id`
- `title`
- `description`
- `file_url`
- `status`
- `review_comment`
- `created_at`
- `reviewed_at`

说明：

- Phase 11 已启用，用于学生资料共建和人工审核。
- 创建时状态为 `pending`，不会直接生成资料。
- `approved` 后由审核接口创建 `materials` 记录。
- `rejected` 必须保留 `review_comment`。

### ai_jobs

- `id`
- `course_id`
- `input_material_ids`
- `output_type`
- `status`
- `result`
- `error`
- `created_at`
- `updated_at`

说明：

- Phase 12 已启用，用于记录 AI 辅助内容生成任务。
- 当前不接真实 AI，成功任务同步生成 draft material。
- `result` 保存草稿资料 ID、来源资料 ID 和人工审核提示。
- `failed` 状态必须保存 `error`。
- AI 任务不得直接生成 published 内容。

## 枚举

### user.role

- `student`
- `admin`
- `reviewer`
- `contributor`

### course.status

- `draft`
- `published`
- `archived`

### material.type

- `knowledge_note`
- `mock_paper`
- `answer`
- `quick_review`
- `past_exam`
- `other`

### material.access_level

- `free`
- `login_required`
- `paid`

### material.status

- `draft`
- `pending_review`
- `published`
- `archived`

### question.type

- `single_choice`
- `multiple_choice`
- `true_false`
- `blank`
- `calculation`
- `proof`

### question.status

- `draft`
- `published`
- `archived`

### order.status

- `pending`
- `paid`
- `failed`
- `cancelled`
- `refunded`

### submission.status

- `pending`
- `approved`
- `rejected`
- `archived`

### ai_job.status

- `queued`
- `running`
- `succeeded`
- `failed`
- `cancelled`

## 初始 seed 数据

学校：

- 河南大学

学院：

- 软件学院

专业：

- 网络工程
- 软件工程

年级：

- 2023级
- 2024级

课程：

- 离散数学
- 概率论与数理统计A
- 大学物理
- 高等数学A
- 软件工程
- 移动开发

离散数学资料：

- 离散数学重点知识点讲义，`knowledge_note`，`login_required`
- 离散数学模拟卷一，`mock_paper`，`paid`
- 离散数学模拟卷二，`mock_paper`，`paid`
- 离散数学答案解析，`answer`，`paid`
- 离散数学考前速背版，`quick_review`，`login_required`

概率论与数理统计A资料：

- 概率论重点知识点讲义，`knowledge_note`，`login_required`
- 概率论模拟卷一，`mock_paper`，`paid`
- 概率论答案解析，`answer`，`paid`

大学物理资料：

- 大学物理电磁学重点整理，`knowledge_note`，`login_required`
- 大学物理模拟卷一，`mock_paper`，`paid`
- 大学物理答案解析，`answer`，`paid`

样例资料：

- 至少准备一个 `free` 资料用于未登录下载验收。

题库样例：

- 离散数学：命题逻辑单选题、图论判断题。
- 概率论与数理统计A：常见分布单选题。
- 大学物理：电磁学判断题。
- 当前 seed 共 4 个知识点、4 道 published 题目。

课程包样例：

- 离散数学期末复习包：包含 `discrete-mock-1`、`discrete-mock-2`、`discrete-answer`。

开发环境 admin：

- `admin@example.com`
- 仅开发环境使用，必须在 README 和 seed 注释中标明。

## 关键约束

- draft 资料不能展示给普通用户。
- pending_review 资料不能展示给普通用户。
- archived 资料不能展示给普通用户。
- paid 内容不能通过直接 URL 绕过。
- 验证码必须过期。
- 验证码只能使用一次。
- 学生邮箱域名必须校验。
- AI 生成内容不能自动发布。
- 学生投稿不能自动发布。
- 支付成功必须校验签名。
- 重复支付回调不能重复授权。
- 错题只能属于当前用户。
- admin 接口必须检查角色。
- 文件上传必须限制类型和大小。
- PDF 水印不能覆盖原文件。

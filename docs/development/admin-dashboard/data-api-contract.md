# 统一管理后台数据与 API 契约 V1.0

> 本文件必须与 [`../api-communication-spec.md`](../api-communication-spec.md) 同时使用。发生冲突时，以 API 与服务通信规范为准。

## 1. 通用协议

### 1.1 路径与字段

- 浏览器 API：`/api/v1/admin/*`。
- 内部服务 API：`/api/v1/internal/*`。
- 资源路径：复数名词 + `kebab-case`。
- JSON：`snake_case`。
- 时间：UTC RFC 3339。
- 新公开 ID：UUIDv4。
- 查询列表：`page`、`page_size`，排序字段白名单。

### 1.2 请求追踪

每个请求：

- 接收或生成 `request_id`；
- 响应头返回 `X-Request-Id`；
- 响应体返回 `request_id`；
- 下游调用、日志、事件、Outbox、邮件投递透传同一 ID。

### 1.3 成功响应

```json
{
  "data": {},
  "request_id": "req_01J..."
}
```

### 1.4 分页响应

```json
{
  "data": {
    "items": [],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 0,
      "total_pages": 0
    }
  },
  "request_id": "req_01J..."
}
```

### 1.5 错误响应

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "请求参数不合法",
    "details": null
  },
  "request_id": "req_01J..."
}
```

### 1.6 幂等

所有产生持久副作用的 `POST`、`PATCH` 必须接受 `Idempotency-Key`。

- 唯一范围：`principal_id + method + normalized_route + key`；浏览器使用已认证 actor，内部服务使用 client/service id。
- 相同 Key + 相同请求：返回首次结果。
- 相同 Key + 不同请求：409 `IDEMPOTENCY_KEY_CONFLICT`。
- 最终幂等由数据库唯一约束保证，Redis 仅作加速或短期防重。

### 1.7 并发更新

审核、调档、来源配置和用户状态变更使用乐观版本：

```json
{
  "expected_version": 7
}
```

版本不一致：409 `RESOURCE_VERSION_CONFLICT`。

## 2. 服务 HMAC

内部调用使用：

```text
X-Service-Id
X-Key-Id
X-Timestamp
X-Nonce
X-Signature
```

Canonical String：

```text
METHOD
PATH_AND_QUERY
X-Timestamp
X-Nonce
SHA256(BODY)
```

- HMAC-SHA256；
- 时间容差 5 分钟；
- Nonce 由 Redis 原子去重；
- 高风险接口 Redis 不可用时 Fail Closed；
- 日志不记录 Secret、完整签名或完整 Body。

## 3. 数据 Owner

| 数据 | Owner | 管理后台读取方式 | 禁止事项 |
|---|---|---|---|
| 用户、邮箱身份、Session、角色、会员、Entitlement | Platform Core | Admin API 直接读取 Core 数据 | Study/Quiz/Food 直接写 Core 用户表 |
| 学业档案、通知订阅 | Platform Core | Admin API | 用自报学院直接授予权限 |
| 邮件投递、抑制名单、DLQ | Platform Core / Platform Worker | Admin API | 业务服务直接操作 SMTP 队列 |
| 统一待办 | Platform Core Ops | 消费业务事件和保存引用 | 复制完整业务正文 |
| 校园通知来源、正文、版本、分发计划 | Notice Service | HMAC 内部 API / 事件摘要 | Platform Core 跨库读 Notice DB |
| 资料、下载、资料审核 | Study | Adapter / 内部摘要 API | Core 保存资料正文或文件 |
| 题库、题目、作答、题目反馈 | QuizCraft | Adapter / 内部摘要 API | Core 修改题目或反馈状态 |
| 美食投稿、榜单、轮次、校准票、调档 | Food Service | HMAC 内部 API / 事件摘要 | Core 保存美食正文或票明细 |
| 系统状态与部署信息 | 各 deploy unit / Infra | 统一健康与内部摘要接口 | 前端直接访问内部探针 |

## 4. 指标口径

所有指标返回：

```json
{
  "value": 100,
  "previous_value": 90,
  "change_rate": 0.1111,
  "definition_version": "metric_v1",
  "as_of": "2026-07-18T08:00:00Z"
}
```

### 4.1 用户

- `registered_users`：未物理删除的 Core 用户。
- `verified_users`：至少一个 verified email identity。
- `dau`：当天至少一次成功受保护业务操作的去重用户；PV 不计入。
- `academic_profile_completion_rate`：完成必填学业档案用户 / verified users。
- `subscription_rate`：指定组织内启用至少一个通知订阅的用户 / 完成组织档案用户。

### 4.2 邮件

- `accepted_rate`：accepted / 已结束 SMTP 尝试。
- `delivered_rate`：仅在有明确 DSN/回调时计算 delivered / 终态。
- `hard_bounce_rate`、`soft_bounce_rate` 分开。
- queued、sending、retry_due 不进入结果率分母。
- Critical、Transactional、Digest 不得混算。

### 4.3 反馈

- `first_response_time`：首次管理员有效操作 - created_at。
- `resolution_time`：resolved_at - created_at。
- `urgent` 创建后 24 小时到期；`normal` 创建后 72 小时到期。
- `overdue`：未解决且当前时间超过 due_at。
- 平台反馈和题目反馈分别计算，再在总览汇总。

### 4.4 美食

- `calibration_participants`：当前轮次有效投票用户数。
- `position_consensus`：三类校准票的分布和 Policy 结论，不转换为五星分。
- `initial_tier_retention_rate`：当前档位仍等于初始档位条目 / 已发布条目。
- `promotion_candidate_count`、`demotion_candidate_count`：按 Policy 版本计算。
- suspected/invalidated 票不进入有效共识分母。
- Policy V1：至少 10 名有效参与者、70% 候选阈值、调档后 7 天冷却。

## 5. 浏览器 Admin API

### 5.1 总览

```http
GET /api/v1/admin/dashboard-snapshots/latest
GET /api/v1/admin/action-items
GET /api/v1/admin/metric-series
```

通用过滤：`from`、`to`、`timezone`、`organization_id`、`product_code`、`environment`。

`dashboard-snapshots/latest` 固定返回用户、校园通知、邮件、反馈、美食、系统六张卡并允许部分成功；未接入域使用 `not_integrated` 和 `value: null`：

```json
{
  "data": {
    "status": "partial",
    "as_of": "2026-07-18T08:00:00Z",
    "users": {},
    "notices": {},
    "mail": {},
    "feedback": {},
    "food": {},
    "system": {},
    "partial_failures": [
      {
        "service_id": "notice",
        "code": "SUMMARY_STALE",
        "last_success_at": "2026-07-18T07:45:00Z"
      }
    ]
  },
  "request_id": "req_01J..."
}
```

认证/授权失败或 BFF 自身不可用才返回整体失败。

### 5.2 用户与受众

```http
GET   /api/v1/admin/users
GET   /api/v1/admin/users/{user_id}
PATCH /api/v1/admin/users/{user_id}
POST  /api/v1/admin/users/{user_id}/session-revocations

GET   /api/v1/admin/users/{user_id}/academic-profiles/current
PATCH /api/v1/admin/users/{user_id}/academic-profiles/current

GET   /api/v1/admin/organizations
GET   /api/v1/admin/audience-summaries
GET   /api/v1/admin/notification-subscriptions
```

用户端：

```http
GET   /api/v1/users/me/academic-profiles/current
PATCH /api/v1/users/me/academic-profiles/current
GET   /api/v1/notification-subscriptions
POST  /api/v1/notification-subscriptions
PATCH /api/v1/notification-subscriptions/{subscription_id}
DELETE /api/v1/notification-subscriptions/{subscription_id}
```

### 5.3 校园通知

V1 仅人工表单与 UTF-8 `campus-notice-import/1.0` JSONL 导入：

```http
POST /api/v1/admin/school-notices
POST /api/v1/admin/notice-import-jobs
GET  /api/v1/admin/notice-import-jobs/{job_id}
GET  /api/v1/admin/school-notices
GET  /api/v1/admin/school-notices/{notice_id}/versions
POST /api/v1/admin/school-notices/{notice_id}/approvals
POST /api/v1/admin/school-notices/{notice_id}/rejections
POST /api/v1/admin/school-notices/{notice_id}/distributions
POST /api/v1/object-upload-intents
```

每任务最多 1,000 条或 10 MB并逐行返回结果。表单与 JSONL 共用 Upsert；相同 Hash 幂等，内容变化建立不可变新版本。附件通过预签名 URL 上传 S3 兼容存储，浏览器不得获得 Secret。自动抓取、网页解析、QQ 空间同步与 OCR 不进入 V1。

### 5.4 邮件

```http
GET  /api/v1/admin/mail-summaries/current
GET  /api/v1/admin/mail-queues
GET  /api/v1/admin/mail-deliveries
GET  /api/v1/admin/mail-deliveries/{delivery_id}
POST /api/v1/admin/mail-deliveries/{delivery_id}/retry-jobs

GET  /api/v1/admin/dead-letters
POST /api/v1/admin/dead-letters/{dead_letter_id}/replay-jobs
POST /api/v1/admin/dead-letters/{dead_letter_id}/resolutions

GET    /api/v1/admin/mail-suppressions
POST   /api/v1/admin/mail-suppressions
DELETE /api/v1/admin/mail-suppressions/{suppression_id}

GET /api/v1/admin/mail-domain-health-checks/latest
```

### 5.5 反馈

```http
POST /api/v1/platform-feedback

GET   /api/v1/admin/platform-feedback
GET   /api/v1/admin/platform-feedback/{feedback_id}
PATCH /api/v1/admin/platform-feedback/{feedback_id}

POST /api/v1/quiz/questions/{question_id}/feedback

GET   /api/v1/admin/question-feedback
GET   /api/v1/admin/question-feedback/{feedback_id}
PATCH /api/v1/admin/question-feedback/{feedback_id}

GET  /api/v1/admin/operation-cases
POST /api/v1/admin/operation-cases/{case_id}/assignments
```

### 5.6 美食

用户端：

```http
POST /api/v1/food-submissions
GET  /api/v1/food-entries
GET  /api/v1/food-entries/{entry_id}
GET  /api/v1/food-entries/{entry_id}/calibration-rounds/current
PUT  /api/v1/food-entries/{entry_id}/calibration-votes/me
```

管理员：

```http
GET /api/v1/admin/food-summaries/current

GET  /api/v1/admin/food-submissions
GET  /api/v1/admin/food-submissions/{submission_id}
POST /api/v1/admin/food-submissions/{submission_id}/approvals
POST /api/v1/admin/food-submissions/{submission_id}/rejections
POST /api/v1/admin/food-submissions/{submission_id}/revision-requests

GET   /api/v1/admin/food-entries
GET   /api/v1/admin/food-entries/{entry_id}
PATCH /api/v1/admin/food-entries/{entry_id}
GET   /api/v1/admin/food-entries/{entry_id}/calibration-rounds

GET  /api/v1/admin/food-adjustment-candidates
POST /api/v1/admin/food-entries/{entry_id}/tier-adjustments

GET  /api/v1/admin/food-vote-anomalies
GET  /api/v1/admin/food-vote-anomalies/{anomaly_id}
POST /api/v1/admin/food-vote-anomalies/{anomaly_id}/invalidations
POST /api/v1/admin/food-vote-anomalies/{anomaly_id}/restorations

GET /api/v1/admin/food-tier-adjustments
```

调档请求：

```json
{
  "expected_version": 7,
  "expected_round_id": "uuid",
  "action": "promote_one_tier",
  "policy_version": "food_calibration_v1",
  "reason": "有效校准结果达到当前规则门槛"
}
```

### 5.7 Study、Quiz 与系统

```http
GET /api/v1/admin/study-summaries/current
GET /api/v1/admin/quiz-summaries/current

GET /api/v1/admin/system-overviews/current
GET /api/v1/admin/services
GET /api/v1/admin/workers
GET /api/v1/admin/deployments
GET /api/v1/admin/data-jobs

GET /api/v1/admin/audit-logs
GET /api/v1/admin/audit-logs/{audit_log_id}
```

## 6. 内部服务摘要接口

各业务服务提供：

```http
GET /api/v1/internal/admin-summaries/current
GET /api/v1/internal/action-items
```

响应只包含聚合和资源引用，不返回完整敏感正文。

Notice/Food/Quiz/Study 的业务详情仍通过对应 Adapter 或内部资源接口读取；Platform Core 不跨库查询。

## 7. 标准事件

事件 Envelope：

```json
{
  "event_id": "uuid",
  "event_type": "food.tier.changed",
  "schema_version": 1,
  "occurred_at": "2026-07-18T08:00:00Z",
  "producer": {
    "service_id": "food"
  },
  "subject_user_id": null,
  "resource": {
    "type": "food_entry",
    "id": "uuid"
  },
  "data": {},
  "request_id": "req_01J..."
}
```

建议事件：

```text
platform.feedback.created
platform.feedback.resolved
quiz.question-feedback.created
quiz.question-feedback.resolved
food.submission.created
food.submission.approved
food.submission.rejected
food.calibration-vote.cast
food.calibration-round.closed
food.tier.changed
school-notice.reviewed
school-notice.distribution.started
school-notice.distribution.completed
mail.delivery.failed
mail.delivery.dead-lettered
```

事件只携带最小数据和引用；完整邮箱、验证码、题目正文、通知正文和美食正文不得进入平台事件。

## 8. 权限与 Scope

访问上下文增加 `organization` Scope，用于学院/校区通知运营和美食审核。

权限码：

```text
users.view
users.manage
users.email.reveal
roles.manage
memberships.manage
notices.view
notices.review
notices.distribute
notice-sources.manage
mail.view
mail.retry
dead-letters.replay
platform-feedback.manage
question-feedback.review
food-submissions.review
food-entries.manage
food-tiers.adjust
food-votes.moderate
system.view
audit-logs.view
```

浏览器自报的 user_id、role、membership、entitlements 和 organization_scope 一律忽略。

## 9. 旧系统兼容

- 旧 Study `/api/v1/admin/*` 保持，Platform Core BFF/Adapter 转为新 Envelope 和 snake_case。
- QuizCraft 现有 FastAPI 反馈接口保持，新增 Adapter 后逐页切换。
- 旧接口加入调用指标，切流前不得删除未知调用。
- 废弃接口返回 `Deprecation`、`Sunset` Header，至少保留两个发布周期。

## 10. API Review Gate

每个接口合并前必须确认：

- OpenAPI 3.1 已更新并通过 Lint；
- 身份来自 Session 或服务凭据；
- 幂等、事务、唯一约束或行锁已定义；
- 查询参数和排序白名单已文档化；
- 正常、失败、重复、并发、越权和依赖故障有测试；
- request_id 全链路；
- 敏感字段与第三方响应脱敏；
- 兼容、Feature Flag、监控和回滚明确。

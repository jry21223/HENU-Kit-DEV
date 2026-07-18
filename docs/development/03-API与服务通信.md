# API 与服务通信规范

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 契约优先

- 新 Platform Core 的唯一契约来源为 `packages/api-contracts` 中的 OpenAPI 3.1 文件。
- Event Schema 放在 `packages/event-schemas`；代码生成物不得手工改。
- 接口实现、前端调用、Mock 和契约测试必须消费同一版本。
- 旧 Study/FastAPI 接口保持兼容；每次只迁移一个 Endpoint，禁止一次性改完整个旧前端。

## 2. URL、字段与时间

- 公共前缀：`/api/v1`。
- 内部服务接口：`/api/v1/internal`，使用独立鉴权中间件。
- 资源名使用复数名词和 `kebab-case` 路径；JSON 字段统一 `snake_case`。
- 时间统一 UTC RFC 3339，例如 `2026-07-17T08:00:00Z`。
- 新公开 ID 使用 UUIDv4；连续自增 ID 和旧系统 ID 不得暴露给浏览器。
- 旧 ID 只能出现在 Adapter 或内部 Payload，并标注来源系统。

## 3. 请求追踪

- 每个请求生成 `request_id`，响应头写 `X-Request-Id`，响应体也返回 `request_id`。
- 若上游传入合法 `X-Request-Id`，跨服务透传；非法或过长值重新生成。
- 日志、事件、Outbox、邮件投递和下游调用均携带同一 `request_id`。

## 4. 响应格式

成功：

```json
{
  "data": {},
  "request_id": "req_01J..."
}
```

分页：

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

失败：

```json
{
  "error": {
    "code": "VERIFICATION_CODE_EXPIRED",
    "message": "验证码已过期，请重新获取",
    "details": null
  },
  "request_id": "req_01J..."
}
```

`message` 面向用户，`code` 供客户端稳定判断；不得把 SQL、堆栈、供应商响应或敏感参数返回给浏览器。

## 5. HTTP 与错误码

| 场景 | HTTP | 最小错误码 |
|---|---:|---|
| 参数或 Schema 不合法 | 400/422 | `VALIDATION_ERROR` |
| 未登录或 Session 失效 | 401 | `UNAUTHENTICATED`、`SESSION_EXPIRED` |
| 无权限 | 403；避免枚举时 404 | `FORBIDDEN` |
| 资源不存在 | 404 | `NOT_FOUND` |
| 幂等键与原请求冲突 | 409 | `IDEMPOTENCY_KEY_CONFLICT` |
| 账户已绑定他人 | 409 | `ACCOUNT_ALREADY_LINKED` |
| 限流 | 429 | `RATE_LIMITED` |
| 验证码过期/已用 | 400 | `VERIFICATION_CODE_EXPIRED`、`VERIFICATION_CODE_ALREADY_USED` |
| PKCE/授权码失败 | 400 | `PKCE_FAILED`、`AUTH_CODE_ALREADY_USED` |
| 服务签名或重放 | 401/409 | `SERVICE_SIGNATURE_INVALID`、`REPLAY_DETECTED` |
| PostgreSQL/Redis 等依赖不可用 | 503 | `DEPENDENCY_UNAVAILABLE` |

## 6. 幂等规范

- 所有产生副作用的 `POST`/`PATCH` 必须接受 `Idempotency-Key`。
- 唯一范围：`client_id + method + normalized_route + key`。
- 默认保存 24 小时；支付、积分、账户绑定和数据迁移应按业务保留更久。
- 相同 Key、相同请求返回首次结果；相同 Key、不同请求体返回 409。
- 幂等不能只依赖 Redis；最终一致性必须由数据库唯一约束保证。

## 7. 分页、过滤与排序

- 默认 `page=1`、`page_size=20`；最大值由 OpenAPI 明确，不能由客户端任意扩大。
- 公开列表使用 `page/page_size`；只有高吞吐内部接口可使用 Cursor。
- 排序字段采用白名单，禁止把客户端字段直接拼接到 SQL。
- 过滤条件必须在 OpenAPI 中定义，禁止未文档化 Query 参数。

## 8. 浏览器登录与跨主域通信

- 统一账户域使用 HttpOnly、Secure、SameSite=Lax 的 Core Session Cookie。
- 各业务站建立自己的本地 Session；不同主域不得共享 Cookie。
- 业务站登录流程：`state + PKCE -> exact callback -> single-use authorization code -> server-side token exchange -> local session`。
- `return_to` 只允许站内路径，由业务站后端存入 Redis State；Core 只重定向到预登记 Callback。
- 长期 Token 不得进入 URL、浏览器 LocalStorage 或前端日志。

## 9. 服务间 HMAC

请求头：

```text
X-Service-Id
X-Key-Id
X-Timestamp
X-Nonce
X-Signature
```

默认 Canonical String：

```text
METHOD
PATH_AND_QUERY
X-Timestamp
X-Nonce
SHA256(BODY)
```

- 算法：HMAC-SHA256。
- 时间容差：5 分钟。
- Nonce 在 Redis 原子去重；Redis 不可用时高风险接口 Fail Closed。
- Verifier 在轮换宽限期同时接受 `active` 和 `retiring` Key。
- 日志只记录 `service_id`、`key_id` 和结果，不记录 Secret、签名原文或完整 Body。

## 10. 标准事件 Envelope

```json
{
  "event_id": "uuid",
  "event_type": "quiz.answer.submitted",
  "schema_version": 1,
  "occurred_at": "2026-07-17T08:00:00Z",
  "producer": {"service_id": "quizcraft"},
  "subject_user_id": "uuid-or-null",
  "resource": {"type": "practice_answer", "id": "uuid"},
  "data": {},
  "request_id": "req_01J..."
}
```

- `(service_id, event_id)` 唯一。
- Payload 只携带业务引用和最小字段，不放验证码、Token、Cookie、完整邮箱、资料正文或答案正文。
- 生产方业务事务与 Outbox 同事务提交；消费方使用唯一键实现幂等。
- 事件处理失败进入重试或 DLQ，不能静默丢弃。

## 11. 接口清单

### 健康检查

| Method | URL | 鉴权 | 阶段 |
|---|---|---|---|
| GET | `/api/v1/healthz` | 公开最小信息 | P1 |
| GET | `/api/v1/readyz` | 公开最小信息 | P1 |

### 账号与会话

| Method | URL | 用途 | 鉴权 |
|---|---|---|---|
| POST | `/api/v1/auth/email-codes` | 请求验证码 | 公开 + 限流 |
| POST | `/api/v1/auth/email-codes/verify` | 校验并建立 Core Session | 公开 + 尝试限流 |
| GET | `/api/v1/oauth/authorize` | 校验 Client/Callback/State/PKCE | Core Session 可选 |
| POST | `/api/v1/oauth/token` | 授权码交换身份 | Confidential Client + PKCE + HMAC |
| POST | `/api/v1/oauth/logout` | 撤销当前 Core Session | 用户 Session |
| POST | `/api/v1/sessions/revoke` | 撤销指定或全部 Session | 用户 + 安全验证 |
| GET | `/api/v1/users/me` | 返回最小身份、角色、会员和授权上下文 | 用户 Session |

### 通知与偏好

| Method | URL | 用途 | 鉴权 |
|---|---|---|---|
| GET | `/api/v1/notifications` | 当前用户通知分页 | 用户 Session |
| POST | `/api/v1/notifications/{id}/read` | 幂等已读 | 用户 Session |
| POST | `/api/v1/notifications/read-all` | 全部已读 | 用户 Session |
| GET | `/api/v1/notification-preferences` | 查询偏好 | 用户 Session |
| PATCH | `/api/v1/notification-preferences` | 更新订阅/退订 | 用户 Session + 幂等 |

### 内部服务接口

| Method | URL | 用途 | 鉴权 |
|---|---|---|---|
| POST | `/api/v1/internal/events` | 接收标准事件 | HMAC + 幂等 |
| POST | `/api/v1/internal/activity` | 上报最小活跃事件 | HMAC |
| POST | `/api/v1/internal/account-links/resolve` | 查询/创建外部账户映射 | HMAC |
| POST | `/api/v1/internal/sessions/introspect` | 检查 Core Session/撤销状态 | HMAC |
| POST | `/api/v1/internal/service-credentials/{service_id}/rotate` | 生成 Next Key | Super Admin + 二次验证 |
| POST | `/api/v1/internal/service-credentials/{service_id}/activate` | 激活新 Key、旧 Key 进入 Retiring | Super Admin |
| GET | `/api/v1/internal/metrics/users` | 用户数和活跃量 | 服务或管理权限 |

### Study 与 Quiz Adapter

| Method | URL | 用途 | 鉴权 |
|---|---|---|---|
| GET | `/api/v1/courses/{course_id}/quiz-target` | 课程到题库显式映射 | 公开/可选用户 |
| GET | `/api/v1/quiz/banks` | Adapter 题库列表 | 公开 |
| POST | `/api/v1/quiz/practice/start` | 开始练习 | 可选本地 Session |
| POST | `/api/v1/quiz/practice/submit` | 提交作答；用户来自服务端 Session | Quiz 本地 Session |
| GET | `/api/v1/quiz/ranking` | 排行榜 | 公开 |
| POST | `/api/v1/quiz/account-links` | 匿名用户绑定统一用户 | 登录 + 签名匿名 Session |
| POST | `/api/v1/library/account-links` | 旧资料库用户绑定 | 登录 + 服务端验证 |

校园通知接口属于可选 Epic，不进入首发 API 门禁。

## 12. 核心原子性要求

| 流程 | 不可省略的保证 |
|---|---|
| 请求验证码 | Code Hash、Event、Outbox 同事务；明文只存在内存和最小邮件 Job |
| 验证验证码 | `FOR UPDATE`/条件更新；`used`、User、Identity、Session 同事务 |
| 交换授权码 | Code 行锁并条件消费；同一 Code 仅一个请求成功 |
| 退出登录 | 先撤销本地 Session，再异步补偿 Core；安全事件可撤销全部设备 |
| Account Link | 唯一 `(service_id, external_user_id)`；已绑定他人不得覆盖 |
| 作答提交 | Append-only Answer、统计和 Outbox 同事务；唯一提交键防重复 |
| 通知已读 | 只按当前用户查询和更新；不得信任请求体 `user_id` |

## 13. 兼容与废弃

- 旧 Study `/api/v1` 和旧响应格式暂时保留，由 BFF/Adapter 转换。
- Quiz Adapter 对外使用新 Envelope，内部代理 FastAPI `/api/*`。
- 废弃接口先返回 `Deprecation` 和 `Sunset` Header，至少保留两个发布周期。
- 切流前统计旧客户端调用量；未知调用不为零时不得删除。

## 14. API Review Checklist

- OpenAPI 已更新且 Lint 通过。
- 是否需要幂等键、事务、唯一约束或行锁。
- 是否从 Session/服务凭据获取身份，而不是客户端自报。
- 正常、失败、重复、并发、越权和依赖故障均有测试。
- 响应、日志和事件均有 `request_id`。
- 敏感字段、错误详情和第三方响应均已脱敏。
- 兼容期、Feature Flag、监控和回滚方式明确。

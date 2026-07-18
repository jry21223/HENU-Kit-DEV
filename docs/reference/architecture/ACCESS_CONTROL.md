# HENU Kit 身份、角色与会员模型

> 目标仓库：`HENUKitDev`  
> 原则：主体类型、权限角色、会员权益、资源授权和账户状态分开建模。

## 1. 四个维度

### 主体类型

- `guest`：未登录游客，不写入统一 `users` 表。
- `user`：已登录的人类用户。
- `service`：经过服务间认证的机器主体。

### 权限角色

一个用户可以同时拥有多个、可限定作用域的角色：

- `student`：学生基础权限。
- `creator`：创建资料或题库草稿。
- `reviewer`：审核指定模块、课程或题库内容。
- `operator`：内容运营、客服和运营数据查看。
- `admin`：业务配置和管理操作。
- `super_admin`：平台级关键配置和角色授权。

现有 `users.role=user` 在迁移时映射为 `student`。兼容期不直接删除旧字段。

### 会员档位

第一阶段只启用：

- `free`：普通学生。
- `vip`：VIP 学生。

VIP 是内容、额度和增强功能的权益档位，不是管理角色。VIP 不自动获得 creator、reviewer、operator 或 admin 权限。

### 权益

具体能力使用稳定的 entitlement code，例如：

```text
materials.download.login_required
materials.download.vip
quiz.progress.persist
quiz.analytics.advanced
quiz.bank.manage
submission.create
submission.review
ai.quota.standard
ai.quota.vip
```

有效权限由账户状态、主体类型、角色和作用域、会员档位、显式授权、资源所有权和资源状态共同计算。前端只能展示结果，后端必须重新判断。

## 2. 首期用户档位

| 类型 | 登录 | 核心能力 |
|---|---:|---|
| 游客 | 否 | 浏览主站和公开内容、受限试刷、提交受限匿名反馈 |
| 学生 Free | 是 | 保存进度、基础资料、基础刷题、投稿、通知和个人数据 |
| VIP 学生 | 是 | Free 全部能力，以及受控高级资料、增强统计和更高额度 |
| Creator | 是 | 在授权范围创建和维护草稿 |
| Reviewer | 是 | 在授权范围审核内容 |
| Operator | 是 | 内容运营和支持，不修改平台级角色 |
| Admin | 是 | 管理指定业务模块 |
| Super Admin | 是 | 平台级关键配置，高风险操作必须完整审计 |

一个人可以同时是 VIP 学生和 Reviewer。会员到期只影响会员权益，不影响学生角色或审核角色。

## 3. 游客与匿名会话

- 业务站生成不可猜测的 `anonymous_subject_id`。
- 使用该业务站自己的 HttpOnly、Secure Cookie，不跨主域共享。
- 不把匿名 ID 放入 URL。
- 不信任请求体自报的 `user_id`。
- 匿名数据设置保留期限，默认建议最后活动后 90 天，最终期限由数据治理评审确认。

QuizCraft 登录绑定时，服务端验证匿名会话持有证明，在事务中创建唯一 account link，并迁移练习会话和作答归属。匿名主体已被其他用户绑定时必须拒绝，不覆盖历史数据。

## 4. 目标表

### `user_roles`

字段至少包括：`user_id`、`role_code`、`scope_type`、`scope_id`、`status`、`assigned_by`、`assigned_at`、`revoked_at`。

有效的 `(user_id, role_code, scope_type, scope_id)` 必须唯一。

### `memberships`

字段至少包括：`user_id`、`tier_code`、`status`、`starts_at`、`expires_at`、`source`、`reference_id`、`idempotency_key`。

### `entitlement_grants`

用于活动、课程包、人工补偿等显式授权，不代替 membership。

### `account_links`

保存平台用户与 Study 旧用户、QuizCraft 用户或匿名主体之间的唯一绑定。

## 5. 现有 Study 迁移

1. 新建 `user_roles` 和 `memberships`，保留旧 `users.role` 和会员数据。
2. `user` 回填为 `student`；其他旧角色按原值回填，并补学生基础角色。
3. 当前有效会员回填为 `vip`，保留来源和到期时间。
4. 兼容期优先读新表，缺失时回退旧字段。
5. 新授权只写新表，旧字段进入只读。
6. 完成对账和回滚窗口后，再独立执行 contract migration。

禁止根据积分余额推断 VIP，禁止根据 VIP 推断管理角色。

## 6. API 身份响应

至少返回：

```json
{
  "user_id": "usr_xxx",
  "actor_type": "user",
  "email_verified": true,
  "roles": [
    {"code": "student", "scope_type": "platform", "scope_id": null}
  ],
  "membership": {
    "tier": "vip",
    "status": "active",
    "expires_at": "2026-12-31T16:00:00Z"
  },
  "entitlements": ["quiz.progress.persist", "quiz.analytics.advanced"]
}
```

业务站建立本地会话时只保存必要身份。权限敏感操作必须按短 TTL、权限版本或平台查询刷新。

## 7. 测试门槛

- 游客公开访问和受保护资源拒绝。
- 匿名 ID 伪造、重放、过期和绑定冲突。
- Free 与 VIP 权益差异。
- VIP 到期、撤销和重复事件幂等。
- 多角色叠加和作用域隔离。
- Reviewer 不得越权审核其他范围。
- Operator 不得修改平台级角色。
- 浏览器篡改 role、membership 或 entitlement 无效。
- 旧角色字段到新角色表的双读和回滚。

## 8. 首期不做

- 不上线超过 free、vip 的复杂档位。
- 不把 VIP 当作权限角色。
- 不把游客自动注册成用户。
- 不允许业务模块直接修改平台角色或会员数据。
- 不在浏览器长期保存完整权限集合。

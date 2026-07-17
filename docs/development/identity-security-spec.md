# 身份、权限与安全规范

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 访问模型

平台访问状态由四层独立信息组成：

1. `actor_type`：`guest`、`user`、`service`。
2. `roles`：`student`、`creator`、`reviewer`、`operator`、`admin`、`super_admin`，允许多角色与作用域。
3. `membership_tier`：首期只有 `free`、`vip`。
4. `entitlements`：具体资源和能力授权，例如 `materials.download.vip`、`quiz.analytics.advanced`。

VIP 是会员权益，不是角色；Admin 不自动获得 VIP 内容；浏览器自报的 Role、Tier、Entitlement 和 User ID 全部忽略。

## 2. 能力矩阵

| 主体 | 可以 | 不可以 |
|---|---|---|
| Guest | 浏览公开入口、受限试刷、受限匿名反馈 | 跨设备进度、受保护资料、通知、管理权限 |
| Free Student | 保存进度、基础资料/刷题、投稿、通知和个人数据 | VIP 内容、审核和管理 |
| VIP Student | Free 全部 + 高级资料、增强统计和更高额度 | 自动获得 Creator/Reviewer/Admin |
| Creator | 在授权课程/题库创建和维护草稿 | 审核、角色管理 |
| Reviewer | 在授权 Scope 审核内容 | 越范围审核、平台角色管理 |
| Operator | 内容运营、客服、通知运营和受控数据查看 | Super Admin 操作 |
| Admin/Super Admin | 业务管理或关键配置 | 自动获得 VIP 权益 |

Scope 首期支持：`platform`、`product`、`course`、`bank`。权限检查必须同时验证 Role、Scope、资源状态和用户状态。

## 3. Guest 与匿名会话

- Guest 不写入 Core `users`。
- 业务站生成不可猜测 `anonymous_subject_id`，保存在自身 HttpOnly、Secure Cookie。
- 匿名 ID 不进入 URL、不跨主域共享、不接受 Body 自报。
- 登录绑定需要持有证明和唯一 Account Link；已绑定他人返回冲突，不覆盖。
- 建议匿名数据保留 90 天，最终由数据治理决策确认。

## 4. 验证码安全

- 校验允许的学生邮箱域名；真实域名范围必须在上线前确认。
- Code 使用安全随机数生成；数据库只存 `HMAC(code + nonce)`。
- 邮箱、IP、设备多维限流；Redis 不可用时高风险请求 Fail Closed。
- 请求接口统一返回 202，不暴露账号是否存在。
- 校验和 `used` 标记必须是同一事务的条件更新。
- 错误尝试达到阈值后锁定或撤销当前 Code。
- 验证码不得进入响应、日志、错误堆栈、指标 Label、数据库正文或 DirectMail 投递记录正文。

## 5. Authorization Code Flow

- Callback 使用精确白名单，不允许通配或任意 `return_to`。
- State 存于业务站 Redis，并使用原子 `GETDEL` 防重放。
- PKCE 只接受 S256。
- Authorization Code 只在 URL 中出现一次，数据库只存 Hash，60–120 秒过期。
- Token Exchange 对 Code 行加锁并条件消费；并发只允许一个成功。
- 业务站使用返回的最小身份建立本地 Session；长期 Token 不发给浏览器。

## 6. Session 与 Cookie

- Cookie：`HttpOnly; Secure; SameSite=Lax`；按域设置最小 Path 和有效期。
- Core Session 与各业务站本地 Session 分离。
- Session Token 只存 Hash；IP/UA 限长并脱敏。
- 普通退出先撤销本地 Session，再撤销对应 Core Session。
- 安全事件、密码/身份风险或用户选择“退出所有设备”时撤销全部 Session。
- Session 撤销事件应在目标 SLO 内传播到所有业务站；默认目标 5 分钟。

## 7. 服务凭据

- Secret 只在创建时展示一次；数据库使用 AES-GCM，主密钥放在外部 Secret 管理或部署环境。
- 日志只记录 Fingerprint、Service ID、Key ID 和结果。
- 轮换流程：`pending -> proven -> active`；旧 Key：`active -> retiring -> revoked`。
- 宽限期内同时接受新旧 Key；监控确认业务已切换后才撤销旧 Key。
- Nonce 防重放和时间容差必须启用，禁止因为 Redis 故障跳过验证。

## 8. 服务端授权规则

- 所有资源访问从当前 Session 获取用户，不能从 Query/Body 接受 `user_id` 作为权限依据。
- 列表查询必须自动附加当前用户或授权 Scope 过滤。
- 越权访问可返回 404 以避免资源枚举，但必须记录脱敏安全审计。
- 前端隐藏按钮只是体验，不是权限控制。
- VIP、积分余额、显示名、客户端缓存和旧 Role 字段均不得单独作为管理权限证据。

## 9. 日志与隐私

禁止记录：

- 完整邮箱、验证码、Authorization、Cookie、Session Token。
- Client Secret、服务签名、DirectMail 内容和支付原始回调。
- 完整答案、资料正文、匿名持有证明或未脱敏第三方响应。

必须记录：

- `request_id`、结果、错误码、服务/Key ID、脱敏用户 ID、资源类型、耗时和重试次数。
- 安全拒绝原因使用分类码，不记录原始敏感输入。
- 日志使用结构化 JSON；生产日志必须可扫描敏感模式。

## 10. Secret 与仓库安全

- 真实 `.env`、数据库密码、JWT/Key/Token、服务器地址、备份和生产数据不得提交。
- 仓库、PR 和镜像必须运行 Secret Scan 和漏洞扫描。
- `.env.example` 只包含键名、说明和安全占位符。
- 导入外部仓库或代码前验证 License、大文件和提交历史中的 Secret。

## 11. 安全测试最低集合

- 同一验证码 20 并发，仅一次成功。
- State 和 Authorization Code 重放阻断。
- 非法 Callback/Return To 不重定向。
- 错误 PKCE 不消费或按策略撤销 Code。
- 浏览器篡改 Role/Tier/User ID 无效。
- Guest 访问受保护资源被拒绝且不创建垃圾用户。
- Free/VIP 与 Reviewer/Admin 权限相互独立。
- HMAC Nonce 重放、过期时间戳和错误 Key 被拒绝。
- A 用户不能读取或已读 B 用户通知。
- 日志扫描不出现邮箱、Code、Token、Cookie 或 Secret。

发现任一敏感日志、身份绕过、重复积分/作答或账号覆盖时，立即阻断发布。

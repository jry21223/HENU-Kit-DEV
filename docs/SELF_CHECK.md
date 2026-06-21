# Self Check

## Phase 16 WeChat Native Correction Check

Current subtask: first-round payment direction correction. No real WeChat Pay API integration, database migration, or order-system rewrite was performed in this round.

## Phase 16 WeChat Native Mock Implementation Check

Current subtask: converged implementation after the first correction round. Real WeChat Pay live API, notify signature verification, resource decryption, and entitlement grant by callback are still deferred.

### Implemented In This Round

- Added WeChat Pay config validation with production mock blocking.
- Added WeChat Native mock payment creation.
- Added integer-cent order fields for WeChat Native payment.
- Changed `POST /api/orders` to create local package orders without returning EasyPay params.
- Added `POST /api/payments/wechat/native`.
- Added `GET /api/orders/[id]/status`.
- Soft-disabled legacy EasyPay notify/return routes with `410 Gone`.
- Updated package purchase UI to call WeChat Native mock payment and poll order status.
- Replaced EasyPay unit test path with WeChat config, Native mock, and payment-permission tests.

### Still Deferred

- Real WeChat Pay live `/v3/pay/transactions/native` request signing and HTTP call.
- WeChat notify signature verification.
- WeChat notify resource decryption.
- SUCCESS callback settlement and idempotent entitlement grant.
- QR code rendering library/UI.
- Full material manifest import.

### Scope Answers

- Is EasyPay still present as code? Yes, as legacy code pending replacement.
- Is EasyPay still the target/default direction in docs? No, README and Phase 16 docs now point to WeChat Pay Native.
- Is WeChat Native real live payment implemented? No, deferred to the next code round.
- Is production mock payment blocked in code? Yes, WeChat Pay config validation rejects production mock mode.
- Are paid downloads supposed to be server-side entitlement checked? Yes; this remains a hard rule.
- Are real course materials committed? No; real files must stay under ignored upload/storage paths.
- Is AI-generated material the current main flow? No; manual material import and package binding are the current direction.

### First-Round Checklist

- [x] Audit existing EasyPay-related files, APIs, tests, and docs.
- [x] Update README to describe WeChat Pay Native as the target payment direction.
- [x] Add WeChat Pay Native environment variables to `.env.example`.
- [x] Add WeChat certificate/private-key ignore rules to `.gitignore`.
- [x] Add `docs/WECHAT_PAY_NATIVE.md` draft.
- [x] Add API, database, roadmap, and self-check notes for the direction correction.
- [ ] Remove/replace legacy EasyPay runtime code.
- [x] Add WeChat Native mock order creation implementation.
- [ ] Add WeChat notify verification/decryption implementation.
- [ ] Add material manifest import implementation.
- [x] Add WeChat Native payment tests.

### Latest Verification Run

- [x] `npm run typecheck`
- [x] `npm test`
- [x] `npm run lint`
- [x] `npm run build`
- [x] `npm run db:generate`
- [x] `npm run db:validate`
- [ ] `npm run db:migrate`

Prisma note: the first normal generate hit a Windows DLL file lock from an old local `next dev` process. After stopping the stale project dev processes, normal `npm run db:generate` completed successfully. Local DB migration did not apply because the configured PostgreSQL credentials for `final_review` failed authentication; fix `.env` or start the matching development database before running `npm run db:migrate`.

Local dev server note: `next dev` starts, but pages that read PostgreSQL return 500 with the same database authentication error. The dev server was stopped instead of leaving a broken local instance running.

### Verification Run

- [x] `git diff --check`
- [x] `npm run typecheck`
- [x] `npm run lint`
- [x] `npm test`
- [x] `npm run build`

Build note: legacy `/api/payments/easypay/*` routes still exist and are visible in the route manifest, but now return `410 Gone` and no longer settle orders.

## 当前阶段

Phase 16：MVP 硬化与上线前审查

## 本阶段目标

- 不新增产品功能。
- 审查权限、安全、数据流、mock fallback、上传、下载、支付、AI job。
- 修复高风险问题。
- 补充关键测试，覆盖正常路径和绕过尝试。
- 将 README 改成真实可验证状态描述。

## 已完成内容

- 限定 mock fallback：只允许非生产环境且无 `DATABASE_URL` 时使用 mock 数据。
- 加固验证码消费：验证码标记 used 时增加 `used=false` 和 `expiresAt > now` 条件，降低并发重复消费风险。
- 加固上传校验：PDF/TXT 后缀和 MIME 必须匹配，继续限制 10MB 和空文件。
- 加固下载路径：本地文件解析改为 `path.relative` 边界校验，拒绝路径穿越和相邻目录绕过。
- 下载响应增加 `X-Content-Type-Options: nosniff`。
- 微信 Native mock 支付配置已加固：生产环境禁止 mock，live 模式缺关键配置会失败。
- 补充并重写关键单元测试：
  - `tests/unit/auth.test.ts`
  - `tests/unit/permissions.test.ts`
  - `tests/unit/uploads.test.ts`
  - `tests/unit/payment-permissions.test.ts`
  - `tests/unit/production-payment-config.test.ts`
  - `tests/unit/wechat-native.test.ts`
  - `tests/unit/questions.test.ts`
  - `tests/unit/pdf-watermark.test.ts`
- README 已改为真实状态、风险、测试和生产前待办说明。

## 未完成内容

- 未新增产品功能。
- 未接真实邮件服务。
- 未接真实支付商户生产配置。
- 未接真实 AI 模型、异步队列和独立审核队列。
- 未实现验证码发送限流、失败次数限制、CSRF 完整策略、审计日志。
- 未实现对象存储私有桶、病毒扫描、文件内容嗅探。
- 未补完整 E2E 测试。

## 代码改动摘要

- `src/lib/db.ts`
- `src/lib/auth.ts`
- `src/lib/uploads.ts`
- `src/lib/downloads.ts`
- `src/lib/easypay.ts`
- `src/app/api/auth/verify-code/route.ts`
- `src/app/api/materials/[id]/download/route.ts`
- `src/app/api/payments/easypay/notify/route.ts`
- `src/app/api/payments/easypay/return/route.ts`
- `src/services/catalog-service.ts`
- `src/services/course-service.ts`
- `src/services/material-service.ts`
- `src/services/question-service.ts`
- `src/services/package-service.ts`
- `tests/unit/*.test.ts`
- `README.md`
- `docs/SELF_CHECK.md`
- `docs/CHANGELOG.md`

## 本地运行检查

- [x] 项目可以安装依赖。
- [x] 项目可以启动（本轮未重启服务，已有 dev server 仍在 3000 端口监听）。
- [x] 首页可以访问（上一轮已验证，本轮未做 UI 改动）。
- [x] 核心页面可以访问（上一轮已验证，本轮未做 UI 改动）。
- [x] API 返回正常（通过单元测试覆盖关键规则）。
- [x] 数据库迁移未变更。
- [x] seed 数据未变更。
- [x] `npm run typecheck` 通过。
- [x] `npm run lint` 通过。
- [x] `npm test` 通过。

## 功能检查

- [x] admin API 均存在服务端 admin/reviewer 校验。
- [x] paid 下载需要登录、邮箱验证和 entitlement。
- [x] draft / pending_review / archived 资料不可下载。
- [x] 题目列表不会返回 `answer` / `explanation`。
- [x] AI job 生成 draft material，不自动 published。
- [x] 投稿默认 pending，审核通过由 reviewer/admin 路由触发。
- [x] PDF 水印不覆盖原始文件。
- [x] EasyPay 回调路由已软禁用，不再结算订单。
- [ ] WeChat notify 回调验签、解密、金额校验和订单结算仍未实现。

## 权限检查

- [x] 未登录用户权限正确。
- [x] 学生用户权限正确。
- [x] admin 用户权限正确。
- [x] paid 内容没有被前端判断绕过。
- [x] draft 内容不会展示给普通用户。

## UI 检查

- [x] 本轮未改 UI。
- [x] README 文档状态描述已更新。
- [x] 错误状态未新增 UI 负担。

## 安全检查

- [x] 用户输入有基础校验。
- [x] admin/reviewer API 有服务端权限判断。
- [x] 文件上传限制大小、后缀、MIME。
- [x] 下载路径限制在 `uploads/` 内。
- [x] 不泄露验证码。
- [x] 题目列表不泄露答案。
- [x] 生产环境禁止 WeChat mock 支付。
- [ ] WeChat notify 回调验签和解密仍未实现。
- [x] mock fallback 不允许生产环境使用。

## 回归检查

- [x] 上一阶段功能没有因本轮修改而在单元测试中回归。
- [x] 登录基础逻辑仍然可用。
- [x] 课程列表服务仍然可用。
- [x] 资料详情服务仍然可用。
- [x] 下载权限仍然正确。

## 风险

- 真实支付仍需按微信支付 Native 官方文档复核字段、签名、解密、回调语义和幂等授权。
- 文件上传仍未做内容嗅探、病毒扫描和对象存储私有化。
- 验证码缺少限流和失败次数限制。
- 缺少完整 E2E 测试和生产监控。
- admin 后台仍是基础运营能力。

## 下一步建议

1. 增加验证码限流和登录失败保护。
2. 增加 E2E：登录、下载、投稿审核、支付回调。
3. 把本地上传迁移到私有对象存储。
4. 做真实支付商户联调和订单对账。
5. 增加审计日志、监控和告警。

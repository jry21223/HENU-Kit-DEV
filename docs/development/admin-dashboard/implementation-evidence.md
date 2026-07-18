# 统一管理后台 V1 实施与验收证据

更新日期：2026-07-18

## 已实现范围

- shadcn-vue / Tailwind CSS v4 新 Shell、固定六域卡片、ECharts 真实趋势、运行时 Feature Flag 与旧后台回退。
- Platform Core Admin BFF 快照、统一待办、指标序列、部分失败快照、HMAC 服务认证与请求幂等。
- 用户列表、角色与冻结状态维护、全 Session 撤销和 Token 版本失效。
- 邮件三队列、Fake/SMTP Adapter、投递尝试、accepted/delivered 分离、抑制、重试、DLQ 与重放。
- 校园通知表单/JSONL 共用 Upsert、不可变版本、版本对比、受众选择、审核、站内分发、邮件 opt-in 与即时退订阻断。
- 两档反馈 SLA（urgent 24h / normal 72h）、统一待办、QuizCraft 提交快照和 JSON/PostgreSQL/API 三方验证 Gate。
- 美食五档定义、投稿审核、每轮唯一票、10 人/70%/7 天策略、异常票剔除、阻断异常处理与调档历史。
- PostgreSQL/Redis/API/Worker/Outbox/DLQ/Migration/HTTP P95 系统观测与高风险操作审计。
- 0001–0013 版本化 SQL Migration；运行时 AutoMigrate 默认关闭。

## 自动化验证

```text
go test ./... (services/api)       PASS
go test ./... (services/worker)    PASS
python contract tests              2 PASS
Redocly OpenAPI 3.1 lint           VALID (1 license-metadata warning)
openapi-typescript generation      PASS
Vitest                             1 PASS
Vue production build              PASS
Playwright desktop + 390px mobile  5 PASS, 1 intentional project skip
```

Playwright 使用本机 Chrome channel，覆盖登录、六张固定业务卡、真实趋势图、用户/通知/邮件/反馈/美食/系统页面、键盘焦点和 390px 横向溢出检查。

## 数据库迁移验证

在独立临时数据库 `final_review_admin_v1_migration_check` 执行：

1. 空库 Up 到 0013；
2. 重复 Up；
3. Down 最新版本；
4. 再次 Up；
5. 校验 `schema_migrations`；
6. 删除临时数据库。

所有步骤通过，正式开发库当前最新版本为 `0013_notice_audience`。

## 运行时验收

- API `/readyz`：PostgreSQL、Redis 均为 `ok`。
- Worker `/readyz`：PostgreSQL、Redis 均为 `ok`。
- Admin Dashboard 连续 30 次请求：P95 `23.13 ms`，最大 `40.68 ms`，低于 `1.5 s` 目标。
- 快照状态：`ok`。
- 六域状态：`users/notice/mail/feedback/food/system = ok`。
- HMAC Critical 邮件真实入队后由 Worker 处理为 `accepted`，`attempt_count=1`，`delivered_at` 仍为空。
- 使用相同 `Idempotency-Key` 和新 nonce 重放，返回同一 delivery ID，没有重复投递。

## 环境边界

- SMTP、S3 与生产域名凭据必须由部署环境注入，不进入仓库。
- 本地开发编排已定义 MinIO 与 bucket 初始化；当前机器拉取固定 MinIO 镜像时网络超时，因此本轮以对象存储签名器单测、上传类型/大小/对象键校验和编排配置作为证据，未把本机 PUT 成功误报为已验证。
- Vite 构建仍报告旧版 Element Plus 全局兼容包与 ECharts chunk 大于 500 kB 的非阻断警告；新页面已按路由拆分，删除 Element Plus 仍按冻结决策另开 PR。

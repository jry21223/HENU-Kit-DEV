---
status: accepted
---

# Platform Core 使用 PostgreSQL 持久化并以 Redis 协调

Platform Core 使用 PostgreSQL 作为账户、权限、Session、Operations Inbox、邮件记录、审计及验证码安全记录的唯一持久数据源；Redis 只用于限流、一次性流程的短期协调、Nonce、防重放、临时锁和可重建缓存，不作为最终业务依据。

验证码由 Platform Core 生成，PostgreSQL 在同一可靠流程中保存验证码 Hash、有效期、消费状态和邮件 Outbox；Mail Worker 领取任务后通过 SMTP 或邮件供应商发送。Redis 不发送邮件，也不单独保存唯一的验证码事实，Redis 丢失只影响短期限制或要求重新开始临时流程。

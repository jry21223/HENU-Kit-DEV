---
status: accepted
---

# Platform Core 使用 pgx、sqlc 和版本化 SQL Migration

Platform Core 明确编写并审查 PostgreSQL SQL，通过 pgx 执行、sqlc 生成类型安全的 Go 调用代码，并使用版本化 SQL Migration 管理结构升级与回滚。账户、权限、Session、验证码、幂等和审计等关键路径不使用 GORM 或运行时 AutoMigrate，以便锁、事务、约束和并发行为保持可见且可测试。

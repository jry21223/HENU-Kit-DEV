---
status: accepted
---

# Platform Core 使用 Go

新的 `services/platform-core` 使用 Go 实现并独立部署，拥有统一账户、权限码与 Scope、Session、Operations Inbox 和邮件基础设施等平台数据。Platform Core 与 Console Gateway 可以共享通用工程包和生成契约，但不得互相导入内部业务模块或绕过网络契约形成进程级耦合。

HTTP 层使用 Go 标准库 `net/http` 与轻量 `chi` 路由器，不沿用 Study Legacy API 的 Gin Router。

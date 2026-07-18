---
status: accepted
---

# Console Gateway 使用 Go

独立部署的 `services/console-gateway` 使用 Go 实现，负责 Console Session、权限校验、并发聚合、超时、受控转发、审计和结构化日志。Gateway 不复制 Study Legacy API 的业务模型、不使用 GORM 管理业务表，并通过版本化 OpenAPI 或事件契约连接 Platform Core 与各 Active Product Module。

HTTP 层使用 Go 标准库 `net/http` 与轻量 `chi` 路由器，不复制旧服务的 Gin Router 和中间件结构。

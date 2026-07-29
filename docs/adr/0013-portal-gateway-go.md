---
status: accepted
---

# Portal Gateway 使用 Go

独立部署的 `services/portal-gateway` 使用 Go 实现，负责 Portal Session、OAuth 认证、默认只读的产品代理和用户端权限校验。Gateway 不复制 Console Gateway 的管理后台逻辑、不使用 Console 专用权限码，并通过版本化 OpenAPI 契约连接 Platform Core 与各 Active Product Module。

HTTP 层使用 Go 标准库 `net/http` 与轻量 `chi` 路由器，复用 Console Gateway 的 HMAC 签名、AES-GCM Session 编解码和 Redis OAuth State 模式。

## 决策

- Portal Gateway 与 Console Gateway 物理隔离，各自独立部署、独立 OAuth 客户端注册、独立 Session Cookie。
- Portal Gateway 默认只代理只读端点（GET），不转发通用写操作。任何自助写入都必须由一份明确 amends 本 ADR 的 ADR 限定路径、actor、独立凭据、幂等和失败语义；当前例外见 ADR-0017 与 ADR-0018。
- Portal 用户权限码为 `portal.library.read`、`portal.food.read`、`portal.practice.read`、`portal.notice.read`，与 Console 的 `console.*` 权限码完全隔离。
- 复用 Console Gateway 的 `serviceauth` 签名和 `session/codec` 模式，但使用独立的 cookie 名称和加密密钥。

## 约束

- Portal Gateway 不拥有任何业务数据，不连接产品数据库。
- Portal Gateway 不暴露服务端凭证给浏览器。
- Portal Gateway 依赖 Platform Core 部署（`account.henukit.cn`）才能完成认证流程。
- Portal Gateway 的 OpenAPI 契约独立于 Console Gateway。

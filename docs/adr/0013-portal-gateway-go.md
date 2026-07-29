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
- QuizCraft V2 目录的预接入路径为 `/api/v1/practice/catalog`。默认配置下该精确路径被既有 Practice wildcard 在 Gateway 内短路为 `404`，不得访问 Portal API 或 QuizCraft Core；只有 `PORTAL_ENABLE_QUIZCRAFT_CATALOG=1` 才注册真实的只读 Core handler。Portal 构建还必须显式设置 `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1` 才请求或显示该目录，两项默认均为 `0`。在 #166 全量切流前，该路径不加入公开 OpenAPI，也不构成生产可见 V2 入口。
- 受控目录读取始终使用显式 `anonymous` Core actor；目录是公开且无用户特定结果，现有读签名不绑定 actor header，因此 Gateway 不从有效、缺失或无效的浏览器 Session 转发 UserID，也不把公开目录误报为登录失败。

## 约束

- Portal Gateway 不拥有任何业务数据，不连接产品数据库。
- Portal Gateway 不暴露服务端凭证给浏览器。
- Portal Gateway 依赖 Platform Core 部署（`account.henukit.cn`）才能完成认证流程。
- Portal Gateway 的 OpenAPI 契约独立于 Console Gateway。

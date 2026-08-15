---
status: accepted
amends: 0032
---

# Food Post 投稿功能封装为远程 MCP 服务

ADR-0032 让 Food 拥有 Food Post 创建与公开读。现在把这个能力以 Model
Context Protocol (MCP) 暴露给远程 AI 代理:一个独立服务
`services/food-mcp`,用 Streamable HTTP 传输,把投稿功能变成 AI 客户端
(Claude / Cursor / 其他 MCP 客户端)可以直接调用的工具,而不是必须走
Portal 浏览器流程。

## Decision

- **独立 Go 服务 `services/food-mcp`**。它不包含业务逻辑,只是把 Food 的
  签名 HTTP 契约翻译成 MCP 工具;Food 仍然是唯一数据与策略所有者
  (ADR-0032 不变)。独立 module,不依赖 portal-gateway 的 internal 包
  (那些是 `internal/`,跨服务不可导入),自带 5 行 canonical 签名实现。
- **传输:Streamable HTTP**(远程),入口 `POST /mcp` 由
  `mcp.NewStreamableHTTPHandler` 提供。不接受 stdio 模式——这是给远程
  代理用的边界。
- **认证**:
  - MCP 客户端 → food-mcp:`FOOD_MCP_ACCESS_TOKEN` 承载在
    `Authorization: Bearer`。未配置 token 时服务拒绝启动(fail closed),
    这是远程暴露边界的信任根。
  - food-mcp → food:复用 ADR-0032 的凭据环
    (`FOOD_POST_CREATE_CLIENT_ID/SECRET/KEY_ID` 与
    `FOOD_POST_READ_CLIENT_ID/SECRET/KEY_ID`),5 行 canonical 签名,与
    Portal Gateway 的转发完全一致。创建与读各自独立凭据。
- **actor 模型:调用方自报**。MCP 客户端没有 Portal Session,因此 create 与
  mine 工具要求调用方在参数里提供 `actor_user_id`(UUID)与(创建时)
  `actor_display_name`。信任边界:MCP 客户端已经通过 Bearer token 认证,
  actor 自报与该信任一致——这是远程代理边的既定取舍,不是匿名通道。
  food-mcp 校验 UUID 格式与 display name 非空 ≤120(与 food 服务端相同),
  之后原样绑进签名请求。
- **工具集**(全部映射 ADR-0032 的 Food 路由):
  1. `create_food_post` — 必填 `venue_name`/`campus`/`tier`/`review_text`/
     `actor_user_id`/`actor_display_name`;可选 `price_reference`/
     `hours_reference`/`dishes`(≤6)/`images`(≤6,base64 无前缀,≤2MiB);
     服务端生成并保留幂等 key(每次调用一个新 key),透传 Food 的 429
     `DAILY_POST_CAP_REACHED` 为可读错误。
  2. `list_food_posts` — 可选 `campus` 过滤,公开读。
  3. `get_food_post` — 单个帖子详情。
  4. `list_food_venues` — 必填 `campus`。
  5. `list_my_food_posts` — 必填 `actor_user_id`。
  - 图片**字节**读取不作为 MCP 工具暴露(图片 URL 已含在帖子 wire 的
    `images` 字段,由客户端自行决定如何展示)。
- **错误语义**:Food 的非 2xx 原样转成 MCP 工具错误(状态码 + error code +
  message),调用方(模型)可以读到 `DAILY_POST_CAP_REACHED` 这类明确原因,
  不吞错误、不回退、不伪造成功。
- **配置**:`FOOD_MCP_ADDR`(默认 `:8098`)、`FOOD_MCP_ACCESS_TOKEN`(必填)、
  `FOOD_POSTS_URL`、两套 food-post 凭据。无 `_ENABLED` 开关,缺 token 即
  拒绝启动。
- **测试**:fake Food 上游 + httptest 起 MCP server,用 SDK client 调工具,
  断言:签名头正确、actor 自报绑定、429 透传、幂等 key 生成、图片上限
  校验、错误映射。既有 `services/food` 与 `services/portal-gateway`
  测试保持全绿(本服务不改它们)。

## Consequences

- 远程 AI 代理可以直接发布校园美食投稿,不经浏览器;Food 的每日上限、
  幂等、图片限制与公开语义全部在 Food 侧强制,没有第二套业务规则。
- 引入一个新的受信服务边界(food-mcp),需要运维提供 `FOOD_MCP_ACCESS_TOKEN`
  并只把它暴露给受信代理。actor 自报意味着 MCP 通道上的身份是调用方声明的,
  不是平台验证的——任何"以他人名义投稿"的担忧需要靠 MCP 客户端的用户
  认证层解决,不在本服务范围内。
- 投稿内容与账户控制台体验保持分离:通过 MCP 创建的帖子同样立即公开、
  显示调用方声明的 display name,出现在 `/food` 榜单与该 actor 的
  "我的投稿"页。

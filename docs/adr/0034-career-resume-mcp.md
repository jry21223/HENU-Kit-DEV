# 简历上传 AI 提取能力封装为远程 MCP 服务

## Status

Accepted

## Context

求职画像（Career Profile）的简历上传 → 后台 AI 识别 → 画像草稿回填能力已经作为
HTTP 能力交付（`services/career-opportunities` 的
`/api/v1/career/profile/extractions`，经 Portal Gateway 暴露）。现在要把这个能力以
Model Context Protocol (MCP) 暴露给远程 AI 代理：一个独立服务
`services/career-mcp`，用 Streamable HTTP 传输，让 AI 客户端（Claude / Cursor /
其他 MCP 客户端）可以直接上传简历并查询识别结果，而不是必须走 Portal 浏览器流程。

已有先例 ADR-0033（food-mcp）：独立 Go 服务、Streamable HTTP、Bearer token 为
信任根、actor 自报。career 与 food 的关键差异是**签名契约**：Food 的 canonical
签名是 5 行、actor 不参与签名；Career 的 canonical 签名是 **6 行，actor 作为第六
行绑定进签名**（`services/career-opportunities` 的 `authenticate` 与
`services/portal-gateway/internal/serviceauth` 的 `SignWithActor`），因此不能直接
复用 food-mcp 的 signing 实现。

另一个差异是会员门：Portal 通道上 Career 的所有路由都经 Gateway 做 Lifetime
membership 校验（`requireLifetime`），而 Career 服务本身不做。MCP 客户端没有
Portal Session，无法携带会话凭证。

## Decision

- **独立 Go 服务 `services/career-mcp`**。它不包含业务逻辑，只是把 Career 的
  签名 HTTP 契约翻译成 MCP 工具；Career 仍然是唯一数据与策略所有者。独立
  module，不依赖 portal-gateway 的 internal 包，自带 6 行 canonical 签名实现
  （`internal/signing`，contract 测试对齐 Career 的 `authenticate`）。
- **传输：Streamable HTTP**（远程），入口 `POST /mcp` 由
  `mcp.NewStreamableHTTPHandler` 提供。不接受 stdio 模式——这是给远程代理用
  的边界。
- **认证**：
  - MCP 客户端 → career-mcp：`CAREER_MCP_ACCESS_TOKEN` 承载在
    `Authorization: Bearer`。未配置 token 时服务拒绝启动（fail closed），这是
    远程暴露边界的信任根。
  - career-mcp → career：复用 Portal Gateway 的 career 凭据环
    （`CAREER_CLIENT_ID` / `CAREER_CLIENT_SECRET` / `CAREER_KEY_ID`），6 行
    canonical 签名，actor 绑定进签名。
- **actor 模型：调用方自报，不做会员身份校验**。MCP 客户端没有 Portal
  Session，因此工具要求调用方在参数里提供 `actor_user_id`（UUID）。信任边界：
  MCP 客户端已经通过 Bearer token 认证，actor 自报与该信任一致——与 food-mcp
  完全相同的既定取舍。**明确不在 MCP 通道校验 Lifetime membership**：这是
  Portal 通道的门，MCP 通道由 token 分发控制谁能调用；如需收紧，应给受信代理
  单独分发 token，而不是在 career-mcp 里引入 membership 依赖。
- **工具集**（映射 Career 的提取路由，异步语义与 Portal 一致）：
  1. `upload_resume` — 必填 `file_name`（安全 basename ≤255）/ `content`
     （base64 无前缀，解码后 ≤10 MiB）/ `actor_user_id`；创建异步识别任务并
     返回任务 id（PDF/DOCX/TXT 校验、AI 未配置 `AI_UNCONFIGURED`、限流
     `EXTRACT_RATE_LIMITED` 均由 Career 服务决定并透传）。
  2. `get_resume_extraction` — 必填 `extraction_id` / `actor_user_id`；查询
     状态与提取结果（completed 时 `extracted` 即画像草稿）。
  - 识别是异步的（通常几十秒），因此拆成两个工具让模型"上传 → 轮询查询"，
    避免单次 MCP 调用在 AI 返回前超时。
- **错误语义**：Career 的非 2xx 原样转成 MCP 工具错误（状态码 + error code +
  message），模型可以读到 `AI_UNCONFIGURED` / `EXTRACT_RATE_LIMITED` /
  `EXTRACT_FAILED` 这类明确原因，不吞错误、不回退、不伪造成功。
- **配置**：`CAREER_MCP_ADDR`（默认 `:8099`）、`CAREER_MCP_ACCESS_TOKEN`（必填）、
  `CAREER_URL`、career 凭据环。无 `_ENABLED` 开关，缺 token 即拒绝启动。
- **测试**：fake Career 上游 + httptest 起 MCP server，用 SDK client 调工具，
  断言：6 行 actor 绑定签名正确、actor 自报绑定、multipart 文件字节透传、
  输入校验（危险文件名/非法 base64/超大内容）、`AI_UNCONFIGURED` 与
  `EXTRACT_RATE_LIMITED` 透传、无 token 401、空 token fail closed。

## Consequences

- 引入一个新的受信服务边界（career-mcp），需要运维提供
  `CAREER_MCP_ACCESS_TOKEN` 并只把它暴露给受信代理。actor 自报意味着 MCP
  通道上的身份是调用方声明的，不是平台验证的；**MCP 通道不校验 Lifetime
  会员**，与 Portal 通道行为不同——任何"通过 MCP 绕过会员门"的担忧靠 token
  分发控制，不在本服务范围内。
- 简历文件字节经 MCP 通道以 base64 参数传输（≤10 MiB，与 food-mcp 图片传输
  同一模式），到达 Career 后仍遵守"解析后即弃、只留提取文字"的承诺。
- 部署镜像清单从 16 增至 17（`henukit-career-mcp`），compose 两处与运维文档
  需同步。

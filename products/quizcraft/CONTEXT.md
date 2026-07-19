# QuizCraft Context

## Language

**Practice Core**:
QuizCraft 中基于一个不可变题库版本创建练习、提交答案并记录结果的业务边界。
_Avoid_: 临时 JSON 刷题、前端题库事实

**Stable Question ID**:
由稳定题库键和源题目 ID 确定、不会因题目内容修改而变化的题目身份。一次具体内容使用独立的 Question Version ID。
_Avoid_: 数组下标、题目文本哈希即题目 ID

**Question Bank Workshop**:
显式导入、校验、版本化与发布题库的管理边界。导入报告必须说明题数、题型、答案、章节与哈希结果。
_Avoid_: 服务启动扫描、JSON 运行时兜底

## Owns

- Practice、Favorites、Ranking、Feedback 与 Question Bank Workshop 的 QuizCraft 契约。
- 稳定 Bank/Question ID、不可变 Bank/Question Version ID 以及题库版本成员关系。
- PostgreSQL 中的题库事实和显式 JSON 导入报告。

## Does not own

- Platform 账户、Console Session、权限或产品 Scope。
- 本地 JSON 文件的自动发现、启动同步或生产运行时兜底。
- 旧 FastAPI 到 Go 的生产切换；该切换属于后续迁移事项。

## Current boundary

HC-16 冻结五个产品域的 OpenAPI，并建立可独立验证的 Go/PostgreSQL 导入基线。HC-17 在该基线上提供 Practice Core 影子 HTTP 服务、四类服务端判题、Session/重放事实、登录学习状态与可选旧响应对比。现有 FastAPI 仍是当前运行实现，Go 与其并行存在；本阶段没有替换生产路由或流量。

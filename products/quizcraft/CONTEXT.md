# Practice Service Context

## Language

**Practice Core**:
练习服务中基于一个不可变题库版本创建练习、提交答案并记录结果的业务边界。
_Avoid_: 临时 JSON 刷题、前端题库事实

**Stable Question ID**:
由稳定题库键和源题目 ID 确定、不会因题目内容修改而变化的题目身份。一次具体内容使用独立的 Question Version ID。
_Avoid_: 数组下标、题目文本哈希即题目 ID

**无人使用维护期**:
因学期或考试结束而不预期有真实用户流量的练习服务维护时段；服务仍可运行，且该事实本身不证明后台任务或数据库写入已停止。
_Avoid_: 服务已停止、零写入窗口

**技术停写窗口**:
全量切换前由系统强制拒绝持久化变更的短时状态；它从旧服务写入被证明阻断开始，到最终同步、快照、对账和 Go 写入验证全部通过后结束。
_Avoid_: 无人使用维护期、人工口头停写

**维护窗口全量切换**:
在技术停写窗口内完成最终对账后，一次性将练习服务全部读写流量从 FastAPI 切换到 Go 的迁移方式；它只取代比例灰度步骤，不降低快照、对账、健康和回退门禁。
_Avoid_: 比例灰度、未停写全量放量、大爆炸重写

**Go 写入承诺点**:
全量切换后 Go 数据库接受首笔持久化业务变更的时刻；在此之前可以从最终快照回到旧服务，在此之后必须前向恢复，或先完整反向同步并对账才能回退。
_Avoid_: 流量已切换、Go 服务已启动

## Owns

- Practice、Favorites、Wrong Answers、Statistics、Ranking 与 Feedback 契约。
- 稳定 Bank/Question ID、不可变 Bank/Question Version ID 以及题库版本成员关系。
- PostgreSQL 中的题库事实和显式 JSON 导入报告。

## Does not own

- Platform 账户、Console Session、权限或产品 Scope。
- 独立产品首页、题库管理界面、管理令牌界面、转盘或独立 OAuth 登录。
- 本地 JSON 文件的自动发现、启动同步或生产运行时兜底。
- 旧 FastAPI 到 Go 的生产切换；该切换属于后续迁移事项。

## Current boundary

HC-16 冻结五个产品域的 OpenAPI，并建立可独立验证的 Go/PostgreSQL 导入基线。HC-17 在该基线上提供 Practice Core 影子 HTTP 服务、四类服务端判题、Session/重放事实、登录学习状态与可选旧响应对比。HC-161 额外准备 ADR-0018 的两条私有 Portal Practice command 路径：Core 使用独立 command key ring 验证精确原始请求体、nonce 和可选签名 actor；游客只能由 Core 签发的 `quizcraft_anonymous` cookie 识别。路径与写门默认关闭，且不会改变当前生产路由或流量；#166 才可在迁移证据和三重 gate 都满足后切换。

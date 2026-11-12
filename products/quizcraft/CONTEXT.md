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

## English training language (ADR-0060)

**Training Unit**: 一组完整上下文的通用英语听力或完形材料，以稳定 Unit ID 跨版本延续。
_Avoid_: 英语首页、浏览器题库、官方 CET 真题

**Training Unit Version**: 绑定不可变题库版本、有序精确题目版本及整份材料语义的不可变版本；语义修订产生新版本，旧会话继续使用原版本。
_Avoid_: 当前文章、可变音频 URL、题目数组快照

**Stable Part / Blank / Sentence Segment**: 材料中可跨重排保持身份的文本部分、空位与完整句音频片段；证据引用稳定 ID。
_Avoid_: 字符偏移、数组下标、平均切段

**Training Session**: 属于单一学习者、固定一个 Training Unit Version 和全量题目成员关系的训练会话；整个会话不进入普通考试指标或排行。
_Avoid_: 普通练习模式、辅助题剔除后计分

**Independent Correct**: 首次作答正确且该题提交前没有适用的已披露提示。
_Avoid_: 面板关闭即独立、提示后正确率

**Assisted Completion**: 首次作答正确但使用了适用提示，仍需要独立复习。
_Avoid_: 独立掌握、普通考试正确

**Training Disclosure**: Core 已确认的提示或复盘文本层级披露事实；刷新、重放和账号续接保留该事实。
_Avoid_: 浏览器显隐、事后取消辅助

**Training Review Item**: Core 学习状态中仍未独立完成的问题聚合及其固定完整会话上下文入口；新的独立成功可清除待复习状态。
_Avoid_: 当前页面结果、精准长期诊断

HC-641 仅冻结 [ADR-0060](../../docs/adr/0060-core-owned-english-training.md)、
[契约](../../docs/development/english-training-contract.md)及生成类型。
训练内容、首答、披露、恢复和学习状态由 Practice Core 所有；未在此切片实现运行能力。


HC-642 implements the listening first-answer slice through explicit Workshop
source imports, immutable unit/bank/question bindings, signed training routes and
Core-owned recovery. Training sessions are created atomically with a purpose and
unit binding. Ordinary projections use `quizcraft_ordinary_attempts`; canonical
training attempts retain factual correctness and independent review outcomes.
Guest-to-account writes retain prior facts in the learning projection; reads never
claim ownership. Shared review requires every exact first answer. Hint/reveal
commands remain closed for the following listening/cloze tickets. The publication
package and the post-write old-binary recovery boundary are documented under
`go-service/content/english-training/`. No production content is automatically
loaded; new human listening acceptance and production publication remain separate.

**Training Consumer Compatibility Boundary**:
任何已创作训练内容（含未发布草稿）、训练 Session 或 Attempt 存在后，发布回退目标必须具备经源码和制品绑定的训练数据消费者能力。Core 只读探针拥有这些事实；发布脚本不得复制业务判定或通过恢复旧备份丢弃接受的写入。
_Avoid_: 有答题后才不兼容、备份即可回旧版

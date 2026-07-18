# HENU Kit 管理产品上下文

本上下文定义 HENU Kit 管理产品在迁移期的新旧边界，避免把仍然存在的旧代码误认为新产品的一部分。

## Language

**HENUKit Console**:
面向整个 HENU Kit 产品族的统一管理入口，覆盖平台能力与明确仍在运营的子产品，同时保持各子产品的数据所有权和运行边界。
_Avoid_: 新版旧后台、期末复习平台后台

**Active Product Module**:
经明确确认仍属于 HENU Kit 产品族、通过自身契约接入 HENUKit Console 的运营模块；代码仍存在或旧页面仍可运行不能使一个模块自动成为 Active Product Module。
_Avoid_: 旧功能、仓库内现成功能

**Portal Module**:
HENUKit Console 中只读呈现 HENU Kit 新主站部署版本、可用状态、反馈和运行异常的首个 Active Product Module；V1 不编辑主站内容、不控制部署或回滚。
_Avoid_: 平台总览、旧版首页管理

**Portal Configuration**:
定义主站导航、工具入口、展示文案和页面结构的仓库代码或配置文件，通过 Git、PR 和部署流程变更。
_Avoid_: 后台内容、运行时 CMS 配置

**Platform Operations**:
HENUKit Console 中管理统一账户、作用域角色、Session、邮件基础设施、审计和平台运行状态的公共能力模块，与 Portal Module 平级。
_Avoid_: Portal 后台、Study 管理后台

**Console Gateway**:
独立部署且无业务状态、专门为 HENUKit Console 验证后台权限、聚合模块摘要并转发受控操作的网关；它不属于 Platform Core 或 Study Legacy API，也不直接连接子产品数据库。
_Avoid_: Console BFF、services/api 管理路由、共享业务数据库后台

**Console Session**:
Console Gateway 使用 Platform Core 账户中心的一次性授权码建立的独立管理会话；会话使用 HttpOnly/Secure Cookie，不跨站共享，也不向浏览器暴露长期 Token。
_Avoid_: 共享登录 Cookie、浏览器 Access Token

**Console Access Context**:
Platform Core 为 Console Session 提供的权限码与 Scope 集合，决定模块可见性和可执行操作；默认拒绝，不使用旧 `isAdmin` 或单一 Role 字符串授权。
_Avoid_: Admin 总开关、前端角色判断

**Operations Inbox**:
Platform Operations 中汇总跨产品待办的运营队列，只保存负责人、优先级、SLA、状态和业务资源引用；完整反馈正文与处理细节仍由来源产品拥有。
_Avoid_: Feedback 产品模块、跨产品反馈数据库

**Console Overview**:
HENUKit Console 的顶层看板，由 Portal、Platform Operations、Notice、Library、QuizCraft 和 Food 六个模块组成；用户、邮件、统一待办和系统状态属于 Platform Operations 内部指标。
_Avoid_: 六业务域大屏、旧后台总览

**Notice Module**:
HENUKit Console 中独立管理校园通知来源、不可变内容版本、审核、受众和分发生命周期的 Active Product Module；通知数据由 Notice 服务拥有。
_Avoid_: Platform Operations 通知页、Portal 公告编辑器

**Library Module**:
HENUKit Console 中管理课程、资料、下载、投稿、审核和纠错的 Active Product Module；当前边界不包含刷题、社区、动态、支付、积分或会员。
_Avoid_: Study Legacy Admin、学习平台后台

**QuizCraft Module**:
以 React 前端与 Go 后端重构的 Active Product Module；HENUKit Console 只呈现题库、反馈和服务状态摘要并提供深链接，题库维护与审核继续由 QuizCraft 自有后台承担。
_Avoid_: Library 刷题页、Console 内置题库后台

**QuizCraft Practice Core**:
QuizCraft 重构时必须保留的题库选择、随机/难题/章节练习、单选/多选/判断/填空、答题结果与解析、学习进度、错题记录、题内纠错反馈和公开排行榜能力。
_Avoid_: QuizCraft 全部旧功能、旧 Ops 模式

**Question Bank Workshop**:
QuizCraft 自有后台中创建、编辑、版本化、导入、人工校验、发布、下架和回滚题库的管理能力；使用 HENU Kit 权限码与 Scope，不使用独立 Admin Token。
_Avoid_: Console 题库编辑器、临时上传脚本

**Public Ranking**:
QuizCraft Practice Core 中提供公开竞争与娱乐性的排行榜，以服务端确认的累计答对次数为主要指标，并允许用户通过重复练习同一道题继续累计；接口失败时不得展示伪造数据。
_Avoid_: 个人统计、示例排行榜、去重掌握题目榜

**Bank Ranking**:
仅比较同一题库内学习表现的 Public Ranking，保留题库自身的题量和难度边界。
_Avoid_: 章节榜、跨题库榜

**Overall Ranking**:
汇总用户在所有有效题库中累计答对次数的默认 Public Ranking；必须由可审计的分题库正确答题事实计算。
_Avoid_: 单题库榜、去重题目总榜

**Correct Answer Count**:
服务端判定为正确的累计答题次数；同一用户在新的有效练习中再次答对同一道题仍增加计数。
_Avoid_: 掌握题目数、去重正确题数

**Scored Attempt**:
Practice Session 中某道题第一次被服务端判定的提交；每个 Session、用户和题目最多产生一个计分结果，请求重放或并发重复提交不能重复增加 Correct Answer Count。
_Avoid_: 提交请求、客户端判题结果

**Ranking Profile**:
用户用于 Public Ranking 的公开昵称、系统头像和参与开关；不包含邮箱、学号、真实姓名或内部 user_id。
_Avoid_: 用户档案、账户身份

**Weekly Ranking**:
按自然周统计 Correct Answer Count 的 Public Ranking，周一开始新周期但不删除历史答题事实；Overall Ranking 默认展示本周榜。
_Avoid_: 清空历史榜、滚动七天榜

**Lifetime Ranking**:
累计全部历史 Scored Attempt 的 Public Ranking，与 Weekly Ranking 并存。
_Avoid_: 本周榜、永久周榜

**Ranking Settlement Event**:
记录某个排行周期最终名次的可审计事件，仅为未来扩展提供事实；V1 不据此计算暗积分、会员或可兑换权益。
_Avoid_: 排行奖励、隐藏积分

**Favorite Question**:
用户主动收藏并归属于该用户的题目引用；收藏不复制题目正文，题目内容继续由 QuizCraft 题库拥有。
_Avoid_: 错题、书签页面快照

**Per-Bank Favorites Folder**:
系统按题库为每个用户自动提供的收藏集合；一个题库对应一个收藏夹，V1 不支持自定义命名、跨题库移动或共享。
_Avoid_: 全局收藏夹、自定义收藏夹、收藏分类

**Favorites Overview**:
汇总展示用户在各题库的收藏数量和入口的导航页；它不合并题目，也不提供跨题库练习。
_Avoid_: 全局收藏题库、混合收藏练习

**Unavailable Favorite**:
原题已下架、删除或当前无权访问但用户收藏关系仍被保留的 Favorite Question；不展示题目正文，也不进入练习题集。
_Avoid_: 已删除收藏、题目快照

**Favorites Practice**:
用户以某一个 Per-Bank Favorites Folder 为题源发起的专属练习，复用 QuizCraft Practice Core 的答题与结果流程，不跨题库混合题目。
_Avoid_: 错题重练、公共收藏题库

**Authenticated Learning State**:
必须绑定 HENU Kit 登录用户、可跨设备同步的收藏、错题和学习进度；游客可以刷题，但不能创建该状态。
_Avoid_: 浏览器本地收藏、匿名用户档案

**Food Module**:
HENUKit Console 中处理美食投稿审核、异常票和调档确认的 Active Product Module；Food 服务拥有业务数据，Console 仅通过 Food API 执行运营操作。
_Avoid_: Platform Operations 美食页、共享数据库美食后台

**Candidate Capability**:
正在评估但尚未进入 HENUKit Console 正式产品边界的能力；现有代码、数据表或旧页面不能代替产品决策，评估期间不显示在导航或数据看板。积分和会员当前属于此状态。
_Avoid_: 暂时保留功能、默认启用模块

**Study Legacy Admin**:
原期末复习平台在迁移期保留的存量运营后台，作为独立构建和部署单元存在；它不是 HENUKit Console 的功能分区或共享前端 Bundle。
_Avoid_: 旧版运营、新 Console 的兼容菜单

**Study Legacy API**:
迁移期继续承载旧学习平台接口和数据模型的现有 `services/api`；不得继续接收 HENUKit Console 或新 Platform Core 的实现。
_Avoid_: Platform Core、新 Console API

**Compatibility Adapter**:
迁移期连接旧接口或旧数据模型的受限兼容层；它可以支撑回滚，但不得成为 HENUKit Console 的导航或产品概念。
_Avoid_: 旧功能入口、临时新功能

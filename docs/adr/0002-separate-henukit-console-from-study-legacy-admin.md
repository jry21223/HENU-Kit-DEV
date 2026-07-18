---
status: accepted
---

# HENUKit Console 与 Study Legacy Admin 硬隔离

HENUKit Console 是整个 HENU Kit 产品族的统一管理入口，只承载平台能力及明确仍在运营的 Active Product Module；各子产品继续拥有自己的数据，并通过契约接入 Console。原期末复习平台的存量能力归入独立的 Study Legacy Admin，不出现在新 Console 的导航或产品模型中。迁移期可以保留 Compatibility Adapter 和独立旧入口用于连续运营与回滚，但保留代码不等于把旧产品能力并入新后台。

## Consequences

- 新旧后台使用独立的应用目录、前端 Bundle、路由树、导航入口、构建、部署和退役边界；Study Legacy Admin 从当前 `apps/admin` 物理拆出，HENUKit Console 不打包旧页面或 Element Plus。
- 新建独立部署的 Console Gateway，专门为 HENUKit Console 验证后台权限、聚合模块摘要并转发受控操作；它不能写入现有混合 `services/api`，也不直接连接各子产品数据库。现有 `services/api` 作为 Study Legacy API 保留旧接口和数据模型。
- Console Gateway 保持无业务状态，只允许短期缓存、限流和请求追踪；用户、角色、Operations Inbox 及各产品业务数据继续由 Platform Core 或对应产品拥有。
- 管理员通过 Platform Core 账户中心登录，Console Gateway 在服务端使用一次性授权码交换身份与作用域权限并建立独立 Console Session；浏览器不保存长期 Token，也不跨站共享 Cookie，角色冻结和 Session 撤销必须在目标时限内传播。
- Console 模块可见性和操作权限由 Platform Core 下发的权限码与 Scope 决定，并默认拒绝；旧 `isAdmin`、单一 `users.role` 字符串和纯前端判断不得作为授权证据。
- 统一 Console 不获得子产品数据库的直接访问权，跨产品聚合通过版本化 API、事件或受控 Adapter 完成。
- HENU Kit 新主站作为首个 Active Product Module 接入；Console 必须提供独立的 Portal Module，呈现主站部署、可用状态、反馈和运行异常。
- Portal V1 的导航、工具入口、展示文案和页面结构属于 Portal Configuration，通过 Git、PR 和部署流程变更；Console 不提供内容编辑、独立内容发布或任意页面搭建能力。
- Portal Module V1 只读，不提供重新部署、回滚或版本切换；生产变更继续由受保护的 CI/CD 流程执行。
- Platform Operations 与 Portal Module 平级，集中承载统一账户、作用域角色、Session、邮件基础设施、审计和平台运行状态，不归入主站或旧 Study 产品边界。
- 跨产品反馈进入 Platform Operations 的 Operations Inbox；中心只保存负责人、优先级、SLA、状态和业务资源引用，完整正文及处理细节继续由来源产品拥有。
- Console Overview 固定按 Portal、Platform Operations、Notice、Library、QuizCraft 和 Food 六个模块组织；用户、邮件、Operations Inbox 和系统状态是 Platform Operations 的内部指标，不作为平级产品域。
- 校园通知作为独立 Notice Module 接入，拥有来源、不可变内容版本、审核、受众和分发生命周期；通知数据由 Notice 服务拥有，Console 只能通过版本化契约访问。
- 现有 Study 收敛为 Library Module，当前只包含课程、资料、下载、投稿、审核和纠错；刷题、社区、动态和支付不随旧页面进入。
- QuizCraft 作为独立 Active Product Module 接入；Console V1 只展示题库、反馈和服务状态摘要并提供深链接，题库维护与审核继续留在 QuizCraft 自有后台，数据通过契约获取。
- QuizCraft 重构必须保留 Practice Core，并新增按用户保存 Favorite Question 及从个人收藏集合发起 Favorites Practice 的能力；收藏只保存稳定题目引用，不复制题目正文。
- Public Ranking 属于 QuizCraft Practice Core 的核心娱乐性，以服务端确认的 Correct Answer Count 为主要指标；同一用户在新的有效练习中重复答对同一道题继续累计，接口失败时不得回退到伪造榜单。
- Public Ranking 同时提供 Bank Ranking 与 Overall Ranking，并默认显示 Overall Ranking；总榜由可审计的分题库 Correct Answer Count 汇总。
- 排行榜只统计 Scored Attempt：每个 Practice Session、用户和题目最多计分一次；重新开始有效练习后可再次累计，请求重放、并发重复提交和客户端自报正确结果均不得加分。
- Public Ranking 只展示 Ranking Profile 中的自选排行昵称和系统头像，并允许用户退出公开排行；邮箱、学号、真实姓名和内部 user_id 不得公开，昵称受长度、敏感词和官方冒充限制。
- Overall Ranking 默认展示按自然周计算的 Weekly Ranking，并提供 Lifetime Ranking；Bank Ranking 同样支持周榜和历史榜，周榜换周不删除原始 Scored Attempt。
- V1 不展示排行奖励，也不计算或发放暗积分、会员和可兑换权益；系统只保留 Ranking Settlement Event，未来奖励方案另行决定且默认不追溯补发。
- QuizCraft 重构移除随机大转盘及 `food_wheel_items` 等美食数据；美食榜单及相关体验统一归 Food Module，QuizCraft 不再提供兼容入口。
- QuizCraft 保留 Question Bank Workshop，用于题库创建、编辑、版本化、标准 JSON 或 PDF/Word/TXT 导入、人工校验、发布、下架和回滚；它留在 QuizCraft 自有后台并使用统一权限码与 Scope，不再使用独立 `ADMIN_TOKEN`。
- QuizCraft V1 按题库为每个用户自动提供 Per-Bank Favorites Folder，支持收藏、取消收藏、题库内筛选和发起练习；不支持自定义命名、跨题库移动、跨题库混刷或共享收藏夹。
- QuizCraft 提供 Favorites Overview，只展示各题库收藏数量和入口，不合并题目或创建全局收藏练习。
- 游客可以刷题，但收藏、错题和学习进度属于 Authenticated Learning State；收藏操作触发统一登录后返回原题继续执行，V1 不实现浏览器本地收藏与账户数据合并。
- 题目下架、删除或失去访问权后，收藏关系保留为 Unavailable Favorite，但不展示正文且不进入练习；稳定题目 ID 未变的内容更新继续指向最新版，练习开始时明确提示被排除的数量。
- Food 作为独立 Active Product Module 接入；Console 直接承担投稿审核、异常票处理和调档确认，但所有操作必须调用 Food API，Console 不得连接或直接写入 Food 数据库。
- 积分和会员仍是 Candidate Capability，在产品决策完成前不视为 Active Product Module，不显示在 Console V1 导航或数据看板，也不得因旧实现存在而提前暴露。
- PR #21 的现有实现与本决策冲突，不再以修补后合并为目标；仅复用经边界审查通过的测试、UI 组件、契约片段和底层实现，并通过新的小型 PR 落地。
- PR #21 暂时保持 Draft 且不删除分支；新的架构与迁移计划 PR 建立并回链后关闭 #21，保留提交历史作为后续复用来源。
- 资料库等仍在运营的能力必须按当前产品归属重新接入，不能因为复用旧页面就继承旧平台边界。
- 积分、旧会员、旧支付、社区、Wiki、博客和旧 AI 等模块在完成逐项决策前，不得出现在 HENUKit Console。
- 回滚通过独立旧入口或 Compatibility Adapter 完成，不通过在新 Console 中公开“旧版运营”分区完成。

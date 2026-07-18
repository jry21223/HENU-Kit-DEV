---
status: accepted
---

# HENUKit Console 与 Study Legacy Admin 硬隔离

HENUKit Console 是整个 HENU Kit 产品族的统一管理入口，只承载平台能力及明确仍在运营的 Active Product Module；各子产品继续拥有自己的数据，并通过契约接入 Console。原期末复习平台的存量能力归入独立的 Study Legacy Admin，不出现在新 Console 的导航或产品模型中。迁移期可以保留 Compatibility Adapter 和独立旧入口用于连续运营与回滚，但保留代码不等于把旧产品能力并入新后台。

## Consequences

- 新旧后台使用独立的应用目录、前端 Bundle、路由树、导航入口、构建、部署和退役边界；Study Legacy Admin 已从原 `apps/admin` 物理拆到 `apps/study-legacy-admin`，HENUKit Console 不打包旧页面或 Element Plus。
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
- Food 作为独立 Active Product Module 接入；Console 直接承担投稿审核、异常票处理和调档确认，但所有操作必须调用 Food API，Console 不得连接或直接写入 Food 数据库。
- 积分和会员仍是 Candidate Capability，在产品决策完成前不视为 Active Product Module，不显示在 Console V1 导航或数据看板，也不得因旧实现存在而提前暴露。
- 资料库等仍在运营的能力必须按当前产品归属重新接入，不能因为复用旧页面就继承旧平台边界。
- 积分、旧会员、旧支付、社区、Wiki、博客和旧 AI 等模块在完成逐项决策前，不得出现在 HENUKit Console。
- 回滚通过独立旧入口或 Compatibility Adapter 完成，不通过在新 Console 中公开“旧版运营”分区完成。

QuizCraft 的 Practice、Favorites、Public Ranking、Question Bank Workshop 和迁移验收行为由领域 Context、执行规格及 QuizCraft ADR 维护；PR #21 的替代与关闭顺序属于替代计划，而不是本硬隔离 ADR 的生命周期决策。

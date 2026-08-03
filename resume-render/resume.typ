#import "template.typ": *

#let theme-color = rgb("#26267d")

#show: resume.with(
  size: 9pt,
  theme-color: theme-color,
  margin: (
    top: 1.05cm,
    bottom: 1.1cm,
    left: 1.35cm,
    right: 1.35cm,
  ),
  header-center: true,
)[
  = 金仁源

  #info(
    color: theme-color,
    (content: "(+86) 139-1106-0322"),
    (content: "2319276740@qq.com", link: "mailto:2319276740@qq.com"),
    (content: "github.com/jry21223", link: "https://github.com/jry21223"),
    (content: "河南大学 · 网络工程 · 2025 级"),
  )
][
  *求职方向：Forward Deployed Engineer / AI 应用工程实习。*具备从模糊业务需求到可运行系统的端到端交付经验，覆盖需求澄清、方案设计、全栈开发、AI 系统集成、私有化部署、上线运维与问题闭环。
]

== 核心能力

- *客户交付：*需求澄清、业务流程拆解、技术方案、原型验证、跨角色沟通、系统联调与上线支持
- *AI 应用：*LLM API、Tool Calling、MCP、Agent 工作流、RAG、结构化输出、Benchmark / Eval
- *工程能力：*Go、Python / FastAPI、TypeScript、React / Vue、PostgreSQL、Redis、Docker、Linux、CI / CD

== 项目经历

#item(
  [*开封府景区业务系统*],
  [*客户交付 / AI 系统集成*],
  date[2025.10 - 至今],
)
#tech[微信小程序 · OpenAPI · Qwen · Qdrant · Linux · GPU 私有化]

- 对接景区真实运营需求，参与一期游客端小程序、管理后台、节目倒计时及运营配置的开发、联调与上线，并维护接口契约与运行配置。
- 使用两张 GPU 以张量并行方式部署 Qwen3.6-27B，预留第三张 GPU 作为冗余备用资源；接入 Qdrant 与 Reranker，完成景区本地 RAG 客服集成及日志诊断。
- 负责二期方案设计与推进：针对小程序与线下剧本游割裂的问题，规划“付费 NPC + 免费自助”双轨体验，拆解游客端、核销端、运营后台与服务端，并设计扫码搜证、数字线索、支付核销、整案试跑及异常进度干预机制。

#item(
  link("https://github.com/jry21223/MetaView", [*MetaView 教育可视化 AI 系统*]),
  [*AI Agent / 全栈开发*],
  date[2026.03 - 2026.07],
)
#tech[FastAPI · Pydantic · Agent Tool Loop · React · Remotion · Benchmark / Eval]

- 独立设计可分步播放、调参、代码同步与继续追问的教育可视化工作台，覆盖数学、算法和代码场景，完成约 20 个精修讲解案例。
- 以统一 PlaybookScript 驱动渲染，设计 Single / Agent 双生成路径、学科 Skill 路由及确定性计算工具，降低数值错误与前后端耦合。
- 建立标准案例、隐藏变体和多维评分组成的评测流程；将 Follow-up 保存为独立版本，支持历史恢复，避免新生成结果覆盖已验证内容。

#item(
  link("https://github.com/jry21223/HENU-Kit", [*HENU Kit 统一校园平台*]),
  [*产品负责人 / 全栈开发*],
  date[2026.01 - 至今],
)
#tech[Go · FastAPI · TypeScript · PostgreSQL · Redis Streams · OpenAPI · MCP · Playwright]

- 整合 HENU Assistant、QuizCraft CN、校园资料库与账户系统；承接原助手约 270 名用户及刷题平台 200+ 用户、约 1 万次答题的存量能力与迁移准备。
- 封装教务、图书馆及研讨室系统，提供课表、空教室、座位 / 研讨室查询预约和请假辅助；将能力适配为 MCP Server、LangBot Plugin、OpenClaw Skill 与 CLI。
- 采用 OpenAPI 契约、兼容代理、功能开关和双跑验证推进渐进迁移；建立 PKCE（S256）授权、服务间认证、exact-SHA 发布、健康检查、E2E 与回滚门禁。

== 教育背景与荣誉

#item(
  [*河南大学*],
  [计算机工程与软件学院 · 网络工程],
  date[2025.09 - 至今],
)

- 2025 级本科生；入选信息安全实验室、知衍大模型实验室；校英语演讲队成员
- 2026 中国高校计算机大赛（4C）省级二等奖；华为 ICT 大赛省级奖项；河南大学数学建模竞赛校级一等奖；HCIP-WLAN

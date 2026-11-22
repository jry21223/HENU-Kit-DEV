# HenuBot 想法记录

> 状态：方向讨论中，技术路线与 MVP 范围尚未确定。
> 整理日期：2026-11-23（Asia/Shanghai）。
> 本文根据本次对话整理用户意图、最新反馈和开放问题，供后续讨论接续使用；不作为已经批准的实施规格。

## 1. 现在最纠结的是什么

用户最新反馈：

> “我感觉还是有点重了……设计吧，有点不太好。”
>
> “如果直接照搬的话，又太重了，又不想抄。如果自己从头搓的话，又问题太多。”

这次要重新考虑的是整体设计方向，不能只把上一版删掉几个页面或工具，就当作问题已经解决。

当前希望找到一个更合适的复用边界：可以学习成熟 Agent 的好设计、复用必要部件，同时保留自己能够理解、掌握和定制的核心。具体哪些复用、哪些自己写，仍需讨论。

当前行动是先把想法放下来。暂未确定采用哪个框架，也未要求按上一版方案开始实现。

## 2. 用户已经表达的目标

- 做一个校园 Bot，将现有 Henubot / HENU_Assistant 的基本插件能力迁移到 QQ 官方机器人形态。
- 目前使用 LangBot，希望新的实现更轻量、容易理解和维护，不再不断叠加复杂度。
- 现有功能不能丢。账号绑定、课表与日程、成绩、图书馆、河宝等只是概括；完整范围要以现有项目实际功能逐项核对。
- 高度可定制，尤其是人设、语气、回复风格，希望交互自然、尽可能拟人。
- 做好多用户隔离，个人账号、对话、数据与操作不能串到其他用户。
- 支持合适的表情包和更好的回复样式；QQ 所谓“响应式”能力还需要明确含义并核实实际可用范围。
- 关心后台能看到什么、怎样调试和配置。此前要求过站点预览，但正式后台的规模和接入方式尚未确定。
- 希望通过这次机会学习现代 Agent 设计，理解关键机制，并形成自己的判断。

这里的“QQ 官方机器人”指使用 QQ 开放平台的机器人形态。HENU Kit 是学生自主开发的项目，不代表河南大学官方。

## 3. 已经讨论过什么，哪些没有定案

| 讨论内容 | 当前状态 |
| --- | --- |
| Pi、mini-swe-agent、Hermes、LangGraph、OpenClaw、nanobot、PicoClaw 等设计 | 已做过初步调研，可继续作为学习材料；尚未选择直接采用哪个项目 |
| Python + PydanticAI + FastAPI + SQLite | 助手上一版提出的组合，未确定采用 |
| 单 Agent、七个工具、四个后台页面、5—20 人内测、四轮模型调用预算 | 上一版 MVP 的具体建议，未确定为需求或验收指标 |
| “课表 → 空教室 → 自定义日程”作为首期主线 | 可以继续讨论的实验场景，尚未确定为首期唯一范围 |
| 旧机器人继续服务，新入口试用，逐项迁移 | 一种迁移建议；用户明确的是功能不能丢，具体路径尚未定案 |
| 直接复用现成内核，或从零编写内核 | 两种方向都有顾虑，需要寻找合适的边界 |
| 此前的后台站点 | 交互预览，不代表真实 Agent、QQ 收发或业务后端已经接通；不默认成为正式后台设计 |

上一版方案涵盖运行时、工具、后台、存储和迁移流程。用户最新评价是仍然偏重，且整体设计不够满意，因此暂不以该草案驱动实施。

## 4. 接下来需要想清楚的问题

1. **轻量具体指什么？** 最在意部署占用、依赖数量、代码可理解性、维护成本，还是产品使用起来简单？哪些是主要目标？
2. **最想亲手理解和设计哪一层？** Agent 循环、上下文、工具接口、记忆，还是自然对话与产品体验？
3. **复用边界怎样划分？** 直接调用一个小组件、使用 SDK、参考设计自行实现，分别在哪些地方合适？不必预设全盘照搬或全盘自研。
4. **现有业务怎么带过去？** 哪些代码能独立复用，哪些仍绑定 LangBot 宿主、存储、确认和消息发送？如何避免为了换内核重写所有校园功能？
5. **最小实验应该证明什么？** 用哪一两段真实对话，才能判断新方向更自然、更容易掌握，而不是只多了一套架构？
6. **拟人和可定制怎样具体判断？** 先收集喜欢与不喜欢的回复示例，再确定行为和配置方式。
7. **后台现在需要到哪一步？** 是能看到一次工具调用过程就够，还是确实需要完整的运营界面？
8. **怎样兼顾小实验和功能完整？** 试验范围可以小，最终替换时功能不能丢；两者之间的衔接需要另行确定。
9. **最终放在哪里？** 暂时放在 Kit 开发仓，还是在边界清楚后拆成独立 HenuBot 仓库？本次目录选择不决定永久代码结构。

不要求一次回答所有问题。后续先挑最影响方向的一两项讨论，再决定是否做一个很小的实验。

## 5. 本目录怎么用

本目录目前只保存 Bot 相关想法和讨论记录。新的反馈直接补充到这里；形成明确决定后，再单独记录决定与理由。

本次先放在 `HENU-Kit-DEV/docs/bot/`，方便和 Kit 的校园业务、账号绑定以及现有项目资料一起查阅。暂不建立代码骨架、服务、依赖或部署配置。以后独立成仓库仍然可选。

后续参与讨论或开发时，应区分三种内容：用户已经表达的目标、助手提出的候选方案、已经明确接受的决定。不要把上一版提案自动升级为已接受架构。

## 6. 已有资料与学习入口

### 现有项目和仓库约定

- [HENU_Assistant](https://github.com/jry21223/HENU_Assistant)：现有机器人能力与插件实现。
- [HENU Kit 项目说明](../../README.md)：现有产品与仓库分工。
- [领域上下文索引](../../CONTEXT-MAP.md)：Kit 各模块的现有边界。
- [QQ Bot 账号绑定 ADR](../adr/0046-qq-bot-account-binding.md)：仓库已经接受的绑定归属与身份规则；本文不修改该决策。后续接入 Kit 时需要对齐。

### Agent 设计阅读入口

以下是已有调研的续读入口，学习主题不代表最终选型。项目文档和默认分支会变化，实际采用时重新核实稳定版本。

| 方向 | 阅读入口 |
| --- | --- |
| 最小 Agent 循环 | [mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent) |
| 内核、上下文与事件边界 | [Pi Agent Core](https://github.com/earendil-works/pi/blob/main/packages/agent/README.md) |
| 长期记忆和技能 | [Hermes Memory](https://hermes-agent.nousresearch.com/docs/user-guide/features/memory)、[Skills](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills) |
| 暂停、恢复和外部操作 | [LangGraph Interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)、[Fault tolerance](https://docs.langchain.com/oss/python/langgraph/fault-tolerance) |
| 聊天入口与运行时的分工 | [nanobot Architecture](https://github.com/HKUDS/nanobot/blob/main/docs/architecture.md) |
| 用户中途修改要求 | [PicoClaw Steering](https://github.com/sipeed/picoclaw/blob/main/docs/architecture/steering.md) |
| 会话与运行生命周期 | [OpenClaw Agent Loop](https://docs.openclaw.ai/concepts/agent-loop) |
| Python 应用的工具与运行上下文 | [PydanticAI Dependencies](https://pydantic.dev/docs/ai/core-concepts/dependencies/)、[OpenAI Agents SDK Context](https://openai.github.io/openai-agents-python/context/) |
| 长任务与上下文管理 | [Claude Code](https://code.claude.com/docs/en/how-claude-code-works)、[Deep Agents](https://docs.langchain.com/oss/python/deepagents/overview) |

对话中提到的“PC Partners”尚未可靠定位到具体项目，待补充链接或准确名称，不猜测替代。

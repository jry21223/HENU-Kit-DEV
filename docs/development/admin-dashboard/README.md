# HENU Kit 统一管理后台产品与交付总纲 V1.0

> 状态：Canonical / 执行中  
> Owner：项目负责人  
> 适用团队：1–2 名开发 + 1–2 名测试  
> 前端基线：Vue 3 + TypeScript + Vite + **shadcn-vue**  
> API 基线：[`../api-communication-spec.md`](../api-communication-spec.md)  
> 架构基线：Platform Core 聚合平台能力；Study、QuizCraft、Food、Notice 各自拥有业务数据。

## 1. 文档性质

本目录不是单纯 PRD，也不是单纯 Roadmap，而是统一管理后台的**产品与交付规范包**，同时承担：

- 产品需求基线：后台服务谁、解决什么问题、页面必须展示什么。
- 架构与数据边界：每类数据由哪个服务持有，后台如何聚合。
- API 契约基线：浏览器接口、内部服务接口、事件、幂等与错误处理。
- UI 实施规范：所有新页面使用 shadcn-vue；旧 Element Plus 页面渐进迁移。
- Roadmap：阶段、依赖、工时、停止线、验收条件。
- 测试基线：正常、失败、重复、并发、越权、依赖故障与回滚。

任何 AI、开发者或测试人员开始该 Epic 前，必须先阅读本目录。实现与本文冲突时，不得自行“按经验优化”，必须先更新本文或补 ADR。

## 2. 文档目录

| 文件 | 用途 |
|---|---|
| [`product-requirements.md`](./product-requirements.md) | 用户角色、后台导航、页面、数据和核心流程 |
| [`data-api-contract.md`](./data-api-contract.md) | 数据 Owner、指标口径、REST API、HMAC、事件和兼容策略 |
| [`ui-shadcn-vue-spec.md`](./ui-shadcn-vue-spec.md) | shadcn-vue 组件、页面布局、状态、响应式和迁移规则 |
| [`delivery-roadmap.md`](./delivery-roadmap.md) | 阶段排期、Issue 切分、依赖、交付物、风险与回滚 |
| [`testing-acceptance.md`](./testing-acceptance.md) | 测试矩阵、验收口径、Definition of Done |
| [`decision-log.md`](./decision-log.md) | 已冻结决策和后续变更记录 |

## 3. 不可漂移的冻结决策

以下内容在 V1 首发前视为冻结：

1. **继续使用 Vue 3 管理后台，不重写为 React。**
2. **所有新页面与新布局使用 shadcn-vue。** Element Plus 仅用于仍未迁移的旧页面；禁止在同一新页面混用两套组件。
3. 管理后台是统一入口，但不是共享数据库客户端。各服务通过 API/事件提供数据。
4. 新 API 严格遵守 `docs/development/api-communication-spec.md`：
   - `/api/v1` 与 `/api/v1/internal`；
   - 复数资源 + kebab-case 路径；
   - snake_case JSON；
   - UTC RFC 3339；
   - UUIDv4；
   - `request_id`；
   - POST/PATCH 幂等；
   - 服务 HMAC。
5. 平台反馈和 QuizCraft 题目反馈是两个数据域；统一待办只保存引用。
6. Food 是独立业务域。美食榜单不是五星评分：
   - 五档：夯、人上人、顶级、NPC、拉完了；
   - 投稿人提出初始档位；
   - 社区只判断“被低估了 / 差不多 / 被高估了”；
   - 调档必须按校准轮次进行，旧轮次投票不得继续推动新档位。
7. 学院归属与通知订阅分离。用户自报学院可用于通知分发，不可直接用于权限提升。
8. 校园通知内容保留来源、原发布时间、原链接和不可变版本；更新不得覆盖历史正文。
9. 校园通知邮件默认不订阅；用户主动订阅后才分发，退订立即生效。
10. 邮件分为 `critical`、`transactional`、`digest` 三类队列，Critical 不得被 Digest 阻塞。
11. 自建邮件系统通过 Provider Adapter 接入；业务代码不得直接调用 SMTP/Postal。
12. V1 的美食调档采用“系统建议 + 管理员确认”，不自动无审核调档。
13. 首发不做自定义报表、拖拽大屏、AI 自动审核、AI 自动调档、文字评论和商家认领。
14. 总览固定保留用户、校园通知、邮件、反馈、美食、系统六张业务卡；灰度期未接入域显示 `not_integrated` 和 `—`，正式 V1 六域必须全部接入真实数据。
15. 校园通知 V1 只支持人工表单与 UTF-8 `campus-notice-import/1.0` JSONL 导入；不实现自动抓取、网页解析、QQ 空间同步或 OCR。
16. 通知附件和美食图片统一使用 S3 兼容对象存储；本地与 CI 使用 MinIO，浏览器不得接触存储 Secret。
17. 通用 SMTP Adapter 是 V1 邮件 Provider；SMTP 接受只记为 `accepted`，没有 DSN 或回调证据不得记为 `delivered`。
18. 统一待办只使用两档 SLA：`urgent=24h`、`normal=72h`；只有未解决且超过 `due_at` 才记为 overdue。
19. 美食试运营 Policy 固定为至少 10 名有效参与者、70% 候选阈值、调档后 7 天冷却；存在阻断异常时禁止调档。
20. 新认证迁移不属于本 Epic。现有 RS256 管理员 Token 通过适配器接入；未来只替换验证器，不改变 Admin API 与前端。

## 4. 文档与实现优先级

发生冲突时按以下优先级处理：

1. 已合并并经 CI/生产验证的 OpenAPI、Migration、代码和测试；
2. `docs/development/api-communication-spec.md`；
3. 本管理后台规范包；
4. `docs/architecture/` 与已接受 ADR；
5. 历史 README、旧后台页面或归档规划。

若第 1 层实现与第 2–3 层规范冲突，应立即建立修复 Issue；不能把已存在的错误实现当成新规范。

## 5. 防偏离执行机制

### 5.1 每个 PR 必须声明

- 对应阶段和 Issue ID；
- 修改的页面/API/数据表；
- 是否修改本目录或 OpenAPI；
- 本 PR 明确不做什么；
- 正常、失败、重复、并发、越权和依赖故障测试；
- 回滚方式。

### 5.2 范围变更

以下变化必须先更新 `decision-log.md`，必要时补 ADR：

- 改用 React 或放弃 shadcn-vue；
- 新增业务域或跨库读取；
- 改变美食五档、校准语义或自动调档规则；
- 改变学院与订阅关系；
- 修改 API Envelope、HMAC、幂等、事件结构；
- 新增高风险批量操作；
- 把首发外功能加入关键路径。

### 5.3 Review Gate

任何管理后台功能进入编码前必须同时满足：

- 页面字段和状态已在 `product-requirements.md` 定义；
- 数据 Owner 与指标口径已定义；
- OpenAPI/事件 Schema 已更新或明确使用旧 Adapter；
- 权限 Scope 已定义；
- 测试和回滚已定义。

## 6. 首发成功标准

管理员进入后台后应在 30 秒内明确：

1. 平台是否健康；
2. 今天必须处理什么；
3. 用户、通知、邮件、反馈、美食投稿与校准是否存在异常；
4. 点击任一异常可进入带筛选条件的处理页；
5. 所有高风险操作有权限、幂等、审计和回滚证据。

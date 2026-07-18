# 测试计划与验收规范

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 测试原则

- 测试从 Phase 0 开始，不在开发结束后集中进行。
- 开发负责单元测试；测试人员负责独立用例、契约、集成、E2E、并发、滥用、回归和发布验收。
- 安全关键功能的 Owner 不得独立签字。
- 每个 Bug 必须有稳定复现步骤；修复后优先补自动化回归。
- 任何“隐藏/冻结”功能仍需保留数据和安全回归，直到正式删除。

## 2. 测试层级

| 层级 | 目标 | 运行时机 |
|---|---|---|
| Unit | 业务规则、状态机、错误码、边界值 | 每次提交/PR |
| Contract | OpenAPI、Event Schema、Adapter 兼容 | PR；共享契约改动时 Fan-out |
| Integration | PostgreSQL、Redis、Migration、Outbox、Worker | PR 和 Staging |
| E2E | Portal/Study/Quiz 登录、导航、资料、刷题和退出 | 每阶段、发布前 |
| Concurrency/Idempotency | 重放、并发、锁、唯一约束、重复消息 | 安全关键 PR、切流前 |
| Security Abuse | 越权、伪造身份、Callback、PKCE、Secret 泄露 | Phase 2 起持续 |
| Performance | Auth、Notification、Quiz Read/Write P95/P99 | Phase 4–6 |
| Recovery | Worker 重启、Redis/PG 故障、备份恢复、Blue/Green | Phase 3、Phase 6 |

## 3. 测试环境

- CI：临时 PostgreSQL/Redis、固定时区 UTC、Fake DirectMail、固定随机种子。
- Staging：与生产相同拓扑和最小权限；使用脱敏规模数据或生成数据。
- 浏览器：Chromium、Firefox、WebKit；移动端至少 360px 和 390px。
- Mock：Platform OpenAPI Mock、Quiz Legacy Fake、DirectMail Fake、学校页面 Fixtures、Deploy Dry-run。
- 测试数据必须可创建、隔离和清理；不得使用真实学生敏感数据作为常规 Fixture。

## 4. Phase 0–1 验收

- PR #10 导入 Hash、License、Secret 和大文件检查。
- Study Go/Node/Compose 与 Quiz Pytest/Smoke/Lint/Build 基线。
- Path Filter 至少 8 组路径矩阵。
- Core/Worker 独立启动，Health/Readiness 正确区分。
- OpenAPI Lint、生成代码和 Mock 可消费。
- Migration 空库/已有库 Up，必要 Down 和重复执行。
- Portal 360px、键盘、链接和非官方声明。

## 5. 账号与权限关键用例

| ID | 场景 | 期望 |
|---|---|---|
| AUTH-001 | 合法学生邮箱请求验证码 | 202 + `request_id`；Code/Outbox 各一条；日志掩码 |
| AUTH-002 | 过期 Code 验证 | 400；不创建 Session |
| AUTH-003 | 同一正确 Code 20 并发 | 仅一个成功，单个 Session/Used 标记 |
| AUTH-004 | 同邮箱/IP 高频请求 | 429；无新增 Code/Outbox |
| AUTH-005 | 未登记 Callback | 安全错误页，不重定向 |
| AUTH-006 | State 篡改或重放 | 业务站 400，无本地 Session |
| AUTH-007 | 同一授权码并发交换 | 仅一个成功，Code 消费一次 |
| AUTH-008 | 错误 PKCE Verifier | 400，日志不记录 Verifier |
| AUTH-009 | 全局退出 | Core 与业务站 Session 最终全部撤销 |
| AUTHZ-001 | Guest 访问保护资源 | 401/403，Core 不创建 User |
| AUTHZ-002 | Free/VIP 访问高级内容 | Free 拒绝，VIP 成功，Role 不变化 |
| AUTHZ-003 | Reviewer(course A) 审核 A/B | A 成功，B 拒绝 |
| AUTHZ-004 | VIP 调用审核 API | 403，不发生提权 |
| AUTHZ-005 | Body 自报 Admin/VIP/User ID | 服务端忽略并按 Session 判定 |

## 6. 事件、Worker、邮件和通知

| ID | 场景 | 期望 |
|---|---|---|
| SVC-001 | 正确 HMAC 请求 | 事件接收，日志含 Service/Key ID |
| SVC-002 | 重放 Nonce | 401/409，无新增数据 |
| EVT-001 | 同一 Event 两次投递 | Event/Outbox/通知只一份 |
| EVT-002 | 业务 + Outbox 提交失败注入 | 两者均不提交 |
| WRK-001 | Worker 处理中 `kill -9` | XAUTOCLAIM 后只完成一次 |
| WRK-002 | Redis 暂停 | DB Outbox 保留；恢复后重放 |
| MAIL-001 | DirectMail Timeout | API 仍 202；Delivery 进入 Retry Due |
| MAIL-002 | 永久拒收 | Failed + DLQ；用户只见安全提示 |
| NTF-001 | A 读取/已读 B 通知 | 404/403；B 状态不变 |
| PTS-001 | 积分事件重复 | Ledger 一条、余额增加一次 |

## 7. QuizCraft 关键用例

| ID | 场景 | 期望 |
|---|---|---|
| LINK-001 | 签名匿名 Session 绑定 | Link 一条，统计合并一次 |
| LINK-002 | 自报旧匿名 ID、无持有证明 | 拒绝，无 Link |
| LINK-003 | Legacy ID 已绑定他人 | 409，原 Link 不变 |
| QUIZ-001 | Legacy/Adapter 影子读取 | 客户端返回 Legacy；内部记录差异 |
| QUIZ-002 | 单选/多选/判断/填空提交 | 判分一致，Answer 和 Stats 各一次 |
| QUIZ-003 | 重复 Idempotency Key | 返回首次结果，不重复统计 |
| QUIZ-004 | 100 并发答题 | 无重复、无未解释 5xx，统计可重算 |
| QUIZ-005 | 排行榜 14 天影子对比 | Top100 一致或差异已签字 |

## 8. Study、Portal 与迁移

| ID | 场景 | 期望 |
|---|---|---|
| LIB-001 | 唯一已验证旧邮箱绑定 | Link 一条；历史 FK 不变 |
| LIB-002 | 同邮箱多个旧账号 | 409 + Manual Review，不自动合并 |
| UI-001 | 360/390px 核心流程 | 无 5xx、遮挡和不可操作控件 |
| UI-002 | 三浏览器跨站登录/退出 | Session、回跳和撤销正确 |
| MOVE-001 | 新旧路径构建 | Artifact Hash/行为等价，可用 Alias 回滚 |
| OPS-001 | 备份恢复到空环境 | 计数/Hash 一致，Readiness/Smoke 通过 |
| OPS-002 | 新版本 Readiness 故障 | 停止放量并自动/一键回旧 |

## 9. 切流门禁

| 能力 | 放量门槛 | 回滚触发 |
|---|---|---|
| 统一登录 | 测试站成功率 ≥99%；重放 100% 阻断；无敏感日志 | 比基线下降 >5 个百分点或 5xx >2%/5min |
| Critical Mail | P95 排队 <30s；真实样本送达报告 | 最老任务 >5min、连续拒收或凭据错误 |
| Quiz 读取 | 字段一致率 ≥99.99%，未知差异为 0 | 未知差异 >0.01% 或 P95 >1.5× Legacy |
| Quiz 作答 | 判分 100% 一致；统计差异 ≤0.1% 且可重算 | 任一判分差异、重复计数、写失败 >0.5% |
| 排行榜 | Top100 连续 14 天一致 | 任一未解释差异 |
| Study 账号 | 下载/投稿历史抽样 100% 正确 | 冲突率超阈值或隔离测试失败 |

## 10. 缺陷等级

| 等级 | 定义 | 处理 |
|---|---|---|
| Blocker | 数据丢失、账号接管、权限绕过、重复扣/加、无法回滚 | 立即停止发布和切流 |
| Critical | 核心流程不可用、持续 5xx、敏感日志、Migration 风险 | 当前阶段不得退出 |
| Major | 主要功能错误但有安全替代路径 | 发布前修复或负责人书面接受 |
| Minor | 文案、非阻断 UI、低频兼容问题 | 可进入明确版本 Backlog |

## 11. 阶段测试报告模板

```text
阶段：
Build SHA / 环境：
范围与排除项：
通过用例 / 失败用例：
自动化覆盖：
数据对账：
安全扫描：
性能指标：
已知问题与风险接受人：
回滚演练结果：
测试结论：通过 / 有条件通过 / 阻断
测试签字：
项目负责人批准：
```

# 统一管理后台交付 Roadmap V1.0

## 1. 投入与周期

### 推荐配置

- 开发 A：Platform Core/BFF、用户、邮件、通知、契约。
- 开发 B：shadcn-vue、Dashboard、Food、Quiz/Study Adapter。
- 测试 A：API、Migration、并发、幂等、安全、数据一致性。
- 测试 B：浏览器、移动端、审核流程、灰度与回滚。

推荐周期：**16–18 周**，包含不少于 20% 集成缓冲。

### 最小配置

1 名开发 + 1 名测试：**24–28 周**。必须按阶段串行，不允许同时展开 Notice、Mail、Food 三条未闭环链路。

## 2. 关键路径

```text
计划与契约冻结
→ shadcn-vue Foundation
→ Platform Core Admin Module/BFF
→ 总览与用户
→ 邮件闭环
→ 通知人工录入、JSONL 导入与分发
→ 两级反馈
→ 美食投稿与校准
→ 系统/审计
→ 旧后台迁移与发布
```

OpenAPI、数据口径和权限必须先于页面实现。页面可以使用 Mock 提前开发，但不得以 Mock 字段反向定义生产 API。

## 3. Phase 0：计划与契约冻结（W1）

### 目标

建立不可漂移的产品、数据、API、UI、测试和决策基线。合并顺序固定为 PR #12 → PR #19 → Admin OpenAPI/Mock PR。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-001 | 合并本规范包并建立文档索引 | 0.5d | 0.25d | 无 |
| ADMIN-002 | 修正 Platform Core OpenAPI 与开发文档 03 的冲突 | 2d | 1d | ADMIN-001 |
| ADMIN-003 | 新增 Admin API OpenAPI 3.1 骨架、公共 Schema 和 Mock | 2d | 1d | ADMIN-002 |
| ADMIN-004 | 增加 organization Scope 与权限码 | 1d | 1d | ADMIN-002 |
| ADMIN-005 | 建立指标字典与 Dashboard Fixture | 1d | 1d | ADMIN-003 |

### 退出条件

- 新旧契约冲突有明确处理；
- Admin OpenAPI Lint 通过；
- Mock 可返回总览、列表、部分失败、错误和分页；
- 权限矩阵评审完成；
- 任何 UI 开发不再自行创造字段。

## 4. Phase 1：shadcn-vue Foundation（W2–W3）

### 目标

建立新 Admin Shell 和统一组件，不改业务数据。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-006 | 初始化 Tailwind v4、shadcn-vue、Alias、Theme 和锁文件 | 2d | 1d | ADMIN-001 |
| ADMIN-007 | 实现 Sidebar/Header/PageHeader/StatusBadge/MetricCard | 2d | 1d | ADMIN-006 |
| ADMIN-008 | 实现 DataTable、Toolbar、分页和 URL 筛选 | 3d | 1.5d | ADMIN-006 |
| ADMIN-009 | 实现 Loading/Empty/Error/PartialFailure/AuditTimeline | 2d | 1d | ADMIN-006 |
| ADMIN-010 | 新旧 Shell 路由兼容与 Feature Flag | 1d | 1d | ADMIN-007 |

### 退出条件

- 新页面无 Element Plus import；
- 桌面折叠、移动 Sheet、键盘导航通过；
- 360/390px Smoke 通过；
- 旧页面仍可访问；
- `npm run build:admin` 通过。

## 5. Phase 2：Admin BFF、总览与用户（W3–W5）

### 目标

管理员能登录新 Shell，看到真实用户数据、部分服务状态和可执行待办。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-011 | Platform Core Admin Module、权限、Envelope、request_id | 3d | 2d | ADMIN-003/004 |
| ADMIN-012 | 内部 admin-summaries/action-items 协议与 HMAC Client | 3d | 2d | ADMIN-011 |
| ADMIN-013 | 用户学业档案 Migration 和 API | 3d | 2d | ADMIN-011 |
| ADMIN-014 | 通知订阅 Migration 和 API | 3d | 2d | ADMIN-013 |
| ADMIN-015 | Dashboard 六卡、待办、部分失败和趋势页面 | 4d | 2d | ADMIN-005/007/011 |
| ADMIN-016 | 用户列表、详情、学业档案、Session 撤销 | 4d | 2d | ADMIN-008/013 |

### 退出条件

- 用户、学院完成率和 DAU 口径明确；
- BFF 任一业务服务失败时返回 partial；
- 完整邮箱查看受权限和审计控制；
- 用户冻结/Session 撤销有幂等和并发测试；
- 总览待办可跳转到带筛选页面。

## 6. Phase 3：自建邮件运营闭环（W5–W7）

### 目标

完成 Mail Provider Adapter、投递状态、重试、抑制和后台操作；先使用 Fake/测试 SMTP，再灰度真实自建服务。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-017 | Mail Provider Adapter 与 Fake Provider | 2d | 1d | ADMIN-011 |
| ADMIN-018 | email_deliveries、suppressions、dead_letters Migration/API | 4d | 2d | ADMIN-017 |
| ADMIN-019 | Critical/Transactional/Digest 独立队列与 Worker | 4d | 3d | ADMIN-018 |
| ADMIN-020 | 自建邮件 Provider 接入、Webhook/DSN 分类 | 3d | 3d | ADMIN-019 |
| ADMIN-021 | 邮件概览、投递、队列、死信、抑制页面 | 4d | 2d | ADMIN-008/018 |
| ADMIN-022 | SPF/DKIM/DMARC/PTR 与发送域健康检查 | 2d | 1d | ADMIN-020 |

### 退出条件

- Critical 不被 Digest 阻塞；
- Worker 重启可 reclaim，重复任务不重复发送；
- accepted 与 delivered 语义分开；
- 真实测试邮箱送达报告完成；
- 死信重放、抑制和敏感信息脱敏通过；
- Provider 可通过配置切回 Fake/备用实现。

## 7. Phase 4：校园通知导入与分发（W7–W10）

### 目标

支持人工表单、JSONL 批量导入、不可变版本、人工审核、主动订阅与邮件/站内分发。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-023 | Notice Service 骨架、独立凭据与版本化 Migration | 3d | 2d | ADMIN-011 |
| ADMIN-024 | 单条表单、JSONL 导入任务和逐行结果 | 4d | 3d | ADMIN-023 |
| ADMIN-025 | Notice/Version/Audience 与 S3 对象键追溯 | 4d | 3d | ADMIN-024 |
| ADMIN-026 | 审核、驳回、版本 Diff 和目标受众页面 | 4d | 2d | ADMIN-025/008 |
| ADMIN-027 | Distribution 计划、订阅匹配、去重和取消 | 4d | 3d | ADMIN-014/019/025 |
| ADMIN-028 | 导入任务、分发任务和异常页面 | 3d | 2d | ADMIN-026/027 |

### 退出条件

- 表单与 JSONL 共用 Upsert，逐行结果可定位；
- 版本更新不覆盖历史；
- 原来源、时间和链接完整；
- 未主动订阅用户不收到邮件；
- 退订立即生效；
- `(user_id, notice_id, channel, window)` 唯一防重复；
- 自动抓取、QQ 空间同步、网页解析器和 OCR 不进入 V1。

## 8. Phase 5：反馈中心（W9–W11）

### 目标

建立平台反馈、题目反馈和统一待办，不混合数据 Owner。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-029 | Platform Feedback/Operation Case Migration/API | 3d | 2d | ADMIN-011 |
| ADMIN-030 | 平台反馈列表、详情、分配和处理页面 | 3d | 2d | ADMIN-008/029 |
| ADMIN-031 | Quiz 题目反馈 Adapter、快照和状态扩展 | 4d | 3d | ADMIN-012 |
| ADMIN-032 | 题目反馈页面和双源修复 Gate | 3d | 2d | ADMIN-031 |
| ADMIN-033 | 统一待办、SLA 和超时聚合 | 2d | 2d | ADMIN-029/031 |

### 退出条件

- 平台反馈与题目反馈完全分域；
- 统一待办只保存引用；
- 题目反馈必须 JSON、PostgreSQL、运行时均验证后 resolved；
- 越权、重复分配和并发处理有测试。

## 9. Phase 6：美食榜单（W10–W14）

### 目标

完成投稿定档、社区校准、轮次、调档建议、异常票和后台处理。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-034 | Food Service 骨架、Tier Definition 与 Migration | 3d | 2d | ADMIN-002 |
| ADMIN-035 | Food Submission/Entry/API 与审核事务 | 4d | 3d | ADMIN-034 |
| ADMIN-036 | Calibration Round/Vote/API 与唯一约束 | 4d | 3d | ADMIN-035 |
| ADMIN-037 | Calibration Policy v1 与调档候选 | 3d | 3d | ADMIN-036 |
| ADMIN-038 | 调档事务、历史、新轮次和 Outbox | 3d | 3d | ADMIN-037 |
| ADMIN-039 | 投稿、条目、校准、调档页面 | 5d | 3d | ADMIN-008/035/038 |
| ADMIN-040 | 异常票规则、作废/恢复与审计 | 4d | 3d | ADMIN-036 |

### 退出条件

- 前端不硬编码五档顺序；
- 投票只有被低估/差不多/被高估；
- 一个用户每轮一个有效判断；
- 调档后旧轮次关闭，新轮次建立；
- 每次最多调整一档；
- suspected/invalidated 票不进入共识；
- Policy V1 固定为 10 人、70%、7 天冷却；
- 首发仅系统建议 + 管理员确认；
- 审核、调档和票作废均可审计和回滚。

## 10. Phase 7：系统、审计、旧后台迁移与发布（W14–W16）

### 目标

补齐系统可观测性，迁移关键旧页面，完成灰度和回滚。

### 任务

| ID | 任务 | 开发 | 测试 | 依赖 |
|---|---|---:|---:|---|
| ADMIN-041 | 服务/Worker/部署/数据任务页面 | 3d | 2d | ADMIN-012 |
| ADMIN-042 | 统一审计查询、权限和导出限制 | 3d | 3d | 各写流程 |
| ADMIN-043 | Study 用户/资料/下载关键页面迁移 shadcn-vue | 5d | 3d | ADMIN-006/008 |
| ADMIN-044 | 旧 Element Plus 页面清单与迁移 Feature Flag | 2d | 2d | ADMIN-043 |
| ADMIN-045 | Staging 全链路、备份恢复、灰度和回滚演练 | 2d | 5d | 全部 |

### 退出条件

- 所有 P0/P1 页面使用 shadcn-vue；
- 服务异常可追踪 request_id；
- 高风险操作全部进入审计；
- staging E2E、恢复和回滚通过；
- 生产灰度无阻断指标；
- Element Plus 删除另开独立 PR，不与生产切流混合。

## 11. PR 切分规则

- 一个 PR 只解决一个业务/基础设施问题。
- OpenAPI + Mock 可以一个 PR；Migration + Repository 可以一个 PR；页面与 API 实现尽量分开。
- 禁止在同一 PR 同时：
  - 初始化 shadcn-vue并迁移全部旧页面；
  - 修改 API 契约和生产数据；
  - 部署自建邮件和切换验证码全量流量；
  - 新建 Food 数据模型和自动调档；
  - 目录移动和业务行为变化。

## 12. 风险与停止线

| 风险 | 停止线 | 处理 |
|---|---|---|
| OpenAPI 与实现继续分裂 | 新页面出现未定义字段 | 停止页面开发，先修契约 |
| shadcn/Element 混用失控 | 新页面出现 Element import | CI 阻断 |
| BFF 跨库直读 | 新增业务 DB 凭据 | 拒绝合并，改内部 API/事件 |
| 邮件信誉不稳定 | Critical oldest age >5m 或连续凭据失败 | 停止放量，切回 Provider/Fake |
| 通知误发 | 未订阅用户被匹配 | 停止分发并审计 |
| 美食刷票 | 异常票未处理仍触发调档 | blocked_by_risk，不允许调档 |
| 题目反馈假修复 | 只修 JSON 或只修 PG | 不得 resolved |
| 权限越界 | Scope 绕过或浏览器自报有效 | 阻断发布 |

## 13. 发布顺序

```text
契约与 Mock
→ UI Foundation（Feature Flag）
→ Admin BFF
→ 用户/总览
→ 邮件测试与灰度
→ 通知小范围学院试点
→ 反馈
→ Food 内测
→ 系统/审计
→ 生产灰度
```

每个阶段均必须有单独回滚开关，不能等待整套后台全部完成后一次上线。

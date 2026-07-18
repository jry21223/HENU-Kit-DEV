# 统一管理后台测试与验收规范 V1.0

## 1. 测试原则

测试从契约与 Migration 阶段开始，不等待页面完成。

每个写流程至少覆盖：

- 正常；
- 参数错误；
- 未登录；
- 无权限/越 Scope；
- 相同幂等键重试；
- 相同幂等键不同请求体；
- 并发；
- PostgreSQL/Redis/下游不可用；
- request_id 透传；
- 日志与响应脱敏；
- 回滚。

## 2. 分层测试

| 层级 | 内容 |
|---|---|
| Contract | OpenAPI 3.1、JSON Schema、生成类型、Mock 与实现一致 |
| Unit | Policy、状态机、权限、指标、脱敏、排序白名单 |
| Migration | 空库、已有库、重复执行、Up/Down、唯一约束 |
| Integration | PostgreSQL、Redis、HMAC、Outbox、Worker、Adapter |
| Component | shadcn-vue 组件状态、表格、Dialog、Sheet、表单 |
| E2E | 管理员关键任务、移动端、跨页面筛选、回滚 |
| Security | 越权、伪造身份、重放、敏感字段、批量操作 |
| Resilience | 部分服务失败、Worker 重启、队列积压、Provider 超时 |

## 3. UI Foundation

| ID | 场景 | 预期 |
|---|---|---|
| UI-001 | 360/390px 打开后台 | Sidebar 使用 Sheet，内容无阻断溢出 |
| UI-002 | 键盘遍历导航和 Dialog | Focus 顺序正确，可关闭并恢复焦点 |
| UI-003 | 新页面扫描 Element Plus import | 结果为 0 |
| UI-004 | Dashboard 一个服务失败 | 其他模块显示，失败模块显示 PartialFailure |
| UI-005 | 列表加载/空/错误 | 三种状态明确且可重试 |
| UI-006 | 总览点击待办 | 跳转到带 Query 筛选的正确页面 |
| UI-007 | 高风险操作 | 使用 AlertDialog，提交期间不可重复触发 |

## 4. 用户与权限

| ID | 场景 | 预期 |
|---|---|---|
| USER-001 | Admin 查看用户 | 邮箱默认脱敏 |
| USER-002 | 无 reveal 权限请求完整邮箱 | 403/404，写安全审计 |
| USER-003 | 有 reveal 权限查看完整邮箱 | 成功并写审计 |
| USER-004 | 浏览器自报 admin/college_scope | 服务端忽略 |
| USER-005 | Reviewer 访问其他 Scope | 403/404，无数据泄漏 |
| USER-006 | 并发冻结/解冻 | 版本控制，只有合法状态成功 |
| USER-007 | 重复 Session 撤销 | 幂等，最终均 revoked |
| USER-008 | 自报学院 | 可用于受众，不产生权限角色 |
| USER-009 | 学院/专业不匹配 | 返回可处理异常，不自动提权或覆盖 |

## 5. Dashboard 与聚合

| ID | 场景 | 预期 |
|---|---|---|
| DASH-001 | 所有服务正常 | `status=ok`，数据时间完整 |
| DASH-002 | Notice 超时 | 200 + `status=partial` + last_success_at |
| DASH-003 | BFF 认证失败 | 401，不返回任何摘要 |
| DASH-004 | 下游返回过期数据 | 显示 stale，不伪装实时 |
| DASH-005 | 同一时间筛选 | 各服务使用同一 UTC 边界 |
| DASH-006 | DAU 口径 | 仅成功受保护操作，PV 不计入 |

## 6. 邮件

| ID | 场景 | 预期 |
|---|---|---|
| MAIL-001 | 发送 Critical | 进入 critical 队列，Digest 积压不阻塞 |
| MAIL-002 | Provider accepted | 状态 accepted，不显示 delivered |
| MAIL-003 | 明确 DSN delivered | 状态 delivered |
| MAIL-004 | 临时失败 | retry_due，指数退避 + jitter |
| MAIL-005 | 永久失败 | failed/dead_letter，不无限重试 |
| MAIL-006 | Worker kill -9 | reclaim 后仅发送一次或按 Provider 幂等查询 |
| MAIL-007 | DB 更新失败 | 不 ACK，恢复后重领 |
| MAIL-008 | 重复 retry/replay | 幂等，不重复生成业务副作用 |
| MAIL-009 | 抑制名单用户 | 不发送，记录 skipped 原因 |
| MAIL-010 | 后台详情 | 无完整邮箱、验证码、Secret、完整第三方响应 |
| MAIL-011 | Redis 不可用 | 高风险 Critical 入口按规范 Fail Closed |
| MAIL-012 | Provider 切换 | 可切 Fake/备用实现，不改业务调用 |

## 7. 校园通知

| ID | 场景 | 预期 |
|---|---|---|
| NOTICE-001 | 白名单来源首次抓取 | 创建 notice + version + fetch log |
| NOTICE-002 | 304 | 只写 fetch 结果，不创建新版本 |
| NOTICE-003 | 内容 Hash 变化 | 新建不可变版本，旧版本保留 |
| NOTICE-004 | 页面结构变化 | review_pending/采集异常，不覆盖旧内容 |
| NOTICE-005 | 非白名单来源 | 拒绝抓取 |
| NOTICE-006 | 用户未订阅 | 不创建邮件任务 |
| NOTICE-007 | 用户退订 | 立即阻断后续任务 |
| NOTICE-008 | 重复分发 | 唯一约束，用户同一窗口只收到一次 |
| NOTICE-009 | 取消分发 | 未发送任务取消，已接受任务保留审计 |
| NOTICE-010 | 学院匹配 | 使用学业档案 + 主动订阅，结果可解释 |
| NOTICE-011 | 原文展示 | 包含来源、原发布时间、原链接和非官方声明 |

## 8. 反馈

### 8.1 平台反馈

| ID | 场景 | 预期 |
|---|---|---|
| PF-001 | 用户提交 | 创建反馈和统一待办引用 |
| PF-002 | 相同幂等键重复提交 | 返回首次反馈 |
| PF-003 | 分配负责人并发 | 版本控制，避免覆盖 |
| PF-004 | 超时计算 | 未解决且超过 due_at 才计入 |
| PF-005 | 关联 request_id | 能追踪日志但不泄露敏感内容 |

### 8.2 题目反馈

| ID | 场景 | 预期 |
|---|---|---|
| QF-001 | 创建反馈 | 保存题目/答案/解析/用户答案快照 |
| QF-002 | 只修 JSON | 不允许 resolved |
| QF-003 | 只修 PostgreSQL | 不允许 resolved |
| QF-004 | 两侧修复但未验证运行时 | 不允许 resolved |
| QF-005 | 三方验证完成 | 可以 resolved，记录 verified_at |
| QF-006 | 重复反馈 | 可 archive/合并，原记录保留 |
| QF-007 | 非授权题库 Reviewer | 无权处理 |

## 9. 美食榜单

| ID | 场景 | 预期 |
|---|---|---|
| FOOD-001 | 用户投稿 | 保存建议档位和理由，不直接上线 |
| FOOD-002 | 审核通过 | Entry、首轮 Calibration、Outbox 同事务 |
| FOOD-003 | 评分字段尝试 | API 不接受五星/普通喜欢票字段 |
| FOOD-004 | 用户首次校准 | 创建一张有效票 |
| FOOD-005 | 同轮修改判断 | 更新原票，不增加参与人数 |
| FOOD-006 | 同轮并发投票 | 唯一约束，只有一个有效结果 |
| FOOD-007 | suspected 票 | 不进入有效共识分母 |
| FOOD-008 | 样本不足 | `insufficient_votes`，不能调档 |
| FOOD-009 | 异常未处理 | `blocked_by_risk`，不能调档 |
| FOOD-010 | 升档确认 | 最多升一档，关闭旧轮、写历史、建新轮 |
| FOOD-011 | 降档确认 | 最多降一档，关闭旧轮、写历史、建新轮 |
| FOOD-012 | 旧轮投票 | 不影响新轮 Policy |
| FOOD-013 | 档位顺序变化 | 前端按定义表 sort_order，不硬编码 |
| FOOD-014 | 并发调档 | 版本/轮次校验，只有一个成功 |
| FOOD-015 | 作废/恢复票 | 状态可追溯，写审计并重算推荐 |
| FOOD-016 | 投稿人与社区偏差 | 仅分析，不自动惩罚投稿人 |

## 10. 系统与审计

| ID | 场景 | 预期 |
|---|---|---|
| OPS-001 | 服务 readiness 失败 | 总览显示降级和可操作待办 |
| OPS-002 | Outbox 积压 | 展示 oldest age 和数量 |
| OPS-003 | 审计查询越权 | 拒绝访问 |
| OPS-004 | 高风险操作 | actor、resource、before/after、request_id 完整 |
| OPS-005 | 敏感导出 | 权限、限量、审计、文件过期 |
| OPS-006 | 旧接口调用存在 | 不允许删除 Adapter |

## 11. 性能目标（首发基线）

在基线数据量下：

- Dashboard BFF P95 < 1.5s；
- 用户/邮件/反馈/美食列表 P95 < 800ms；
- 普通详情 P95 < 800ms；
- 高风险写操作不以追求低延迟牺牲事务与审计；
- Dashboard 单个下游超时不阻塞其他服务超过总体预算；
- 列表禁止无上限 page_size 和进程内全表聚合。

实际 SLO 必须在生产数据盘点后复核，不把本文数值解释为第三方送达保证。

## 12. Definition of Done

一个 Issue 完成必须满足：

- 产品字段/状态与本文一致；
- OpenAPI/Event Schema 已更新并 Lint；
- Migration Up/Down 和已有库测试；
- Unit/Integration/Contract 测试；
- 正常、失败、重复、并发、越权、依赖故障测试；
- request_id 全链路；
- 日志和响应脱敏；
- shadcn-vue 页面有 Loading/Empty/Error/Partial；
- 360/390px 和键盘 Smoke；
- 审计与权限测试；
- Feature Flag、监控和回滚说明；
- Reviewer 不是主要作者；
- 文档和生成代码一致。

## 13. 发布门禁

任何生产灰度前必须：

- P0/P1 测试全部通过；
- 无未解释的契约差异；
- 无 Critical/High 安全问题；
- 备份恢复完成；
- 旧页面/Provider/Adapter 回滚开关验证；
- 管理员真实任务演练完成；
- 测试负责人和项目负责人签字。

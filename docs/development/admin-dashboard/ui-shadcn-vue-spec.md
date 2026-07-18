# 统一管理后台 shadcn-vue UI 实施规范 V1.0

## 1. 技术基线

```text
Vue 3
TypeScript
Vite
Vue Router
Pinia
TanStack Query for Vue
TanStack Table
ECharts
shadcn-vue（Reka UI base）
Tailwind CSS v4
```

shadcn-vue 采用“组件源码进入仓库”的方式使用。新增组件必须通过 CLI 安装并进入 `apps/admin/src/components/ui/`，不允许在运行时依赖远程 Registry。

## 2. 迁移原则

当前后台使用 Element Plus。迁移采用渐进方式：

1. 新 Admin Shell 和所有新页面只使用 shadcn-vue。
2. 旧 Element Plus 页面可以暂时保留，但冻结新增 Element Plus 组件。
3. 一个页面不能混用 shadcn-vue 和 Element Plus；跨页面并存仅限迁移期。
4. 先迁 Shell、Dashboard 和通用表格，再迁低风险旧页面。
5. 登录页最后迁移，避免首个 PR 同时改认证流程和 UI 基础设施。
6. 删除 Element Plus 只能在所有页面迁移、构建和 E2E 通过后单独执行。

## 3. 初始工程配置

首个 UI Foundation PR 必须完成：

- Tailwind CSS v4 和 Vite 插件；
- `components.json`；
- `@/* -> ./src/*` Alias；
- `src/lib/utils.ts` 的 `cn` 工具；
- HENU Kit 主题 CSS Variables；
- shadcn-vue Button、Card、Badge、Alert、Skeleton、Separator；
- Sidebar、Sheet、Dropdown Menu、Tooltip；
- Table、Checkbox、Pagination、Select、Input；
- Dialog、Alert Dialog、Tabs、Popover、Calendar；
- Sonner；
- 对应依赖锁文件更新。

CLI 只用于生成/更新源码；生成后必须 Review 代码和依赖，不允许无审查执行 `add --all`。

## 4. 目录结构

```text
apps/admin/src/
├── components/
│   ├── ui/                    # shadcn-vue 生成组件
│   ├── admin/
│   │   ├── AdminSidebar.vue
│   │   ├── AdminHeader.vue
│   │   ├── PageHeader.vue
│   │   ├── MetricCard.vue
│   │   ├── StatusBadge.vue
│   │   ├── DataTable.vue
│   │   ├── DataTableToolbar.vue
│   │   ├── EmptyState.vue
│   │   ├── ErrorState.vue
│   │   ├── PartialFailureAlert.vue
│   │   └── AuditTimeline.vue
│   └── charts/
├── features/
│   ├── dashboard/
│   ├── users/
│   ├── notices/
│   ├── mail/
│   ├── feedback/
│   ├── food/
│   ├── study/
│   ├── quiz/
│   └── system/
├── lib/
│   ├── api/
│   ├── auth/
│   ├── permissions/
│   └── utils.ts
├── router/
├── stores/
└── styles/
    └── globals.css
```

业务页面不得直接从其他 Feature 深层导入内部组件；跨 Feature 复用进入 `components/admin` 或 `lib`。

## 5. 主题与视觉

### 5.1 品牌色

- Primary：Kit 墨绿 `#0C6B45`。
- 背景：纸白/浅灰，不使用大面积高饱和绿色。
- 警告、危险、成功使用语义 Token，不以业务代码硬编码色值。
- 所有状态必须同时通过文字/图标表达，不能只依赖颜色。

### 5.2 密度

后台采用中等偏紧凑密度：

- 桌面表格行高 44–48px；
- 页面最大内容宽度不强制固定，但图表和详情页应控制可读宽度；
- 卡片数量优先少而复合，不堆叠大量无动作 KPI；
- 首屏突出待办与异常，而不是装饰性图表。

### 5.3 圆角与阴影

- 普通 Card 使用轻边框、低阴影；
- Dialog/Sheet 使用统一层级；
- 不创建多套自定义圆角系统，统一消费 CSS Variables。

## 6. Admin Shell

### 6.1 桌面

- 左侧可折叠 Sidebar；
- 顶部 Header：面包屑、环境、全局搜索、通知、用户菜单；
- 内容区：Page Header、页面动作、主体；
- Sidebar 按业务域分组，不展示无权限入口。

### 6.2 移动端

- Sidebar 进入 Sheet；
- 表格优先横向滚动或卡片化关键列；
- 高风险操作不可因移动端隐藏确认步骤；
- 核心页面至少覆盖 360px 和 390px。

## 7. 页面骨架

### 7.1 Page Header

必须包含：

- 页面标题；
- 简短用途；
- 数据更新时间；
- 主操作；
- 可选全局筛选。

### 7.2 加载与错误

每个数据块独立处理：

- 初次加载：Skeleton；
- 空数据：EmptyState；
- 权限不足：明确提示，不展示空白；
- 部分服务失败：PartialFailureAlert + 上次成功时间；
- 整页失败：ErrorState + 重试；
- 高风险请求提交：Button loading + 禁止重复提交。

不得用全屏 Spinner 阻塞已成功加载的其他数据块。

## 8. Dashboard 规范

布局：

```text
Page Header + Filters
Action/Incident Alert
6 个复合 MetricCard
用户趋势（8列） + 今日待办（4列）
通知漏斗 + 邮件趋势
反馈趋势 + 美食五档/调档
服务状态表
最近审计
```

MetricCard 必须包含：标题、主值、趋势/辅助值、数据时间、跳转入口。没有清晰动作或口径的数据不进入首屏。

## 9. Data Table 规范

统一 DataTable 支持：

- 服务端分页；
- 白名单排序；
- 列筛选；
- URL 持久化筛选条件；
- 列显示控制；
- 行选择；
- 单行 Dropdown Menu；
- 批量操作；
- Empty/Error/Loading；
- `request_id` 错误反馈；
- 响应式处理。

默认每页 20，允许 20/50/100，具体最大值服从 OpenAPI。

筛选状态写入 Query String，使总览待办可以跳转到已过滤列表。

## 10. 高风险操作

以下操作必须使用 AlertDialog，不得用普通 Dialog 或浏览器 confirm：

- 冻结用户；
- 撤销全部 Session；
- 角色授予/撤销；
- 校园通知正式分发或取消；
- 邮件死信重放；
- 美食调档；
- 作废/恢复校准票；
- 批量审核；
- 导出敏感数据。

确认区必须展示对象、影响范围、幂等结果和不可逆性。需要二次验证的操作由后端统一控制。

## 11. 详情交互

- 轻量查看使用 Sheet；
- 复杂审核、版本 Diff、调档和完整用户详情使用独立路由；
- 审计历史使用 Timeline；
- 原始 JSON 只对技术管理员展示，默认使用结构化字段；
- 邮箱、IP、SMTP 错误和敏感 Payload 默认脱敏。

## 12. 图表规范

ECharts 仅用于趋势、漏斗、分布和排名；不用于可由文本/表格更准确表达的内容。

- 图表必须有文本摘要；
- Tooltip 显示口径与时间；
- 不在一个图表塞入超过 6 条难以区分的序列；
- 对色弱友好，不能只靠红绿；
- 导出图表不进入首发。

## 13. 可访问性

- 所有交互可键盘完成；
- Focus Ring 不得被移除；
- Icon Button 必须有可访问名称；
- 表单错误与字段关联；
- Dialog/Sheet 打开后正确管理焦点；
- 图表有文本替代；
- 颜色对比达到常规后台可读标准。

## 14. 前端数据层

- TanStack Query 管理服务端状态、缓存、重试和失效；
- Pinia 只保存认证上下文、UI 偏好和全局轻量状态；
- 禁止把列表数据复制到 Pinia；
- API Client 统一解析成功/失败 Envelope 与 `request_id`；
- Query Key 以业务域和筛选参数组成；
- Mutation 成功后精确失效 Query，禁止全局无差别刷新。

## 15. UI Foundation 验收

- `npm run build:admin` 通过；
- shadcn-vue 组件源码进入仓库；
- Tailwind v4 与主题变量生效；
- Admin Shell 支持桌面折叠和移动 Sheet；
- Dashboard Skeleton/Empty/Error/Partial 状态齐全；
- 新页面不存在 Element Plus import；
- 键盘导航和 360/390px Smoke 通过；
- 旧页面仍可访问和回滚。

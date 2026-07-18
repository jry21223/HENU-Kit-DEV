---
status: accepted
---

# HENUKit Console 使用 shadcn-vue 组件体系

HENUKit Console 使用 shadcn-vue、Reka UI 和 Tailwind CSS v4，并通过 HENU Kit Design Tokens 定制视觉；复杂表格按需组合 TanStack Table。新应用不全局安装 Element Plus，也不再引入第二套完整 UI 组件库；PR #21 中只有无旧业务依赖的纯 UI 组件可以选择性复用。

运营图表使用 ECharts 6 按需引入。每张图必须提供文字摘要或数据表，并明确处理空数据、过期数据和部分失败；普通数字卡片不得为了装饰引入图表。

服务端接口数据由 TanStack Vue Query 管理请求、缓存、刷新和失败状态；Pinia 只保存 Console Session、权限上下文和少量跨页面 UI 状态，页面局部状态使用 Vue 原生响应式能力。接口响应不得长期复制到 Pinia 形成第二份数据源。

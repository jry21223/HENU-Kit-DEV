---
status: accepted
supersedes:
  - 0003
  - 0004
---

# HENUKit Console 统一至 React 19 + Ant Design 6

## 背景

ADR-0003 与 ADR-0004 曾为 `apps/console` 规划 Vue 3 + `shadcn-vue` + TanStack Table + Vue Query + Pinia。实际演进中，`shadcn-vue` 仅落地了 7 个基础原子组件，且未安装 `vue-router`、TanStack Table 与 Vue Query，导致：

1. 侧边栏切换依赖整页刷新，各业务视图反复手写 150–250 行状态/分页/弹窗/幂等恢复样板与原生 `<dialog>`/`<select>`；
2. 仓库长期并存 React 19（`apps/portal`）、React 18（`products/quizcraft/web-app`）与 Vue 3（`apps/console`）三套前端栈，题库工坊等新模块难以共享前端能力；
3. 业务文案将底层幂等与游标围栏等实现细节暴露为主界面说明，增加运营理解成本。

## 决策

- `apps/console` 切换为 **React 19 + Vite 8 + Ant Design 6 (`antd`) + `@ant-design/icons` + React Router 7 + TanStack Query 5**。
- **100% 复用既有框架无关的契约与安全底座**：保留 `src/lib/console-gateway.ts`、`src/lib/console-session.ts`、`src/lib/pending-operations.ts`、`src/lib/account-labels.ts`、`src/lib/base-path.ts`、`src/lib/platform-view-context.ts`，以及 OAuth Continuation 门禁要求的 8 条路由与 heading ID。
- **新增题库工坊模块（`/practice`）**：通过 `src/lib/quizcraft-workshop.ts` 对接 `console-gateway` 已注册的 `/api/v1/quizcraft/workshop/*` 代理接口，并在网关未开启桥接时降级提示。
- **视觉定位延续 `DESIGN_SYSTEM.md` 第 21 节**：通过 Ant Design `ConfigProvider` 配置中性灰白高可读主题，不加载外部展示字体，不混入学生端工程图纸主题。
- **学生端（`apps/portal`）保持独立**：继续坚守纸白/墨色/强调橙视觉体系，不引入 Ant Design。

## 影响

- 现役前端产品（Portal / Console / QuizCraft）统一为 React + TypeScript 生态。
- 后台表格、分页、筛选、抽屉表单、高风险确认弹窗与全局提示全部复用成熟组件库，显著降低新模块接入成本。

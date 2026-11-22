# HENUKit Console

HENUKit Console is the **sole operator / admin UI** for HENU Kit production. Study Legacy Admin (`apps/study-legacy-admin`) is retired from the product entry and default deploy path; keep that tree only for emergency break-glass, never as the default admin surface.

Console is deployed on its own subdomain (`console.henukit.cn`) and built with `VITE_BASE_PATH=/` (the compose default), so the container serves the app at `/` and same-origin `/api/*` calls reach the Console Gateway edge instead of the Portal Gateway. The legacy `henukit.cn/console` subpath is retired: the shared edge redirects `/console/` to `https://console.henukit.cn/` during the cutover observation window. Vite `base` is derived from `VITE_BASE_PATH`; app-relative navigation uses the same base, and `/api/*` calls are not rewritten by it.

This directory is the physical application boundary for HENUKit Console. Per ADR-0060, it is built with React 19, TypeScript, Vite 8, Ant Design 6 (`antd`), React Router 7, and TanStack Query 5, using Ant Design's neutral theme (per `DESIGN_SYSTEM.md` §21). The legacy Study Admin does not contribute routes, Element Plus, or Study API types to this bundle.

Console obtains its Session, access context, module summaries, and operational commands from the same-origin Console Gateway (`/api/v1/*`). Active workspaces cover Overview (`/`), Food Governance (`/food`), Campus Notices (`/notices`), QuizCraft Question Bank Workshop (`/practice`), Library Readiness (`/library`), Account Tickets (`/account`), Account Memberships (`/account/memberships`), Account Points (`/account/points`), and Platform Operations (`/operations`).

## Commands

- `pnpm --filter @henukit/console run dev`
- `pnpm --filter @henukit/console run lint`
- `pnpm --filter @henukit/console run test`
- `pnpm --filter @henukit/console run test:e2e`
- `pnpm --filter @henukit/console run build`
- `VITE_BASE_PATH=/ pnpm --filter @henukit/console run build`

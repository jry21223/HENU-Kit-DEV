# HENUKit Console

HENUKit Console is the **sole operator / admin UI** for HENU Kit production. Study Legacy Admin (`apps/study-legacy-admin`) is retired from the product entry and default deploy path; keep that tree only for emergency break-glass, never as the default admin surface.

`VITE_QUIZCRAFT_WORKSHOP_URL` is optional and defaults to empty. The standalone QuizCraft workshop is not part of the HENU Kit primary runtime; set this only for an explicitly provisioned external QuizCraft deployment. The Console does not assume a production domain.

To serve Console under a subpath such as `/console/`, set `VITE_BASE_PATH` at build time (for example `VITE_BASE_PATH=/console/`). Vite `base` is derived from that env; app-relative navigation uses the same base. Same-origin `/api/*` calls still target the Console Gateway edge and are not rewritten by `VITE_BASE_PATH`.

This directory is the physical application boundary for HENUKit Console. It contains a responsive six-module Overview built with Vue, shadcn-vue conventions, Reka UI, Tailwind CSS v4, and HENU Kit Design Tokens. The legacy Study Admin does not contribute routes, Element Plus, or Study API types to this bundle.

Console obtains its Session, access context, and six module summaries from the same-origin Console Gateway. The cards retain only presentation metadata in the bundle; all metrics, degradation states, observation timestamps, last-success timestamps, and request identifiers come from the bounded Gateway aggregation response. Product operation routes remain planned.

## Commands

- `pnpm --filter @henukit/console run dev`
- `pnpm --filter @henukit/console run lint`
- `pnpm --filter @henukit/console run test`
- `pnpm --filter @henukit/console run test:e2e`
- `pnpm --filter @henukit/console run build`
- `VITE_BASE_PATH=/console/ pnpm --filter @henukit/console run build`

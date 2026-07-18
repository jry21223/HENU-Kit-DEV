# HENUKit Console

This directory is the physical application boundary for HENUKit Console. It contains a responsive six-module Mock Overview built with Vue, shadcn-vue conventions, Reka UI, Tailwind CSS v4, and HENU Kit Design Tokens. The legacy Study Admin does not contribute routes, Element Plus, or Study API types to this bundle.

The current cards are presentation fixtures used to validate information architecture, degradation states, permission-state rendering, accessibility, and 390px behavior. They do not call Console Gateway, authenticate an operator, make authorization decisions, or expose production operations.

## Commands

- `pnpm --filter @henukit/console run dev`
- `pnpm --filter @henukit/console run lint`
- `pnpm --filter @henukit/console run test`
- `pnpm --filter @henukit/console run test:e2e`
- `pnpm --filter @henukit/console run build`

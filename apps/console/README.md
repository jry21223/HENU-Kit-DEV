# HENUKit Console

This directory is the physical application boundary for HENUKit Console. It contains a responsive six-module Mock Overview built with Vue, shadcn-vue conventions, Reka UI, Tailwind CSS v4, and HENU Kit Design Tokens. The legacy Study Admin does not contribute routes, Element Plus, or Study API types to this bundle.

The current cards remain presentation fixtures for information architecture, degradation states, accessibility, and 390px behavior. Console now obtains its Session and access context from the same-origin Console Gateway; fixture metrics are withheld until Platform Core has verified `console.overview.read` and platform Scope. Product summary aggregation and production operations are not yet connected.

## Commands

- `pnpm --filter @henukit/console run dev`
- `pnpm --filter @henukit/console run lint`
- `pnpm --filter @henukit/console run test`
- `pnpm --filter @henukit/console run test:e2e`
- `pnpm --filter @henukit/console run build`

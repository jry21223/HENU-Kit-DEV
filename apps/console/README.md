# HENUKit Console

This directory is the physical application boundary for HENUKit Console. It contains a responsive six-module Overview built with Vue, shadcn-vue conventions, Reka UI, Tailwind CSS v4, and HENU Kit Design Tokens. The legacy Study Admin does not contribute routes, Element Plus, or Study API types to this bundle.

Console obtains its Session, access context, and six module summaries from the same-origin Console Gateway. The cards retain only presentation metadata in the bundle; all metrics, degradation states, observation timestamps, last-success timestamps, and request identifiers come from the bounded Gateway aggregation response. Product operation routes remain planned.

## Commands

- `pnpm --filter @henukit/console run dev`
- `pnpm --filter @henukit/console run lint`
- `pnpm --filter @henukit/console run test`
- `pnpm --filter @henukit/console run test:e2e`
- `pnpm --filter @henukit/console run build`

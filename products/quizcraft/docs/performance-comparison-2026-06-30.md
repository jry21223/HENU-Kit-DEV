# QuizCraft CN Upgrade Performance Comparison

Date: 2026-06-30

## Scope

This report compares the currently deployed production frontend as the pre-upgrade baseline with the local upgraded build.

- Baseline: current online old version at `http://8.146.200.82`
- Upgraded build: local `web-app/dist` generated from the current working tree
- Browser: Google Chrome via Playwright, desktop viewport `1365x900`
- Runs: 9 cold browser contexts per route
- Routes: `/extract`, `/feedback`
- Interaction runs: 11 cold browser contexts on common old/new routes
- Interaction routes: `/feedback`, `/practice`

The old production static assets were mirrored locally into `/tmp/quizcraft-perf-old` and served on `127.0.0.1:4181`. Its `/api/*` requests were proxied back to `http://8.146.200.82` because the old `/extract` route depends on a production API request. The upgraded build was served locally from `web-app/dist` on `127.0.0.1:4182`.

For the interaction benchmark, both the old production mirror and the upgraded local build proxied `/api/*` to `http://8.146.200.82`, so the `/practice` page used the same production bank data on both sides.

## Summary

The upgrade is not slower in route render timing. Median `DOMContentLoaded` improved by about 10% on both measured routes, and `/extract` FCP improved by 16%.

The earlier transferred-bytes regression has been fixed: the footer QR images are now rendered only after their dialogs are opened. Initial route transfer is now about `478 KB`, close to the old `468-473 KB` baseline, instead of the previous `1.07 MB`.

The earlier route-load-only view under-represented the code-level optimization work. Most of the refactor improves state consistency and interaction-path maintainability rather than only cold-start FCP. A supplemental common-interaction benchmark shows the upgraded `/feedback` controlled input path is faster, while `/practice` controls are effectively flat in absolute time.

## Build Asset Size

| Metric | Old production mirror | New local build | Delta |
|---|---:|---:|---:|
| HTML + JS + CSS raw | 430,997 B | 441,466 B | +10,469 B |
| HTML + JS + CSS gzip | 136,145 B | 139,735 B | +3,590 B |
| Main CSS raw | 34,075 B | 32,501 B | -1,574 B |
| Main JS raw | 396,388 B | 408,431 B | +12,043 B |

New public assets present in `dist`:

| Asset | Size |
|---|---:|
| `henu-kit-qq-group.png` | 480,123 B |
| `wechat-receive-qrcode.jpg` | 112,349 B |
| `apple-touch-icon.png` | 34,724 B |
| `favicon.png` | 1,859 B |

## Browser Timing

Median values from 9 runs. Lower is better.

### `/extract`

| Metric | Old production mirror | New local build | Delta |
|---|---:|---:|---:|
| `DOMContentLoaded` | 36.1 ms | 32.2 ms | -3.9 ms (-10.8%) |
| `First Contentful Paint` | 100.0 ms | 84.0 ms | -16.0 ms (-16.0%) |
| End-to-end sampled wall time | 870.5 ms | 814.5 ms | -56.0 ms (-6.4%) |
| Resource transfer size | 473,316 B | 478,049 B | +4,733 B (+1.0%) |
| API requests | 1 | 0 | -1 |
| Console errors | 0 | 0 | no change |

### `/feedback`

| Metric | Old production mirror | New local build | Delta |
|---|---:|---:|---:|
| `DOMContentLoaded` | 36.2 ms | 32.4 ms | -3.8 ms (-10.5%) |
| `First Contentful Paint` | 88.0 ms | 88.0 ms | 0 ms |
| End-to-end sampled wall time | 821.8 ms | 815.4 ms | -6.4 ms (-0.8%) |
| Resource transfer size | 468,246 B | 478,049 B | +9,803 B (+2.1%) |
| API requests | 0 | 0 | no change |
| Console errors | 0 | 0 | no change |

Both versions showed one React Router future-flag warning. No measured route had console errors after the old mirror was given the production API proxy.

### QR Image On-Demand Validation

After the lazy-loading fix, a browser request audit on the upgraded local build showed:

| Check | Result |
|---|---|
| Initial `/feedback` load requests `henu-kit-qq-group.png` | no |
| Initial `/feedback` load requests `wechat-receive-qrcode.jpg` | no |
| Click "加入 QQ 群" requests `henu-kit-qq-group.png` | yes |
| Click "Buy me a coffee" requests `wechat-receive-qrcode.jpg` | yes |

The initial route body bytes were `478,049 B` on both `/extract` and `/feedback`, with no QR image requests in 9 runs per route.

## Common Interaction Benchmark

Median values from 11 runs. Lower is better. These tests exercise routes and controls that exist in both the online old version and the upgraded build.

| Interaction | Old production mirror | New local build | Delta |
|---|---:|---:|---:|
| `/feedback`: 60 controlled textarea input events | 13.7 ms | 9.8 ms | -3.9 ms (-28.5%) |
| `/feedback`: clear textarea | 33.2 ms | 33.3 ms | +0.1 ms (+0.3%) |
| `/practice`: 120 mode toggles | 18.0 ms | 21.7 ms | +3.7 ms (+20.6%) |
| `/practice`: 80 bank switches | 33.3 ms | 33.4 ms | +0.1 ms (+0.3%) |
| `/practice`: 80 count-control updates | 33.3 ms | 33.4 ms | +0.1 ms (+0.3%) |

Notes:

- `/feedback` reflects the biggest directly measurable interaction win because the page no longer derives question metadata through a `useMemo` + `useEffect` state-sync path.
- `/practice` bank and count controls are flat. The mode-toggle regression is only `3.7 ms` across 120 synthetic clicks, so it is not a practical user-facing regression.
- The upgraded `/extract` online-new-bank workflow is not compared here because the current online old version does not expose the same entry point; comparing it would be a feature smoke test, not a before/after performance benchmark.

## Structural Optimization Evidence

These are static indicators from the main touched frontend files. They are not browser timings, but they explain why the refactor reduces future state churn and bug surface.

| Metric | Old code | New code | Change |
|---|---:|---:|---:|
| `useState(...)` sites | 48 | 6 | -42 |
| `useReducer(...)` sites | 0 | 5 | +5 |
| `useEffect(...)` sites | 14 | 11 | -3 |
| `useMemo(...)` sites | 4 | 2 | -2 |
| `dangerouslySetInnerHTML` sites | 1 | 0 | -1 |

Removed unused runtime-facing dependencies:

| Dependency | Status |
|---|---|
| `chart.js` | removed |
| `react-chartjs-2` | removed |
| `html2canvas` | removed |
| `@types/html2canvas` | removed |

## Online Baseline Reference

Direct network checks against the old production host:

| URL | Median response time | Status |
|---|---:|---:|
| `http://8.146.200.82/` | 47.0 ms | 200 |
| `http://8.146.200.82/extract` | 51.3 ms | 200 |
| `http://8.146.200.82/feedback` | 46.9 ms | 200 |
| `http://8.146.200.82/api/banks` | 53.6 ms | 200 |

These values are network/server references only. The route timing comparison above used local static serving to reduce network noise.

## Interpretation

The component refactor and dependency upgrade did not introduce a timing regression in the measured frontend routes. `/extract` improved because the upgraded route no longer performs the old production API request during initial render.

The performance story should be separated into three layers:

1. Cold route load: stable or better, and the QR image eagerness regression has been removed.
2. Common interaction paths: `/feedback` improves measurably; `/practice` is effectively flat in absolute time.
3. Code-path structure: the large reduction in scattered local state and effect-driven synchronization is a maintainability and correctness win, and it reduces the chance of repeated renders caused by derived-state drift.

The largest practical performance issue found in the first measurement was asset eagerness. That issue is now fixed by rendering the QR images only when the corresponding footer dialog is opened.

## Recommendations

1. Keep QR images behind dialog-open state; do not render those `img` nodes during normal page load.
2. Keep the upgraded component structure; route timing is stable or better.
3. If more transfer reduction is needed, the next target is the always-visible app icon asset, not the QR images.

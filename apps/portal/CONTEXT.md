# HENU Kit Portal Context

Last updated: 2026-09-11.

## Owns

- The HENU Kit main site frontend (henukit.cn).
- Campus tool system product shell: brand, navigation, and entry points.
- Homepage module sections (Library, Practice, Food, Campus, Career) with scroll-driven animations.
- Sub-site layouts and navigation for Library, Practice, Food, Campus Market, and Career.
- Mock data layer and deterministic SSR rendering (mulberry32 seeded PRNG).
- GSAP animation system with prefers-reduced-motion respect.
- Three.js 3D hero scenes (homepage and practice bank hero).

## Does not own

- Question banks, attempts, rankings, or practice logic (owned by the Practice service).
- Library public-free material eligibility, signed download grants, Download
  Start facts, and aggregates (owned by Library); remaining legacy operational
  facts stay behind the Study compatibility boundary.
- Food submissions, anomaly tickets, or tier adjustments (owned by Food service).
- Campus market transaction data (owned by campus_life).
- Credential validation, account records, Core Sessions, recovery policy, or mail delivery (owned by Platform Core).
- Any business backend or database access, including Account Portfolio points, memberships, notifications, and tickets.
- Console operations dashboard (owned by apps/console).

## Current boundary

Portal owns the public account page presentation at `/account/login`, `/account/recover`, `/account/security`, and the account overview. These existing pages submit same-origin forms through `/account-auth` to Platform Core; Portal never validates credentials or treats a local response as authenticated. Successful login, registration, and recovery continue through OAuth so Portal Gateway establishes the Portal Session. The account overview reads Account Portfolio only through that Gateway and shows an explicit error instead of session or local mock data when the owner is unavailable. Production mock authentication is prohibited. See ADR-0014 and ADR-0016.

Account pages obtain CSRF and flow state through Platform Core's bounded Account Center Bootstrap response and request explicit status responses for every credential form. Portal does not parse Platform Core HTML or expose backend error detail; Platform Core still decides every credential and Core Session outcome. Direct Platform Core account-page requests now return users to the Portal presentation, and no embedded credential page remains.

When a validated Portal or Console OAuth authorize request reaches Platform Core without a Core Session, the Account Center receives only a short-lived opaque continuation handle. It renders the trusted server-provided product name, submits credentials through the same explicit Platform Core contract, and navigates with a same-origin POST to resume that continuation. Its form policy allows only the Portal origin plus the exact production Console origin so the registered Console callback can complete cross-origin; local acceptance may add only a loopback origin. Portal never receives OAuth state, PKCE, callback, Authorization Code, or product Session facts; expired, replayed, tampered, and cross-browser handles render the Portal-owned safe recovery state.

The first `/account/login?continuation=...` request is transport-only and must
never render HTML. Portal's Next proxy answers it with a private, no-store,
no-referrer `303` whose same-origin destination carries the handle in the URL
fragment. Fragments are excluded from the following HTTP request and Referer,
so the final Account Center document and its eager font/script requests start
from `/account/login` without the handle. Before paint, the client captures and
removes the fragment with the native `History.prototype.replaceState`; using
Next's patched history instance here is prohibited because it can dispatch an
RSC request before URL sanitization. The handle then exists only in component
state for bootstrap, same-page retry, and the final resume POST.

The release gate runs one shared parameterized browser journey for Portal and
Console. It verifies signed-out continuation, the existing-Core-Session fast
path, fail-closed variants, 360px keyboard and reduced-motion behavior, public
copy, browser leakage, and bounded observability before any fixed-SHA release
artifact can be built. See `docs/operations/oauth-continuation-acceptance.md`.

The Practice catalog preparation is explicitly dark by default: without `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1`, `/practice` does not request or render the service catalog. When the browser flag and Gateway's matching server-side flag are both deliberately coordinated, Portal renders only Gateway-provided real bank and immutable bank-version facts, emits both IDs to `/practice/quiz`, and treats loading, empty, and error states honestly. A failed catalog read must not fall back to legacy Practice, cache, or local mock success. The merged default-off Practice session boundary owns the browser `create-session` command; this catalog ticket verifies its real bank/version handoff into that boundary, including a stable idempotency key under React development replay.

For ADR-0027 public-free downloads, Portal navigates only to the same-origin
owner façade for the selected material ID. It never constructs `/materials/`
or OSS URLs from `storage_key`, stores a signed URL in component state, counts a
click locally, or treats a failed grant as a successful download.

ADR-0031 keeps the complete public materials UI download-only. Portal renders
no online reader, Slides viewer, or preview action; both legacy `/library/read/`
and `/library/slides/` routes return users to the material detail, whose only
file capability is the owner download route.

The Library homepage reads one complete active owner snapshot through Portal
Gateway. Its collection count comes from that full snapshot and its cumulative
download count comes from Library's append-only Download Start ledger; browser
search and filters change only visible cards. Loading and failed reads expose
no numeric claim, while an empty successful owner snapshot may truthfully show
zero.

## Design language

"Industrial Minimal" — warm paper white (#F2F0EA), deep ink text (#161513), safety orange accent (#FF4D00). Typography: Space Grotesk (display), IBM Plex Mono (labels), system Chinese fonts (body). Visual elements: 1px structural lines, crosshair alignment marks, mono numbering, engineering blueprint grid backgrounds.

## Tech stack

Next.js 16 (App Router) + React 19 + Tailwind CSS v4. GSAP 3.15 with ScrollTrigger/Observer. Three.js via @react-three/fiber. No external state library. No charting library (hand-written SVG charts).

## Key terms

- **Portal Configuration**: Content and navigation changes through Git, review, and CI/CD. Console has no content editor.
- **Account page**: Existing Portal presentation that collects credential form input and sends it unchanged through the same-origin `/account-auth` bridge. Platform Core alone decides success and establishes the Core Session.
- **Account entry**: A Portal navigation point that starts Portal Gateway OAuth. After Platform Core accepts a credential flow, Portal continues that OAuth flow to establish its own Gateway Session.
- **Module section**: A homepage block introducing one of the five products: Library, Practice, Food, Campus, or Career.
- **Sub-site**: A product area with its own navigation: Library, Practice, Food, Campus, or Career.
- **Back to the level above**: The top-left control of a sub-site navigation. It walks the path hierarchy exactly one level up — an inner page to that sub-site's home (quiz to the bank catalog, a venue to the board), a sub-site home to the platform home — so an inner page never jumps straight to the platform home. It is the level above, not browser history: arriving by deep link or refresh lands on the same page. Its label names the level it returns to, in the sub-site navigation's own words (书库, 榜单, 市集, 题库) — the word the reader sees on the destination's tab row. The homepage entries and module SEO titles use a separate register (资料库, 美食榜, 互助平台); unifying the two registers is a separate, undecided change. The account console and the account auth shells are not sub-sites: they keep their own brand link to the platform home.
- **Reading position**: The scroll offset a reader last left a path at, remembered per path for the browser tab. Returning to a level above puts the reader back at it, including the platform home; entering a path for the first time, moving sideways between tabs, or going deeper all start at the top.
- **Gesture intent**: What one touch or pointer gesture is asking for, decided once when the finger lifts, from that gesture's net displacement and peak velocity — not from a running accumulation crossing a small threshold. A step needs net displacement of 6% of the viewport height, or 3% with a peak velocity of at least 600 px/s (provisional, awaiting real-device calibration); that is what stops a light tap, a wobble, or a pre-click shift from flipping a whole screen. The judgement is one pure module (`src/lib/navigation/gesture-intent.ts`); the wheel keeps its per-tick behaviour until its own burst window is decided.
- **One gesture, one screen**: A touch or pointer gesture steps at most one module section. The judgement runs once, in the release callback, and nothing is consumed before it: a gesture that begins while a screen animation is still running can still step once it settles, and a gesture that finds the first or last screen does not spend itself on the empty step.
- **Net displacement**: The displacement of one gesture accumulated while it keeps its direction and restarted from the new direction when the reader turns around — the last direction is the one that counts. It is sampled from Observer's reported pointer position rather than its `deltaY`, whose bucket fires only once the accumulated movement passes `tolerance` (12px) and therefore loses both the trailing remainder and the resolution of the threshold it is compared against.
- **Peak velocity**: The Observer velocity read at the moment the finger lifts. It is only available there: `onStop` runs after the velocity has been zeroed, so reading it in `onStop` always yields 0. Its reading drifts with the frame rhythm (measured −59…−960 px/s for the same physical flick, and 0 for a single-frame flick), so it carries the secondary channel rather than deciding alone.
- **Viewport-relative threshold**: A step threshold expressed as a share of the viewport height (6% for net displacement, 3% as the floor for the velocity channel) instead of a fixed pixel count, so the same gesture falls into the same judgement on a short and a tall screen. The share never drops below the drag minimum (8px), the displacement under which Observer does not treat the movement as a drag at all.
- **Quick entry**: A first-screen shortcut to a common task: finding materials, starting practice, or viewing job opportunities.
- **Material display title**: A readable material name shown alongside its course and material type to help readers identify it.
- **Original material title**: The complete title supplied by the material owner. It is distinct from the file name; a title alone does not establish the file name.
- **Deterministic SSR**: Seeded randomness (mulberry32) and picsum seed URLs ensure server/client output match.

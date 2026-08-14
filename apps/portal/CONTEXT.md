# HENU Kit Portal Context

## Owns

- The HENU Kit main site frontend (henukit.cn).
- Campus tool system product shell: brand, navigation, and entry points.
- Homepage module sections (Library, Practice, Food, Campus) with scroll-driven animations.
- Sub-site layouts and navigation for Library, Practice, Food, and Campus Market.
- Mock data layer and deterministic SSR rendering (mulberry32 seeded PRNG).
- GSAP animation system with prefers-reduced-motion respect.
- Three.js 3D hero scenes (homepage and practice bank hero).
- Leaflet map integration for food post locations.

## Does not own

- QuizCraft question banks, attempts, rankings, or practice logic (owned by QuizCraft product).
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

The QuizCraft V2 catalog preparation is explicitly dark by default: without `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1`, `/practice` does not request or render a V2 catalog. When the browser flag and Gateway's `PORTAL_ENABLE_QUIZCRAFT_CATALOG=1` are both deliberately coordinated, Portal renders only Gateway-provided real bank and immutable bank-version facts, emits both IDs to `/practice/quiz`, and treats loading, empty, and error states honestly. A failed V2 catalog read must not fall back to legacy Practice, cache, or local mock success. The merged default-off Practice session boundary owns the browser `create-session` command; this catalog ticket verifies its real bank/version handoff into that boundary, including a stable idempotency key under React development replay.

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

Next.js 16 (App Router) + React 19 + Tailwind CSS v4. GSAP 3.15 with ScrollTrigger/Observer. Three.js via @react-three/fiber. Leaflet + react-leaflet. No external state library. No charting library (hand-written SVG charts).

## Key terms

- **Portal Configuration**: Content and navigation changes through Git, review, and CI/CD. Console has no content editor.
- **Account page**: Existing Portal presentation that collects credential form input and sends it unchanged through the same-origin `/account-auth` bridge. Platform Core alone decides success and establishes the Core Session.
- **Account entry**: A Portal navigation point that starts Portal Gateway OAuth. After Platform Core accepts a credential flow, Portal continues that OAuth flow to establish its own Gateway Session.
- **Module section**: A full-viewport homepage block for each product (Library, Practice, Food, Campus).
- **Sub-site**: A top-level route group (/library, /practice, /food, /campus) with its own layout and navigation.
- **Deterministic SSR**: Seeded randomness (mulberry32) and picsum seed URLs ensure server/client output match.

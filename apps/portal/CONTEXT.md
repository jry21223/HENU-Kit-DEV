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
- Library material files, downloads, or business data (owned by Study API).
- Food submissions, anomaly tickets, or tier adjustments (owned by Food service).
- Campus market transaction data (owned by campus_life).
- Account Center pages, registration, authentication credentials, account recovery, or mail delivery (owned by Platform Core).
- Any business backend or database access.
- Console operations dashboard (owned by apps/console).

## Current boundary

Portal is transitioning from its frontend-only prototype to real Portal Gateway data and Session integration. Its pre-launch account pages still contain product-local mock authentication and an interim Account Center form adapter; neither is valid production authentication. ADR-0014 makes Account Center the sole registration and login UI, leaving Portal with account entry points and its own OAuth-established Portal Session only.

## Design language

"Industrial Minimal" — warm paper white (#F2F0EA), deep ink text (#161513), safety orange accent (#FF4D00). Typography: Space Grotesk (display), IBM Plex Mono (labels), system Chinese fonts (body). Visual elements: 1px structural lines, crosshair alignment marks, mono numbering, engineering blueprint grid backgrounds.

## Tech stack

Next.js 16 (App Router) + React 19 + Tailwind CSS v4. GSAP 3.15 with ScrollTrigger/Observer. Three.js via @react-three/fiber. Leaflet + react-leaflet. No external state library. No charting library (hand-written SVG charts).

## Key terms

- **Portal Configuration**: Content and navigation changes through Git, review, and CI/CD. Console has no content editor.
- **Account entry**: A Portal navigation point that starts Account Center or Portal Gateway OAuth. It does not collect credentials or decide whether authentication succeeded.
- **Module section**: A full-viewport homepage block for each product (Library, Practice, Food, Campus).
- **Sub-site**: A top-level route group (/library, /practice, /food, /campus) with its own layout and navigation.
- **Deterministic SSR**: Seeded randomness (mulberry32) and picsum seed URLs ensure server/client output match.

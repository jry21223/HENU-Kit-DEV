# Practice Markdown and touch navigation

Approved scope: 2026-10-18 user request and accepted research plan.
Baseline: 6008f76346fcc4dab1652505c371d62ebde3d30d.

Portal owns presentation only; Practice Core retains scoring, immutable question
versions, attempts, and idempotency. No database or API-contract change.

## Acceptance seams

- Student-visible rendered question: fenced and inline code preserve content,
  indentation and generic type characters; GFM tables render; raw HTML does not
  execute. Stem, options and explanation share a renderer. Code/table overflow
  stays inside its container at 360px. No guessed reconstruction of damaged code.
- Student-visible navigation: touch follows the finger, snaps or returns, respects
  direction and boundaries, retains drafts, retry keys and confirmed results.
  Code scrolling, form controls, browser edge gestures, vertical reading and
  pinch zoom must not trigger question changes. Buttons and keyboard share the
  same navigation entry. Reduced motion simplifies settling animation.
- Sub-site navigation: retain native scrolling; smooth active-item reveal and
  respect reduced motion. No global smooth scrolling or navigation delay added.

## Implementation

Use react-markdown, remark-gfm and rehype-highlight with a bounded language set.
Use Swiper React for the question track and existing GSAP only on separate
presentation elements. Render the current and adjacent questions, retaining
answer state in the page keyed by immutable question identity. Inactive slides
are inert and hidden from assistive technology. Follow existing design tokens.

## Verification and release

Red/green renderer tests, real browser drag and nested-scroll cases, existing
Practice state/idempotency tests, Portal lint/typecheck/unit/build. Independent
Standards and Spec reviews, plus Public-ready Copy, before committing. PR and
main release gates must pass before the documented exact-SHA artifact activation;
verify the public site and record the live revision. A merge alone is not proof
of production activation.

## Implementation evidence

- Renderer regression: first failed because the renderer did not exist, then
  passed; five examples cover Java fences/generics/indentation, inline code,
  GFM tables, inert HTML/unsafe URLs and valid structured option content.
- Portal unit suite: 39 files / 300 tests passed; unit contracts: 5 passed.
- TypeScript passed. ESLint passed with existing navigation warnings only.
- Production build and both public-artifact checks passed.
- Chromium production browser run covers Practice, submission retry identity,
  drafts/results, direction cancellation, reduced motion, native controls,
  code keyboard scrolling, genuine touchscreen follow-finger geometry and
  horizontal code scrolling. Sub-site navigation covers 360px/390px and desktop.
- Independent Standards and Spec reviews identified keyboard focus transfer,
  code/table arrow-key ownership and structured option tables; all were fixed
  and re-reviewed with zero remaining findings. Public-ready Copy: 0 findings.

Local browser adaptation: the environment's stock browser download returned
empty archives; Chromium 141 headless shell was used through an uncommitted
local config. The full Chrome executable cannot create its singleton socket in
this environment. Next dev/webpack caused an HMR reload during parallel route
compilation; the final browser acceptance runs against the built production
app. Chinese Noto fonts were installed in the test environment for faithful
mobile tab widths. Neither local config nor fonts are repository changes.

Production activation requires the existing operator-owned exact-SHA release
entry and evidence described in docs/operations/henukit-artifact-deployment.md.
This workspace has no configured production SSH identity; local success and
GitHub merge must not be described as proof that production changed.

Compatibility follow-up: an additional red/green regression showed that discarding
raw HTML nodes also lost the literal `<String>` from unformatted legacy prose.
The renderer now uses react-markdown's default escaped-text handling (without
rehype-raw), preserving that notation while keeping HTML inert. Standards and
Spec delta reviews: 0 findings; no new product copy.

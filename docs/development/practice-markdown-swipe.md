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
The original implementation workspace had no configured production SSH identity.
Local success and GitHub merge must not be described as proof that production
changed; the joint release now uses the existing Mac operator environment.

Compatibility follow-up: an additional red/green regression showed that discarding
raw HTML nodes also lost the literal `<String>` from unformatted legacy prose.
The renderer now uses react-markdown's default escaped-text handling (without
rehype-raw), preserving that notation while keeping HTML inert. Standards and
Spec delta reviews: 0 findings; no new product copy.


## H3C joint release integration (2026-10-20)

PR #598 is the existing H3C delivery, owned by the “整理并上线 H3C 题库” task.
Its nine source sets contain 1,369 reviewed questions; 542 source records remain
held. The owner has completed publication and acceptance, merging supplements
into five published banks while retaining nine original PDFs. PR #588 retains
that delivery in the same source history. getWork remains deferred and Food
author editing must remain available through the reviewed release boundary.

Merge the current main into this existing PR and retain the content-addressed
H3C figure allowlist in the shared Markdown renderer. Reviewed figures work in
stems, options and analysis; option figures do not nest interactive links inside
answer buttons. External/traversal image sources do not create image requests.
Keep the current source-figure fallback/recovery behavior and Markdown/swipe
acceptance together. No content import, publication, permissions or migrations
are performed by this presentation change.

The dependency was freshly identified from PR #598 and the owner's current
publication evidence, not from the unrelated standalone QuizCraft upload skill.
Merged code, successful builds and frozen local banks are not proof of completed
production content publication. The joint release additionally requires final
main CI, attestation, both database isolated restores, scoped rollback safety,
real author Food edit, real Library download and public Practice answer/image/
Markdown/swipe acceptance on the user's Mac.


Joint integration verification: Portal 307 unit tests and five Node contracts,
TypeScript, production build and public-artifact checks passed. Lint reports no
errors (three existing navigation warnings). On the user's macOS 26.6, the
production build passed all 51 Chromium Practice/session/answer-feedback/swipe/
transition/navigation cases. This includes H3C figures and failed-image recovery.
A real CDP pinch first reproduced unintended navigation. The fix uses passive
native document capture listeners scoped by the active pager gesture; it avoids
Swiper React's event-prop translation, catches a second finger in the page
gutter, leaves browser defaults intact and restores swiping after all touches
end. The regression passes before/after horizontal intent locks, from the gutter,
and confirms the next fresh single-finger swipe works. Independent Standards,
Spec and Public-ready Copy delta reviews have zero remaining findings. These
are local integration results, not a production deployment claim.

## Current-main compatibility and release hold

The next integration baseline is main
`27b922825452f46fa1678170b6b1f20ef6180c94` (PR #601). Keep its self-service
statistics/favorites authorization and Food author editing unchanged. The shared
figure renderer retains #601's responsive maximum height of `min(28svh,280px)`,
automatic width and no upscaling, including figures in Markdown options and
explanations. Existing rendered-browser tests cover three viewport sizes,
portrait/square/landscape figures, the original-image link and image recovery.

This continuation prepares the existing PR only. Do not merge or switch
production until the release hold is resolved. The active production HMAC
configuration already exists, but the current fixed-five helper generates a
new key for each new release. Existing configuration is not authorization to
create or rotate a credential. Investigating a path that preserves credentials
in place does not authorize changing the helper or production configuration.

# HENU Kit English training: executable release spec

## Problem Statement

Learners need short listening and cloze practice that explains the evidence behind an answer, preserves full context, and remembers what still needs independent practice. Ordinary exam scoring, rankings, and browser-only progress cannot represent assistance honestly.

## Solution

Add two general English training kinds to the existing Practice experience. Listening proceeds from real audio and answers to evidence replay, sentence looping, and layered text. Cloze keeps the whole passage visible, supports stable blank navigation and layered help, then shows contextual evidence and error categories. A short unit can be resumed and reviewed through Practice Core. Publish at least one legally usable, checked unit of each kind through the normal owner lifecycle and formally deploy the complete feature.

## User Stories

1. As a learner, I can choose listening or cloze in the existing Practice experience without a second English homepage.
2. As a learner, I know these are general English exercises and can inspect source, attribution, license, and originality claims.
3. As a listener, I can load real audio, play/pause, seek, and change playback speed without receiving answer-bearing text before I submit the complete unit.
4. As a listener, I can recover from missing, failed, or unsupported audio and retry without losing saved answers.
5. As a listener, after submitting the unit I can locate the audio evidence for an answer and loop a complete sentence.
6. As a listener, after submission I can reveal English text and then Chinese meaning progressively rather than see everything at once.
7. As a cloze learner, I can read the full blanked passage, jump among stable blanks, choose options with keyboard or touch, and retain context.
8. As a cloze learner, I can request layered help, see that it affects independent completion for every still-unsubmitted question in this unit, and never have assisted work presented as exam accuracy.
9. As a learner, I can submit a unit with repeated clicks or a flaky network without duplicate attempts, counters, or scores.
10. As a learner, when only some answers saved I see the exact saved count, recover Core facts, and retry only the missing answers.
11. As a learner, I can refresh or return through a session URL and recover the pinned content, saved answers, disclosed hints, and results.
12. As a guest, I am told that recovery follows this browser's existing anonymous session, while signed-in learning state follows the account.
13. As a learner, another account or browser cannot read or alter my training session by guessing its identifier.
14. As a learner, I see independent correct, completed with help, and needs review as distinct outcomes.
15. As a learner, wrong or assisted answers remain in the existing Core-owned review state; a fresh independently correct attempt can clear them.
16. As a learner, I can review the relevant contextual evidence and authored error category without claims of precise long-term diagnosis.
17. As a learner, starting another unit, returning, cancelling, or unmounting stops old audio and does not create a duplicate score.
18. As a keyboard or mobile learner, the workflow works at 360px without page overflow, lost focus, horizontal gesture conflicts, or motion when reduced motion is requested.
19. As an operator, invalid media metadata, duplicate blanks, bad evidence references, missing license records, or malformed options fail publication without partial writes.
20. As an operator, new content versions preserve old session question, passage, hint, and timestamp identity.
21. As an operator, existing ordinary Practice totals, difficulty, mastery, rankings, settlements, and write gates remain correct with training facts present.
22. As the product owner, I receive exact merged SHA, required CI, actual production version, backup/restore and rollback evidence, and complete production workflow evidence.

## Implementation Decisions

- Practice Core owns immutable training content, exact question versions, sessions, first attempts, disclosure ledger, learning state, and recovery. Portal owns presentation and transient unsaved input; Gateway owns verified actor binding and bounded signed HTTP translation. No browser JSON bank or learning-state source of truth.
- Reuse the existing single-choice grader for each listening question and cloze blank. Stable bank/question/blank/segment/part IDs are explicit. A training unit version binds exact immutable bank and ordered question-version references. Every semantic change, including passage, hints, media hashes, timing, evidence, and attribution, changes the unit version. Reordering does not rename stable questions/blanks.
- Listening stores immutable MP3 asset identity, SHA256, duration, provenance/license, and validated sentence ranges. Cloze stores ordered stable text parts and blank references. Evidence refers to stable parts/segments, never mutable text offsets or array positions.
- Additive training catalog/create/recovery/hint/answer/review contracts use explicit bounded routes and exact closed schemas. Freeze OpenAPI and generated clients first. Keep ordinary API envelopes unchanged. Signed guest-capable reads are a narrowly enumerated new boundary, never a generic GET proxy. Preserve command/catalog/write-default gates and migration stop-write rules.
- Whole training sessions are excluded from ordinary question difficulty, personal totals/accuracy/days/mastery, all ranking windows and settlements. Existing sessions default to ordinary. Training still uses canonical immutable attempts and Core learning state.
- Record factual correctness separately from assistance. Independent correct requires a correct first answer and no prior applicable hint. Assisted correct is completed with help and remains review-required. Incorrect is review-required. Fresh independent success may clear training review. Live updates, deterministic rebuild and reconciliation use the same rule; no historical attempt or settlement rewrites.
- Reject training banks/versions/sessions through ordinary random/chapter/difficult/favorites create and ordinary answer paths. An alternative URL cannot bypass the training policy.
- First submission is immutable, including a replay with another key or changed answer. Re-practice creates a fresh session. This release does not add guided resubmission that overwrites the first result.
- Portal submits the existing per-question transaction pattern with stable per-question idempotency keys. Recovery obtains persisted attempts and retries only missing items. Core decides completeness from exact session membership. A saved-answer receipt never includes shared analysis/transcript/evidence. Shared review is authorized only after every pinned question has a first attempt.
- Core serializes hint and submit on the same session lock. A pre-answer cloze hint is persisted before text is returned and marks all currently unanswered unit questions assisted. A replay cannot reduce assistance; login claim preserves it. Already committed outcomes do not change. Listening transcript is post-complete only. Post-submit layer reveal is also owner-gated and cannot be unlocked with client flags.
- Use existing Practice shell/components, SubSiteNav, design tokens, content widths and account flow. Paper #F2F0EA, ink #161513, orange #FF4D00 for appropriate fills, #BB3800 orange text on paper. Two purposeful content regions: real audio/evidence and full cloze passage/blank selection. No decorative player, new second navigation, or duplicate QuizPage.
- Ship the lawful North Wind and Sun audio with separate CC BY-SA 3.0 audio and CC BY-SA 4.0 caption/adaptation notices; retain attribution, source and modification records. The campus repair cloze is original project practice. Do not use Wehuster PDF/audio or imply official CET provenance.
- Content is explicitly imported/reviewed/published through the existing owner lifecycle, never scanned into production at startup. Media serving must support real byte ranges and Content-Type. Do not bundle private transcript/keys into public Portal assets or public catalog responses.
- Use the current single formal publication owner, exact origin/main, required artifacts/signing/provenance, backup plus restore rehearsal, approved migration and recoverable rollback. No new credential, rotation, security rule or permission grant. Existing in-place credentials may be validated read-only; the present unconditional credential-config rewrite requires a separately reviewed preservation path before feature merge can trigger publication.

## Acceptance matrix and testing decisions

Test observable contracts at the highest existing seams: real PostgreSQL Core HTTP plus migration runner, Gateway HTTP with signed owner boundary, and real Portal browser. Mock only downstream failures where a real failure would not be reproducible. Each implementation ticket uses a failing behavior test before its minimal production change.

| Area | Required observable acceptance |
| --- | --- |
| Versioning/publication | Exact unit membership; all semantic fields hashed; stable question/blank identity survives reorder; corrected content creates new version; old sessions recover old evidence; invalid input atomically rejected; fresh/upgrade/repeat/down-restore migration paths pass. |
| Security/gates | Guest/account isolation; spoofed actor, raw body tampering, nonce replay, wrong version and foreign session rejected with no facts; invalid login never becomes guest; default-disabled catalog/commands and stop-write gates preserved; legacy routes reject training. |
| Information release | Catalog/create/pending recovery contain no keys, transcript, private hints or answer-bearing shared evidence; partial submission cannot unlock any complete transcript or filled passage; completed recovery only reveals owner-authorized layers. |
| Assistance/scoring | Independent correct, assisted correct, wrong, hint/submit race, replay/new-key and guest-login claim produce one consistent outcome; all ordinary metrics/rankings/settlements and question difficulty equal an ordinary-only baseline. |
| Review/projections | Training wrong/assisted facts survive refresh and deterministic reconcile/rebuild; fresh independent success clears review; wrong review opens the complete pinned context and authored error cause. |
| Recovery | Start→answer→submit→relisten/hint→review→refresh works for both kinds; repeated clicks, lost responses, partial save, network failure, cancel/back and retry do not duplicate or overwrite saved results. |
| Media | Actual MP3 decode, playback progress, speed, exact evidence seeks and sentence loop; changing unit/unmount stops previous playback; missing/error/retry states; HTTP206 and duration/hash match; no fake play state. |
| Content | At least one unit per kind; all options/keys/evidence/translation checked, legitimate attribution/permission record; every listening sentence range checked against actual clip. Existing human-edited source plus offline ASR evidence is recorded separately from the pending new human ear review. Do not claim that review completed. |
| Accessibility/brand | Keyboard/focus/labels, 360px and wide layout, contained passage scroll, no gesture collision, reduced-motion and actual screenshots; independent UI necessity challenge, later fresh visual and UX review. |
| Delivery | Required lint/type/unit/integration/browser/contract/migration checks; independent Standards/Spec/Public-ready Copy on fixed candidate before commit; exact-head required CI then final main CI; backup/restore, actual production audio and both training workflows verified. |

## Task graph

1. Freeze training domain, contracts, generation and acceptance spec (no blockers).
2. One real listening unit end-to-end, safe submission/recovery/metric isolation (blocked by 1).
3. Complete listening evidence/reveal/review loop and browser recovery (blocked by 2).
4. Complete original cloze with context/hints/evidence/review (blocked by 3).
5. Preserve existing release configuration and grants without credential mutation (independent of product tickets; must precede any merge/publication that invokes it).
6. Integrated fixed-head acceptance, legal content publication, formal release and production verification (blocked by 2–5 and human listening acceptance).

One integration branch carries product tickets with separate implementer branches/worktrees and merger review. The release-owner preservation prerequisite remains a separate concern/PR. Product commits must not accidentally invoke a still-mutating automatic release path.

## Out of Scope

Reading, composition, microphone recording, speech scoring, official CET-format claims, bulk third-party PDF/audio imports, a recommendation system, precise long-term diagnostics, alternate local progress databases, arbitrary migration/cutover, new paid services, persistent credentials, security configuration or permission expansion.

## Current checkpoint / Further Notes

Fresh source main: aa534fa58f3def7596fb77b0508de48540b2b7a5. Read-only production check at 2026-11-12 07:27 UTC: both aliases and all 18 HENU application images run c81827f5b2ca4c40cdca3b41ca9badf0fb75730f, 21 containers total; direct/watcher locks have no holders; auto-deploy service is active. This snapshot is not a reservation of a future release window. Recheck before any mutation.

Native persistent-goal tools are not exposed in this execution environment after tool discovery; this spec/ticket graph tracks the same complete implementation-to-production goal, without claiming a native goal or creating an automation.

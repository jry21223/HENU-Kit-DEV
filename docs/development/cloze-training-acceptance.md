# HC-644 cloze acceptance

Scope: [#644](https://github.com/jry21223/HENU-Kit-DEV/issues/644), after listening
HC-643 and the complete CI ancestor f30d03849af1b8d6943daad8c33d870314a861ca.
This slice implements the cloze requirements in
[the complete spec](./english-training-spec.md). Production deployment, content
publication, final whole-feature visual/UX review and new human listening
acceptance remain separate release gates; no production write is performed here.

## Content and ownership

The explicit Workshop package is
`products/quizcraft/go-service/content/english-training/campus-repair.import.json`.
Its original passage of about 220 words has 35 parts, ten exact stable blanks/questions,
two hint levels each, full English/Chinese layers and contextual evidence.
Source, first-party terms, editorial review and exact payload/source hashes are
in the adjacent README. Private keys/hints/full layers remain owner content;
Portal has only transient draft selections and retry identities.

Core uses the existing schema12 tables and single-choice grader. Hints and
answers serialize on the same session lock. Each hint persists its then-unsubmitted
question IDs before returning text. Already saved first answers retain their
outcome. Assisted correct remains review-required; new independent correct
practice clears review through the same live/rebuild/reconcile rule. The entire
training session remains outside all ordinary statistics and rankings.

Gateway validates closed schemas, signed current actor/cookie authority and
kind-specific cross-field facts. A hint response is checked against a fresh
owner recovery disclosure before its text crosses the browser boundary. Complete
review checks the exact pinned questions, contextual part evidence and language
layers; pending/partial units cannot receive shared answers or full text.

## Observable verification

- Real PostgreSQL16 Core HTTP tests: legal import/validate/publish, invalid
  publication atomicity, hint/new-key/same-key replay, skipped layers, guest claim,
  read purity, foreign owner isolation, complete-only review, old-version hints
  and passage identity, hint/answer races with coherent serialized timestamps,
  assisted learning rebuild/reconcile, ordinary-stat isolation and fresh success.
- Real signed Gateway HTTP: default gates, invalid login, tampered/private hint
  readback, malformed cloze bindings/disclosures, pinned evidence/layer mismatch.
- Actual installed Chrome through real Core → Gateway → Portal: both kinds start,
  answer, lose a committed response, refresh/recover missing answers, review,
  language reveal, owner isolation and new independent sessions. Cloze also loses
  a hint response, recovers both hint levels, navigates stable blanks by keyboard,
  checks 360px overflow/touch targets/contrast and returns through review items.
- Existing listening browser suite remains applicable to the shared controller.
  Screenshots include 360px options/context/hints/partial-save/result, wide cloze
  context, and listening 360px/wide/result focus. All are local acceptance data.

Run `bash scripts/test-training-stack.sh` with Go1.26.11, an isolated PostgreSQL
`QUIZCRAFT_TEST_DATABASE_URL`, and `PLAYWRIGHT_CHROME_CHANNEL=chrome` on macOS.
Override `TRAINING_CORE_PORT`, `TRAINING_GATEWAY_PORT` and
`PLAYWRIGHT_TRAINING_PORT` for isolated ports. The script prints the stack log
folder, publishes both packages explicitly in a disposable database, and saves
browser evidence under `apps/portal/test-results`. Its stop file shuts down the
harness and removes the disposable database through the normal test cleanup.

## Listening replay mapping correction

The same immutable MP3 bytes/hash are retained. The formal listening package now
uses the already researched derived replay margins, instead of raw source-caption
bounds. A failing real Core review regression showed old s4 start26500 omitted the
independently checked ASR first-word start26440. Published replay now contains all
five verified word intervals, remains ordered/nonoverlapping and within media
length; actual Chrome checks evidence seek/loop with those ranges. Source caption
history, derived margins and pending new human ear review are distinguished in
the package README and attribution. This is no new human timing assertion.

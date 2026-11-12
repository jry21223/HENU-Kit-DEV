---
status: accepted
amends: 0018, 0021, 0036, 0038, 0052
---

# Practice Core owns immutable English training and disclosure

English listening and cloze are general training, with assistance and whole-unit
context that ordinary exam scores cannot honestly represent. Practice Core owns
Training Unit Versions, exact question membership, first answers, disclosures,
learning state and recovery. Portal owns presentation and unsaved input; Portal
Gateway binds verified actors and translates only the frozen routes. A second
browser question bank or local progress database would split these facts.

A stable Training Unit ID identifies a material through edits. Its immutable
Training Unit Version pins an immutable Bank Version and the ordered exact
Question Versions. Stable Part, Blank and Sentence Segment IDs survive reorder;
evidence references these IDs, never offsets or positions. Every semantic edit,
including options, keys, hints, media hashes/duration/timing, evidence, translation,
attribution and ordering, creates a new version. Existing sessions retain their
original unit and question versions. The canonical hash and atomic validation
rules are in [the contract specification](../development/english-training-contract.md).

Stable Unit/Unit Version and media identities reuse the importer's deterministic
UUIDv5 convention, an explicit content-identity exception to the generic new
public-ID UUIDv4 guidance. New session and disclosure event IDs remain UUIDv4.

The entire Training Session is excluded from ordinary difficulty, totals,
accuracy, days, mastery, every ranking window and settlement. Existing sessions
default to ordinary. Training keeps canonical first attempts and Core learning
state: correctness is factual, assistance is independent. Correct without prior
applicable assistance is `independent_correct`; assisted correct is
`assisted_completion`; incorrect is `review_required`. The latter two require
review. A fresh independent correct first attempt can clear review. Live updates,
rebuild and reconciliation use this same latest-attempt rule. Historical attempts
and settlements are never rewritten. Ordinary create/answer routes reject all
training banks, versions and sessions, including favorites selections.

The unit is submitted as individual immutable first answers, with a stable
idempotency key for each question. Partial saves release only answer receipts;
shared keys, analysis, transcript, filled passage and evidence require **every**
pinned question's first answer. Re-practice creates a new session. Hint and answer
commands lock the same session. A cloze hint is committed before its text returns
and marks every then-unanswered question assisted; a replay or login claim cannot
remove assistance. Already saved outcomes stay immutable. Listening text is
post-completion only. English then Chinese reveals are persisted, monotone,
owner-gated commands; client flags cannot authorize disclosure.

The additive [owner](../../packages/api-contracts/openapi/quizcraft-training.yaml)
and [browser](../../packages/api-contracts/openapi/portal-training.yaml) contracts
leave ordinary envelopes unchanged. The public training catalog has no actor or
learning facts. Only the exact training session, review and review-items GETs
extend signed reads to a Core-issued guest cookie. They reuse the existing
independent read credential and optional sixth actor HMAC line; no generic GET
proxy, learner grant or new credential is authorized. GET never creates a guest
or claims learning records. Account/guest claim remains an existing write-command
transaction, preserving session-scoped hint and reveal history. All writes retain
the default-off command/write gates and migration stop-write rule.

The actor-owned review-items index comes from Core learning state and points to a
pinned session for full context. An ordinary catalog or a current-session results
panel cannot substitute for this durable review loop. Publication remains explicit
through the existing owner lifecycle, with valid provenance and actual media; no
startup import or frontend fallback is allowed.

The browser review-items list is a narrow exception to the public page/page_size
rule in [API communication §7](../development/api-communication-spec.md): only
`GET /api/v1/practice/training/review-items` uses an actor-bound opaque cursor and
page_size (default 20, maximum 50). This personal chronological review stream
needs a server-owned continuation over immutable unit-version references without
deep offsets. The cursor is bounded to 512 characters and carries no browser
selection authority; ownership is verified on every page. The view is live and
restarts after learning mutations. This does not change pagination for any other
public list or allow arbitrary signed queries.

HC-641 freezes these decisions and generated contracts only. It does not install
handlers, tables, content, permissions, runtime flags or production configuration.
Runtime acceptance belongs to the dependent implementation tickets in
[the complete release spec](../development/english-training-spec.md).

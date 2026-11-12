# English training contract freeze (HC-641)

Sources: [#641](https://github.com/jry21223/HENU-Kit-DEV/issues/641), parent
[#640](https://github.com/jry21223/HENU-Kit-DEV/issues/640),
[full executable spec](./english-training-spec.md),
[ADR-0060](../adr/0060-core-owned-english-training.md). This slice declares
contracts and acceptance constraints; the routes and storage below are not yet
implemented. The full spec's acceptance matrix remains the implementation gate.

## Exact routes and authority

The owner prefix is `/api/v1/portal/practice/training`; the browser prefix is
`/api/v1/practice/training`. These eight method/path pairs are the entire façade.
Owner and browser use the same closed success shapes and `{request_id,data}`
envelope; errors use `{request_id,error:{code,message}}`. Ordinary Practice and
Gateway contracts and envelopes are unchanged. Browser clients call Gateway only.

| Method / suffix | Success data | Authority / gate |
| --- | --- | --- |
| GET `/catalog` | TrainingCatalog | Public anonymous signed read; existing Gateway catalog gate. |
| GET `/review-items` | TrainingReviewItems | Verified account or existing anonymous owner; existing V2 read gate. |
| POST `/sessions` | TrainingSession (201) | Existing command/write gates, selected published version, idempotency. |
| GET `/sessions/{session_id}` | TrainingSession | Verified account or existing anonymous owner; existing V2 read gate. |
| POST `/sessions/{session_id}/answers` | TrainingAnswerReceipt | Owner, pinned membership/version, command/write gates, idempotency. |
| POST `/sessions/{session_id}/hints` | TrainingHintDisclosure | Owner, cloze only, sequential hint, command/write gates, idempotency. |
| GET `/sessions/{session_id}/review` | TrainingReview | Owner, all first answers present, existing V2 read gate. |
| POST `/sessions/{session_id}/reveals` | TrainingLayerDisclosure | Owner, complete unit, sequential language layer, command/write gates, idempotency. |

Every success is `Cache-Control: private, no-store`; JSON is UTF-8
`application/json`. Errors are also private/no-store. No wildcard or legacy
Portal API fallback is allowed. All bodies reject unknown fields. Training option
answers are a zero-based integer index, preserving the existing single-choice
grader. Core validates the index against the pinned options, not just schema's
0..7 bound. These schemas contain no untyped answer or arbitrary JSON payload.

The existing credentials are reused, without provisioning or rewriting them.
The catalog uses the read credential, permission `portal.practice.read`, scope
`product`, product `quizcraft`, and no actor or guest cookie. The other three
GETs are an **exact** new signed guest-capable read allowlist, using those same
service assertions. Commands use the existing separate command credential.
Each owner request requires Basic credentials, `X-Key-Id`, `X-Timestamp`,
`X-Nonce`, and `X-Signature`. The HMAC canonical string is the existing five lines
`METHOD\nREQUEST_URI\nTIMESTAMP\nNONCE\nSHA256(raw_body)`; append
`\nX-Actor-User-Id` only for a verified account UUID. Timestamp/nonce validation
and replay rejection are unchanged. A retry gets a fresh service nonce while
preserving its browser command idempotency key and exact command body.

Gateway takes account identity only from the valid unexpired encrypted Portal
Session; invalid/tampered/expired identity is 401, never guest fallback. For guests
Gateway relays only `quizcraft_anonymous`; Core verifies that cookie and session
ownership. It never forwards a browser-supplied actor or arbitrary cookie jar.
Anonymous-cookie forwarding/reissue retains ADR-0018's Secure, HttpOnly,
SameSite=Lax, Path=/, no-Domain constraints. Unknown, missing or foreign session
IDs all return 404 with `NOT_FOUND`, without confirming existence. A valid account
plus its original guest cookie may prove access to that guest session read-only;
GET does not create an anonymous identity, claim a session, increment learning
facts, or disclose a new layer. Claim can occur only in the existing authenticated
write-command transaction. Claim retains all prior assistance/disclosures and
first-answer facts; an unrelated account/cookie cannot claim or access them.

No query is allowed except `review-items`' single `cursor` and `page_size`.
Page size is 1..50, default 20. Cursor is opaque base64url, 1..512 characters,
bound to the actor and last `(last_attempt_at,unit_version_id)` tuple. Sort is
descending by that tuple, with a stable UUID tie-break. Gateway signs canonical
query in the fixed order `cursor`, `page_size`, RFC3986 encoded, excluding omitted
parameters; duplicates, unknown keys, malformed cursors and zero/oversize sizes
are 400. Core validates the same URI. The index is a live learning-state view,
not a historical snapshot; callers restart at page 1 after learning mutations.
Empty `next_cursor` means the final page; no silent truncation is allowed.

`PORTAL_ENABLE_QUIZCRAFT_CATALOG` still defaults off (catalog 404 when disabled).
`PORTAL_ENABLE_QUIZCRAFT_V2_READS` still defaults off for the three private reads.
`PORTAL_PRACTICE_COMMANDS_ENABLED`, `QUIZCRAFT_PORTAL_COMMANDS_ENABLED` and
`QUIZCRAFT_WRITES_ENABLED` retain their existing off defaults and protect every
training command. Core's migration stop-write gate also covers hints/reveals and
claim. No new runtime flag or enablement change is made by this contract commit.
Disabled commands/reads or stop-write yield 503 `TRAINING_DISABLED`; no facts are
written. Reads do not bypass a disabled read gate merely because writes are on.

## Immutable authored content and publication

`TrainingUnitVersion` is an owner-only authored representation for the existing
explicit import → validate → publish lifecycle. No learner endpoint returns it.
It contains the safe unit plus private question review, authored hint levels,
sentence timings, and English/Chinese layers. Safe catalog/session schemas are
separate closed projections, never that private object with fields hidden by UI.
An importer must fail the whole transaction before any version/member/publication
write when any of the following invariants fails:

- A unit has 1..20 distinct stable questions, exactly one immutable Bank Version,
  and exact ordered Question Version references that belong to that bank version.
  Each question is single-choice with 2..8 nonempty options and a valid key index.
  The public options/content equal the pinned Question Version. No training bank,
  version or session may enter ordinary create, favorites-create or answer paths.
- Stable unit/question identities survive semantic edits; blank/part/segment IDs
  are authored stable IDs, independent of text hashes and array order. They are
  unique within the unit. Reordering changes the version/hash, never these IDs.
- Cloze parts form the complete ordered blanked passage. Every blank part refers
  to exactly one declared blank, every blank occurs exactly once and maps to one
  unique pinned question, and every pinned cloze question has exactly one blank.
  Evidence refers only to existing stable parts; translations/filled passage
  preserve those part IDs. Each hint descriptor has exactly its declared 1..3
  authored levels with matching unique hint ID. Listening has no pre-answer hints.
- Listening has one immutable MP3 asset: stable asset ID, SHA256 of actual bytes,
  `audio/mpeg`, actual duration 1..1,800,000ms, and a content-addressed public URL
  `/practice-media/{sha256}.mp3` whose digest equals the declared SHA256. Serving
  actual media supports byte ranges (206, Content-Range), correct Content-Type,
  and full-body 200; this is a public lawful asset, not a signed owner JSON read.
  Segment ranges use integer milliseconds with `0 <= start < end <= duration`;
  segments are ordered, non-overlapping complete sentences. Evidence segment IDs
  exist and belong to this exact media version. No manufactured timing fallback.
- Every question has exactly one review record for the exact question version;
  evidence kind matches unit kind. Every referenced segment/part exists.
  Both language layers exist once each, ordered English then Chinese, with exact
  matching segment/part IDs. Listening text layers cover all sentence segments;
  cloze text layers cover all passage parts, including filled blank text.
- Attribution records are nonempty, HTTPS-sourced, component-specific and include
  license identifier/URL, attribution, modification statement (possibly empty)
  and an explicit originality claim. Every present audio/text/translation component
  has a record. North Wind and Sun audio CC BY-SA 3.0 and caption/adaptation
  CC BY-SA 4.0 remain separate records. Original campus cloze records identify the
  project author/source and `LicenseRef-HENU-Kit-Service-Terms` with
  `https://henukit.cn/terms` and the first-party use basis. Do not grant a new CC
  license on the owner's behalf. Open-license audio/captions/translation retain
  their marked reuse rights without adding a noncommercial restriction. No
  component implies official CET provenance.

The canonicalizer is the existing Go `hashCanonical` convention: decode the
schema-valid JSON object, then use `encoding/json.Marshal` and SHA256 its UTF-8
bytes (object keys lexically sorted, default Go string/HTML escaping, arrays in
authored order, integer numbers in decimal, no whitespace or Unicode
normalization). Training semantic numbers are integers at most 2^53-1, so the
existing decode/marshal numeric round trip is exact. Omitted optional fields stay
omitted; an explicitly present field cannot be dropped as a zero value. This
uses the repository's deterministic normalizer, not an additional RFC8785 library.

Training enters **only** the existing Workshop bank import endpoint:
`POST /api/v1/quizcraft/workshop/banks/{bank_id}/imports` at Console Gateway and
`POST /api/v1/console/workshop/banks/{bank_id}/imports` at Core (the older local
Workshop import uses the same BankImportRequest). The existing owner-authorized version
detail read adds optional `WorkshopVersionDetail.training: TrainingUnitVersion`
for operator review of resolved private content. It is absent for ordinary
versions; no learner endpoint uses this response. `BankImportRequest.training`
is an additive optional `TrainingAuthoring`; when present the whole request must
also satisfy closed `TrainingBankImportRequest`, including 1..20 typed
single-choice questions, required expected_version, integer keys, and no unknown
fields. This is a request extension only; existing import reports/envelopes are
unchanged. The management contract declares OpenAPI 3.1 so its shared source-input
schema is validated consistently. Runtime typed decoders/validators must support
this input before any implementation claims training import support.

The importer sequence, in one transaction with existing idempotency/lifecycle
locks and the existing `quizcraft.workshop.write` bank Scope, is:

1. Validate the complete source input, source references, real asset metadata,
   question shapes and provenance before writing content. Training authoring
   refers to `source_unit_id` and `source_question_id`, never generated UUIDs.
   It includes parts/blanks, authored hints, sentence ranges, evidence and both
   language layers. Keys/analysis come from the typed questions. A training bank
   must be new/empty or already training; do not convert a bank with ordinary
   content/history. Reject ordinary version/create/import paths for a training
   bank when the training input is absent, rather than silently drop its binding.
2. Preserve `training` when `importWorkshopDraft` reconstructs its strict bank
   source. Extend the strict `bankDocument` decoder with that closed typed field.
   Existing normalized question semantics and question-version hashing stay the
   same. Extend the existing `bankSemantic` hash object with `training` only when
   present (using decoded authored source fields, not generated IDs). Merely
   adding it to raw source bytes or `source_sha256` is insufficient: current
   `bankSemantic` hashes only bank key/name/source version/question reports.
   Ordinary imports omit the new key and retain their exact existing hashes.
3. Derive existing stable Bank/Question IDs and exact Question Version IDs with
   the existing importer. Derive Bank Version from the extended semantic hash.
   Resolve all source references using `ImportReport.Questions` inside the
   existing `beforeCommit` hook. Derive stable Unit ID with existing UUIDv5 helper
   `stableID(bank_id,"training-unit:"+source_unit_id)`; derive media asset ID as
   `stableID(quizcraftNamespace,"training-media:"+sha256)` and its URL from digest.
4. Build the full resolved owner `TrainingUnitVersion`. Its fingerprint hashes
   that whole representation, omitting only `unit.unit_version_id` and
   `unit.content_sha256`. It includes all other IDs, exact bank/question versions,
   order, keys, hints, media, evidence, translations and attribution. Unit Version
   ID is `stableID(unit_id,"version:"+fingerprint)`. Bank hashing used only source
   input and question reports, so no Bank/Unit Version hash cycle exists.
5. Persist immutable unit/version/member/disclosure definitions in `beforeCommit`
   with the bank draft and report. Any validation, source lookup, conflict or
   persistence failure rolls everything back. A bank version has exactly one
   training unit version; one stable source_unit_id belongs to that training bank.
   Repeat existing import requests obey existing lifecycle/idempotency conflicts;
   they never replace immutable bindings. Only media/hint edits still change the
   Bank Version because all authored source participates in its semantic hash.

Existing `validate`, `publish`, `unpublish`, and `rollback` commands and their
permissions remain the only publication lifecycle. Validate checks the complete
training binding and real media/provenance; publish requires its validated state
and atomically activates bank **and** its exact bound unit. Unpublish removes
new-session eligibility without deleting content. Rollback atomically selects the
previous validated exact bank/unit binding with existing optimistic lifecycle
version checks. No new grant, credential, management endpoint or auto-import is
introduced. Import responses already provide Bank Version ID and question reports;
Unit/Unit Version IDs can be deterministically resolved above. Old sessions pin
both exact bindings; unpublication or a newer version never changes their
recovery/review, and their media must remain available.

Logical persistence required by the next implementation ticket is an immutable
unit-version record plus ordered exact member records, a training discriminator
on sessions (all existing sessions default ordinary), session-bound disclosure
ledger and canonical per-question first attempts. A disclosure includes stable
ID, requested hint/language layer, timestamp and affected question IDs. Its content
is recoverable from the pinned version. These are schema constraints to implement,
not permission to create tables or run migrations in HC-641.

## Exact state projections and concurrency

| State | Session recovery | Answer receipt | Review / reveal |
| --- | --- | --- | --- |
| Pending | Safe unit, zero receipts, saved_count=0, total_count=N, committed cloze hint disclosures only; revealed_layers empty. | Absent until first commit. | Review/reveal 409 UNIT_INCOMPLETE. No keys, transcript, shared evidence or filled passage. |
| Partial | Same safe unit, exactly saved_count unique receipts (1..N-1), persisted hints and assisted outcomes; revealed_layers empty. | Only that saved first answer, correctness, assistance/outcome, timestamp and current counts/phase. | Same denial. A correct partial answer never unlocks cross-question shared content. |
| Completed, no text reveal | Safe unit, N unique receipts, phase=completed, hints; revealed_layers empty. | Still excludes expected_answer/analysis/evidence/text. | Review returns all N keys, authored analyses/error categories and stable evidence; listening sentence ranges contain no transcript. layers empty. |
| Completed, revealed | Same safe recovery plus monotone revealed_layers names. | Remains the same safe shape. | Review includes exactly committed layer disclosures/text; reveal returns only its committed requested layer. English must precede Chinese. |

`N` is exact pinned membership, never a client-provided completion flag or a
count of arbitrary submitted IDs. Receipts match session/unit/question versions;
answer facts are immutable. `saved_count` and `phase` in recovered receipts are
refreshed to the current session snapshot (they are response metadata, not facts).
`replayed` is also response metadata: false for a newly committed answer, true
for an answer-command replay and for receipts hydrated by session recovery.
Answer/correctness/assistance/outcome/review_required/submitted_at never change.
Recovery sorts receipts in pinned question order, hint disclosures by commit time
and disclosure UUID, and revealed language names in English/Chinese order.
The create response has the same safe schema and zero saved answers; an idempotent
create replay is the current recovery snapshot of the original session, not a new
session. No request may select another actor, assistance or completion status.

Commands serialize on the same Core session transaction lock. For a cloze hint,
validate owner/version/next level, persist disclosure and mark **all currently
unanswered pinned questions** assisted, then return text only after commit.
Committed outcomes are unaffected by a later hint. A repeat level returns the
same disclosure; requesting a skipped level yields 409 LAYER_LOCKED. The level
number is one-based and scoped to its stable hint ID. Every pending answer carries
forward any applicable session assistance. Reveals similarly persist one monotone
English/Chinese disclosure after completeness and ownership checks. Listening
cannot invoke the hint endpoint. Playback, seek, speed and looping are transient
media operations and do not mark assistance.

Idempotency scope is `(actor, method, exact route, key)` with exact raw-body digest.
Keys are 16..160 characters. Same scope/key and body replays the original immutable
fact; changed body yields 409 IDEMPOTENCY_CONFLICT. Independently, a unique first
attempt is enforced for `(session_id,question_id)` regardless of key or login
claim. New key + same answer returns that fact with `replayed=true`; new key +
changed answer yields 409 FIRST_ANSWER_IMMUTABLE. A lost response is recovered by
reading the session and retrying only missing questions with their original keys
and exact bodies. A key conflict never overwrites a first answer. Re-practice
always creates a fresh session/key. Hint/reveal replay resolves to the committed
disclosure and never reduces assistance. No automatic retries change actor or key.

For a correct first answer, `assisted=false` means `independent_correct` and
`review_required=false`; `assisted=true` means `assisted_completion` and
`review_required=true`. Incorrect always means `review_required` outcome and
`review_required=true`, retaining its factual assistance bit. Training still
records canonical attempts and Core learning state. Ordinary difficulty,
personal totals/accuracy/days/mastery, all ranking windows and settlements exclude
the entire session in live updates, deterministic rebuild and reconcile.

The review-items index groups current review-required learning facts by pinned
unit version. It returns one item per version, a count of distinct unresolved
questions, and the most recent contributing session reference; that session has
the entire pinned unit and the returned phase. Partial sessions resume safely;
completed ones can read review. Counts use Core's latest-attempt ordering and
assisted-success rule, never UI-derived diagnostic categories. A later independent
correct first attempt for the same stable question clears its unresolved state
and can remove the item; it does not rewrite the old receipt. Choosing an index
item restores its pinned session. Re-practice explicitly chooses the current
published version from the catalog and creates a new full session; it does not
mutate old versions or silently replace unavailable versions. Guests use only
the existing browser cookie; account state follows the account after Core claim.

## Failure and acceptance requirements

400 `INVALID_REQUEST`: invalid JSON/unknown fields/query, invalid option or member.
401 `UNAUTHENTICATED`: invalid Portal login or missing/invalid service signature.
403 `FORBIDDEN`: invalid service read permission/product scope.
404 `NOT_FOUND`: unknown/foreign session, missing anonymous identity, or dark catalog.
409 distinguishes `VERSION_UNAVAILABLE`, `UNIT_INCOMPLETE`, `LAYER_LOCKED`,
`IDEMPOTENCY_CONFLICT`, `FIRST_ANSWER_IMMUTABLE`, and `SERVICE_REPLAY`.
503 distinguishes `TRAINING_DISABLED` and `DEPENDENCY_UNAVAILABLE`.
Gateway owner network/contract/authentication failures are 502
`DEPENDENCY_UNAVAILABLE`, never a guest retry, mock empty state or login prompt
for a valid browser actor. Errors contain no hidden content or foreign IDs.

Run `bash scripts/generate-training-contract.sh` for pinned Go owner types,
Gateway owner client and Portal browser client. Generated payload types are not a
runtime validation substitute: implementers must validate against closed schemas
plus the cross-field invariants above before forwarding/committing/returning.
Schema tests prove valid saved receipts and rejection of shared review fields;
lint, generated compilation and drift prove a consumable declaration. They do
not prove runtime locking, persistence, media or security. The dependent tickets
must test these through real PostgreSQL Core HTTP, signed Gateway HTTP and real
Portal browser seams as required by the full executable acceptance matrix.

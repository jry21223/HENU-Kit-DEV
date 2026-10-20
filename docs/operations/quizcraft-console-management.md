# Console question-bank management: rollout and recovery

Status: implementation candidate; not a production release receipt. See ADR-0050
and `../development/quizcraft-console-management-spec.md`. The material library
and question-bank publication remain separate operations.

## Read-only baseline (2026-10-20)

Source was freshly fetched at `37225bd5a40456b12bf31924dc2b114598752592`;
this implementation must first merge and resolve a new exact current-main SHA.
Production full-stack alias and both affected images were on
`11bf9e656b1253caddbd60afc9e77f5a07a812e4`; Portal/Gateway/Food also have a
separately recorded scoped release. Do not call the full stack current-main.

The Platform database has `console-gateway` registered, with the live Console
callback `https://console.henukit.cn/api/v1/auth/callback`. All eight Console
runtime configuration inputs are populated. Three `quizcraft.workshop.*`
permission codes are active and one existing operator grant has product scope
`quizcraft`. No account, role or grant needs to be created for this bridge.
At inspection there was no unexpired Console exchange Session; the operator must
sign in normally. The exchange token is client-bound to `console-gateway` and
cannot be checked using a different OAuth client ID.

No `quizcraft` OAuth client was registered. The running QuizCraft container lacks
legacy Platform/OAuth configuration. Anonymous Workshop access returned 401.
Source `workshopActor` would return 503 `authorization_unavailable` for a signed
legacy Session when Platform is absent; that 503 is a source/config conclusion,
not an observed request made with a fabricated token. Current code has neither
product login/callback routes nor a cookie encoder. The old provision script's
`/auth/callback` is not a supported recovery route.

## Review and release gates

1. Review the two service changes, separate OpenAPI contract, ADR and focused
   security/lifecycle tests. Require zero Standards/Spec/Public-ready Copy and
   security findings. No local code or branch is a releasable source.
2. After authorized merge, fresh-fetch and bind clean HEAD, current origin/main,
   mandatory CI and candidate artifact provenance to the same full SHA.
3. Use `henukit-artifact-deployment.md` and the production-release skill for the
   supported successful Actions artifact path or controlled WSL2 Linux/amd64
   signed builder fallback. Never build on production or install a branch.
   The Food-only scoped helper does not authorize a new two-service scope.
   The full formal runtime carries the trusted fixed-SHA image inventory.
4. Capture the current alias/image cohort, healthy retained rollback runtime,
   exact environment checksum/owner/mode, disk floor, materials control-plane
   state and backup/restore evidence. The existing Platform/Account release
   gates still apply. Also snapshot `quizcraft_v2` with verified restore-list
   evidence before any content write; keep learning data on a content rollback.
5. Verify signer/attestation, manifest SHA, archive and inner hashes before
   extracting. Activate only through the exact-SHA repository release helper.
   This change needs no database migration and no Platform grant operation.

## Configuration (both services default off)

Only add approved keys to the root-owned mode-0600 production environment after
making a recoverable checksum-bound backup. Preserve all unrelated bytes.
Application HMAC credentials are separate from release-signing keys; never put
either in artifacts or logs. Use a new dedicated management identity/secret,
not an OAuth, summary, catalog, Portal-command or browser-Session key.

- `QUIZCRAFT_CONSOLE_ENABLED=0` is the default for ordinary deployments.
  The separately reviewed [Practice scoped release](henukit-practice-scoped-release.md)
  enables only its selected candidate after its backup/restore gate and retains
  the original global environment for rollback (ADR-0051).
- `QUIZCRAFT_CONSOLE_CLIENT_ID`: a dedicated internal management caller name.
- `QUIZCRAFT_CONSOLE_API_URL=http://quizcraft:10089`: Gateway's exact Compose owner.
- `QUIZCRAFT_CONSOLE_KEY_ID` and `QUIZCRAFT_CONSOLE_SECRET`: Gateway's active signer.
- `QUIZCRAFT_CONSOLE_KEYS_JSON`: Owner key-ID-to-secret JSON object. It may retain
  a previous dedicated key during rotation. Its active entry must equal the
  Gateway's active secret. Do not print the object to compare it.

Compose explicitly forwards the Gateway fields only to Console Gateway and the
key ring only to QuizCraft. The `prebuilt` overlay inherits these exact-source
settings; it continues to pin images and forbid builds. Validate rendered Compose
with `config --quiet` without logging its secret-bearing output. Verify default-off
404 and enabled authentication rejection. Both services must have the same reviewed enablement state;
partial configuration fails closed. No production change was performed while
preparing this runbook.

## Accepted API journey

The operator signs in through the existing Console OAuth flow. Do not read
Session encryption keys, synthesize cookies/JWTs, copy exchange tokens, or share
Platform client credentials to manufacture a management caller. Browser requests
use the normal HttpOnly Console cookie. Writes require exact same-origin HTTPS
`Origin`, `Content-Type: application/json`, and a 16–160 character
`Idempotency-Key`. Responses are `Cache-Control: no-store`.

Console path prefix: `/api/v1/quizcraft/workshop`.
Owner's internal prefix: `/api/v1/console/workshop`.
The latter accepts only dedicated signed service requests; browser credentials
are never forwarded. Its signature binds method, URI, timestamp, nonce, raw body,
actor, exact permission, requested bank, and idempotency key. Replay uses the
existing durable nonce table. Owner and Gateway enforce 2 MiB request bodies;
full version responses are bounded to 8 MiB. Do not truncate an oversized bank.

1. `GET /catalog`: capture stable bank key/UUID, lifecycle and active version.
2. New bank only: `POST /banks` with `bank_key,name`. Reuse existing bank keys
   for existing content. Record the returned stable bank UUID.
3. `POST /banks/{bank}/imports` with explicit `expected_version`, source file
   SHA256 in `source_sha256` and full `questions`. Map local `id` to `source_question_id`, selecting
   only supported fields. Local audit metadata is not an import request.
4. `GET /banks/{bank}/versions/{version}`: read every question, option, answer,
   analysis, chapter and source ID. Verify source counts, omissions, duplicate
   mapping, diagrams and answer/analysis agreement; preserve evidence.
5. Only after actual full review, `POST .../{version}/validate` with refreshed
   `expected_version` and an honest review note. Automation must not claim
   human review that did not occur.
6. `POST .../{version}/publish` with the next lifecycle version. Retry unknown
   outcomes with the same exact body/idempotency key and a fresh service nonce.
   Changed bodies require a new key. A stale version is a stop/refresh result.
7. Read the public Portal catalog, start practice, submit an answer, and verify
   the stored answer result and analysis for the published immutable version.
   A health response, imported draft or successful validation is not publication.

Read actions require `.read`; create/import/validate require `.write`;
publish/unpublish/rollback require `.publish`. Listing/creation use product
scope; bank actions use exact resource scope, which an existing product grant
may cover under Platform Core's existing policy. Each call checks live Session
revocation and grants through Platform Core before the owner is contacted.

## Existing identity and recovery

Keep `bank_key` and existing `source_question_id` verbatim. These derive stable
bank/question UUIDs; changing analysis creates an immutable question version
while learning identity remains stable. Never replace all IDs with row positions
or content hashes. New source documents require an explicit dedup/source mapping.

The two existing H3C banks had no Workshop state for their migrated active
versions. Therefore those UUIDs are not automatically eligible rollback targets.
Before corrections, capture the owner database's active questions in a read-only snapshot, import the
same questions and source IDs as a Workshop baseline, read all fields, validate
and publish that baseline. Preserve its validated version as the rollback target.
The frozen Workshop version GET requires a Workshop state and cannot export a
legacy active version. The initial snapshot is therefore an operational read-only
owner-database export, bound to bank UUID, active-version UUID, lifecycle, ordered
source IDs and a SHA256 receipt. Read no user, Session or credential tables.
No SQL mutation is permitted.

This may create a new bank version while reusing identical question identities
and contents; old practice sessions retain their immutable versions. Stop if the
baseline differs. Do not insert validation state or active versions with SQL.

On failed application acceptance, restore the previous exact runtime and captured
environment using the formal release rollback contract; verify alias, images,
health and materials state. Disable this bridge to close management without
turning off Portal practice. Application rollback does not undo a published bank.
For a new bank use `.../{version}/unpublish`; for corrections use
`POST /banks/{bank}/rollback` with the validated baseline version and current
`expected_version`. Keep all immutable content, audit and learning facts. Never
restore an old whole-database snapshot over new student attempts as a shortcut.

# Food Post author editing

## Scope and existing rules

Students can currently create and list their own Food Posts, but only Console
operators can correct them. This slice adds author editing to the existing
Food-owned Post flow. ADR-0032 makes Posts public immediately; legacy
Submissions retain their separate review workflow. No new review state,
credential, guest claim, delete command, or moderation permission is added.

## Contract

- `GET /api/v1/food/posts/{id}/edit` returns structured editable fields,
  current version and retained image positions only to the verified author.
- `PUT /api/v1/food/posts/{id}` replaces those fields, dishes and selected
  photos. It requires `expected_version` and an actor-scoped idempotency key.
  Existing image positions explicitly retained by the author precede new images.
- Portal Gateway takes the actor exclusively from the Portal Session. Food
  independently checks `author_user_id`; missing/other/guest-owned Posts return
  the same 404. Reads use the existing read credential, writes the existing
  create credential. Public and Console credentials cannot write this route.
- Ownership, author display name, creation time and hidden state are immutable
  through author editing. Hidden Posts reject author reads/writes. A concurrent
  Console correction or hide bumps the shared version and blocks a stale save.
- Same key + same request replays the committed response; changed body, target
  or command with that key conflicts. Updates do not consume the daily create
  quota. Fields, images, version and replay response commit atomically.
- Existing create limits and image sanitization apply. Image URLs include a
  content digest so replacement/removal cannot reuse a cached photo URL.

## Acceptance at public seams

1. Food HTTP + real PostgreSQL/Redis: author read and text/dish/photo updates;
   foreign/anonymous/credential rejection; unknown fields and invalid values;
   hidden/Console concurrency; same-key replay, changed-key conflict and
   concurrent version conflict; rollback on persistence failure; daily cap
   unaffected; old photo URL never serves replacement bytes.
2. Gateway HTTP: session-bound actor, browser actor spoof ignored, exact
   allowlisted routes, create/read credential separation, idempotency/body
   bounds, owner errors and no-store private responses, no fallback.
3. Portal browser/form: entry from My Posts, structured prefill, validation,
   keep/remove/add photos, single in-flight save, preserved draft and same key
   on uncertain failure, stale-version reload without silent overwrite,
   login/not-found/unavailable states; desktop and 360px layouts.

## Delivery and release

One Draft PR based on fresh main, independent Standards/Spec/Public-ready Copy
reviews, affected contracts/lint/typecheck/unit/integration/browser/build CI.
The user subsequently authorized merging and deploying this repair from their
Mac. Use existing trusted artifact and activation gates; unrelated open PRs
remain separate. Never mutate another person's real Post during acceptance.

Rollback uses the retained fixed-SHA application release. This slice requires
no schema migration and preserves the existing Console version column and
idempotency ledger.

## Affected-service build baseline

The required govulncheck scan found nine reachable standard-library advisories
with Go 1.26.6. The same Food and Gateway source scans with Go 1.26.9 report
zero reachable vulnerabilities. Pin only these two affected service modules and
build images to the patched toolchain; third-party dependency versions remain
unchanged. This is a release prerequisite, not a new product capability.

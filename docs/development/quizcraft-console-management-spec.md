# Console question-bank management: bounded implementation spec

The existing Console login establishes the operator. Platform Core checks the exact
`quizcraft.workshop.read`, `.write`, or `.publish` permission against QuizCraft
product scope (listing and creating) or the requested bank resource scope on every
request. Console Gateway signs only the existing Workshop actions for the owner.
QuizCraft verifies a separate management key ring, timestamp, nonce, raw body,
operator UUID, permission and resource before reusing its Workshop lifecycle.
No independent OAuth entry, account, role, grant, test claim, or direct activation
is introduced. Both service configurations default off.

Agreed public test seams: Console HTTP session/permission refusal and forwarding;
QuizCraft HTTP signature tampering/replay refusal and create → import → full
version read → validate → publish, including stale lifecycle and idempotency.
Console imports require an explicit non-negative `expected_version`; the frozen
legacy endpoint's omitted-version compatibility is unchanged. Writes use the
owner's existing 16–160 character idempotency-key boundary.
Unauthenticated, denied, stale, malformed, oversized and replayed requests fail
closed. A browser cannot select an actor or service credential. Cookie-authenticated
writes require a matching HTTPS Origin. Management responses carry no secret.

The owner keeps bank/source-question identity and immutable versions. Existing
migrated active versions without Workshop state are not assumed rollback-ready.
Before updating one, import its exact questions as a Workshop baseline, fully
read it, validate and publish it, and retain its validated version for rollback.
This creates no new question identities when source IDs remain unchanged.

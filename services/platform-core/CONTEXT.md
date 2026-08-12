# Platform Core Context

## Owns

- Platform users and their account status.
- 30-day absolute Core Sessions on the account origin and revocable client exchange Sessions: eight hours by default for high-privilege product work, with the `portal-gateway` client extended to 30 days so the Portal Session survives the full Core Session window.
- Registered OAuth clients, exact callbacks, PKCE challenges, and single-use Authorization Codes.
- Permission codes, authorization roles, user Scope grants, authorization revisions, and authorization audit events.
- Verification-code security facts and the encrypted critical mail Outbox.
- Normalized Email Identity lookup hashes, encrypted email content, Display Names, HENU Kit Password Credentials, and atomic registration.
- The Account Center registration, login, recovery, and credential-management boundary.
- Operations Inbox coordination metadata and immutable source-resource references; source product content remains with its owner.
- PostgreSQL identity facts and Redis-based short-lived coordination for this context.

## Does not own

- Console Session cookies or Console Gateway state.
- Product-local sessions, product content, or product database credentials.
- Study Legacy users, roles, routes, or API compatibility behavior.

## Current boundary

HC-05 through HC-08 establish identity authorization, verified-email mail delivery, and reference-only Operations Inbox coordination. Registration now requires a HENU Kit Password Credential and Display Name to commit atomically with the verified Email Identity and Core Session. Password recovery requires a fresh Email Verification Code, revokes all old Sessions, and issues one new Core Session; an authenticated password change additionally requires the current password, retains only the current Core Session, and revokes every other Session. Temporary email, IP, and device failure counters escalate password authentication to email-code login and fail closed when Redis is unavailable. HC-12 adds `platform.operations.read` and `platform.operations.write`, both requiring platform Scope, plus a bounded operational snapshot and audited mutation APIs for Session revocation and optimistic account status/role/Scope replacement. HC-170 registers the exact `account.membership.write` permission; it is usable only through an active role grant carrying product Scope `account-portfolio` and does not grant points or payment authority. The root-only initial-operator CLI grants only Platform Operations and QuizCraft Workshop scopes and writes an immutable audit. Durable idempotency records distinguish replay, conflicting payloads, and unknown outcomes; append-only operation audits accompany the existing authorization audits. Responses omit Session hashes/tokens, recipient ciphertext, provider identifiers, mail secrets, verification secrets, and source-product content. HC-356 makes Platform Core the owner of the Console-facing identity pair: exact account lookup and operational snapshot rows return Display Name plus normalized email, while a bounded batch resolution seam serves Account ticket rows under the caller's exact ticket permission and product Scope. HC-357 adds a maximum 20-account membership identity page under the exact `account.membership.write` product grant; Display Name search is a case-insensitive substring and email search is an exact normalized match. Display Name remains optional for legacy rows, while UID stays a resource identifier rather than a human label. Lookup rate limiting and response-time equalization for misses remain caller-keyed.

The Account payment release uses a separate Platform Core-owned command to
grant the exact eight Account Console permissions to one explicitly named
active role, increment its authorization revision, and append an immutable
release audit. Deployment scripts never write authorization tables directly.

## Language

**Account Center**:
The sole user-facing boundary for registration, login, account recovery, and authentication-credential management across HENU Kit products.
_Avoid_: Portal login, product registration

**Registration**:
The atomic establishment of a Platform User, verified Email Identity, Display Name, HENU Kit Password Credential, and Core Session. If any part fails, no registration is established and the Email Verification Code remains unconsumed.
_Avoid_: Passwordless signup, partial registration

**Email Identity**:
The normalized HENU mailbox identity that uniquely identifies a Platform User and remains the only login identifier.
_Avoid_: Username, account name

**Display Name**:
A non-unique, mutable presentation label that is never authentication or authorization evidence.
_Avoid_: Username, login name

**HENU Kit Password Credential**:
The Platform Core-owned password verifier required to establish Registration and available for later password login. It is distinct from the user's school-mailbox password, which HENU Kit never receives.
_Avoid_: Email password, mailbox password

**Email Verification Code**:
A short-lived proof of control over an Email Identity, used for Registration, code login, account recovery, and recent verification of high-risk credential changes.
_Avoid_: Email password, permanent login code

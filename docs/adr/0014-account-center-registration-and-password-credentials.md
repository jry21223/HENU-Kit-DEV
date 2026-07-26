---
status: accepted
---

# Centralize registration and password authority in Platform Core

HENU Kit will make a password credential mandatory for first registration while retaining email-code login as a permanent alternative. The existing Portal account pages remain the public registration, login, recovery, and credential-management UI. They submit only to Platform Core through the same-origin `/account-auth` route; Platform Core remains the sole credential and Core Session authority. Console and QuizCraft initiate OAuth and do not collect credentials. Production mock password sessions are prohibited.

## Considered options

- Keep separate credential UI inside every product. Rejected because it duplicates a security-critical workflow.
- Replace the already deployed Portal account pages with a second visual implementation in Platform Core. Rejected because it duplicates an existing product surface without changing authentication authority.
- Keep the existing Portal account pages and route their form operations to Platform Core on the same origin. Accepted because the deployed UI remains the source of truth while all credential validation, code consumption, Session issuance, recovery, and password changes stay server-authoritative.

## Consequences

- Portal provides `/account/login`, `/account/recover`, and `/account/security`; `/account/login` supports registration plus password and email-code login. The edge routes `/account-auth/*` to the corresponding Platform Core browser endpoints.
- Registration requires Display Name, normalized HENU Email Identity, Email Verification Code, and HENU Kit Password Credential. Verification-code consumption, user and credential creation, and Core Session issuance commit atomically.
- Successful registration immediately signs the user in and continues a validated OAuth `return_to`; direct registration ends at the Account Center account page.
- Display Names are non-unique, mutable presentation data and never login or authorization evidence. The normalized HENU email remains the sole login identifier.
- One Platform Core-owned Password Policy is shared by registration, login, recovery, and credential changes. It counts Unicode code points, accepts 10–128 code points, permits passphrases and Unicode, and neither trims, truncates, nor normalizes input. A password is rejected when it matches an entry in the versioned weak-password set or, by exact case-sensitive comparison, the local part of the already normalized Email Identity. Platform Core stores only a per-credential salted, versioned Argon2id PHC verifier and upgrades parameters after successful authentication.
- Registration-code requests do not reveal whether an Email Identity exists. After a valid code proves mailbox control, duplicate registration is rejected without changing the existing User, Display Name, or Password Credential.
- Password reset requires a fresh Email Verification Code, revokes all old Core and exchange Sessions, and issues one new Core Session. An authenticated password change requires both the current password and recent email verification, revokes other devices, and retains the current Session.
- Password failures are throttled by Email Identity, IP, and device. Escalation requires email-code login rather than permanently locking the account. Redis or credential-verification dependencies fail closed.
- Authentication-cookie transport and trusted-proxy handling follow ADR-0015.
- No passwordless-account compatibility path is required before the first public launch. Portal must never mint a local success session from form state: only a Platform Core Core Session followed by the product OAuth exchange establishes authentication. Production mock authentication is removed before acceptance.
- MFA, username login, phone login, third-party identity providers, and native-client JSON registration are outside the first release.

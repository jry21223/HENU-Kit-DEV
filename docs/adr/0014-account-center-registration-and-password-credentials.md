---
status: accepted
---

# Centralize registration and password authentication in Account Center

HENU Kit will make a password credential mandatory for first registration while retaining email-code login as a permanent alternative. Platform Core's Account Center is the sole registration, login, recovery, and credential-management UI; Portal, Console, and QuizCraft initiate OAuth and do not collect credentials. This replaces the interim Portal-owned hybrid flow because product-local forms, HTML response parsing, and mock password sessions create ambiguous authentication state and duplicate a security-critical workflow.

## Considered options

- Keep registration and password UI inside each product. Rejected because it duplicates credential handling and lets product state disagree with Platform Core.
- Expose a cross-origin JSON registration API and build an Account Center SPA. Rejected for the first release because it expands the public interface and client-side state machine without a native-client requirement.
- Use server-rendered Account Center forms with minimal same-origin JavaScript and a pure-HTML fallback. Accepted because the server remains authoritative while users can request a code without losing other form input.

## Consequences

- Account Center provides separate `/register`, `/login`, and `/recover` pages; `/login` supports both password and email-code login. `/account/security` owns password changes and Session management.
- Registration requires Display Name, normalized HENU Email Identity, Email Verification Code, and HENU Kit Password Credential. Verification-code consumption, user and credential creation, and Core Session issuance commit atomically.
- Successful registration immediately signs the user in and continues a validated OAuth `return_to`; direct registration ends at the Account Center account page.
- Display Names are non-unique, mutable presentation data and never login or authorization evidence. The normalized HENU email remains the sole login identifier.
- Passwords are 10–128 characters, permit passphrases and Unicode, are neither trimmed nor truncated, and are rejected when commonly weak or identical to the email local part. Platform Core stores only a per-credential salted, versioned Argon2id PHC verifier and upgrades parameters after successful authentication.
- Registration-code requests do not reveal whether an Email Identity exists. After a valid code proves mailbox control, duplicate registration is rejected without changing the existing User, Display Name, or Password Credential.
- Password reset requires a fresh Email Verification Code, revokes all old Core and exchange Sessions, and issues one new Core Session. An authenticated password change requires both the current password and recent email verification, revokes other devices, and retains the current Session.
- Password failures are throttled by Email Identity, IP, and device. Escalation requires email-code login rather than permanently locking the account. Redis or credential-verification dependencies fail closed.
- Production authentication cookies use the `__Host-` prefix with `HttpOnly`, `Secure`, and `SameSite=Lax`; forwarded HTTPS is trusted only from configured proxies. Local HTTP uses distinct non-`__Host-` cookie names.
- No passwordless-account compatibility path is required before the first public launch. Product-local mock authentication and Portal-side parsing of Account Center HTML are removed before production acceptance.
- MFA, username login, phone login, third-party identity providers, and native-client JSON registration are outside the first release.

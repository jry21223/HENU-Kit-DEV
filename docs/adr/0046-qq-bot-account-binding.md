---
status: accepted
amends: 0013, 0014
---

# Platform Core owns QQ Bot account bindings

The user approved #522 and #523 on 2026-09-25. A QQ identity is the pair of
official Bot application and sender identifier; it is never a user-entered QQ
number or an LLM parameter. A Platform User has at most one active QQ binding
across applications. Changing either account requires unlinking first.

Platform Core owns durable bindings and five-minute challenges in PostgreSQL.
The Bot must be provisioned as an exact service client mapped to an application;
ordinary registered OAuth clients have no Bot authority. Portal Gateway adds
only authorize/status/unlink façades, from its own verified Session. Core checks
the exchange Session, parent Core Session and active user at consent and final
confirmation. Signed requests use the existing nonce replay guard. Binding does
not introduce QQ login, transfer school credentials, or grant product roles.

The Bot initiates a link, the authenticated website explicitly consents, and a
later message from the original QQ completes binding after showing the target
account. Links use fragments, not query strings. They may survive the same tab's
login redirect in expiring sessionStorage; they cannot by themselves finish a
binding. The Bot keeps the challenge encrypted and handles it before the model.
Unlink cancels outstanding challenges and removes the binding, including when
the bound account has been suspended. Product requests must resolve the current
binding and account state on every call rather than cache a durable authorization.

This decision does not change website Food Post publication. #523 separately
introduces a Bot-only review queue and will amend Food's direct-publication
contract for that channel only; the existing Food MCP must not be used as a
shortcut around it.

# HENUKit Console 与 QuizCraft 重构执行规格

## Problem Statement

当前仓库的管理后台源自期末复习平台，其路由、权限、组件和业务模型仍混合着大量旧代码，但现在真正需要服务的是整个 HENU Kit 产品族。继续修补旧 Admin 会把 Study Legacy Admin 的产品边界、Element Plus、单一管理员判断和混合 API 带入新系统，使 Portal、Platform Operations、Notice、Library、QuizCraft 与 Food 无法按各自的数据所有权、安全边界和发布节奏演进。

QuizCraft 同时需要保留刷题的核心体验和公开排行的娱乐性，并新增用户收藏能力，但旧技术栈、临时身份、运行时 JSON 兜底和混入其中的美食转盘不能直接成为新平台的一部分。迁移必须允许旧系统继续运行、逐段验证新实现并在生产切流失败时回滚。

## Solution

建立与 Study Legacy Admin 物理隔离的 HENUKit Console，作为 HENU Kit 产品族统一的管理入口。Console 只通过独立的 Console Gateway 使用 Platform Core 和各 Active Product Module 的版本化 API，不连接子产品数据库，也不继承旧 Admin 的路由、样式或授权模型。

Console Overview 固定呈现 Portal、Platform Operations、Notice、Library、QuizCraft 和 Food 六个平级模块。Platform Core 提供统一账户、权限码与 Scope、Session、Operations Inbox、邮件和审计；各产品继续拥有自己的业务数据。Portal 在 V1 只读；QuizCraft 的题库工坊保留在自身后台；积分和会员保持隐藏候选能力。

QuizCraft 保留 React 用户端并用 Go 重构后端，以稳定题库和题目标识承载 Practice Core、按题库收藏夹、公开排行、反馈和 Question Bank Workshop。迁移采用 OpenAPI-first、真实 PostgreSQL/Redis 验证、影子读、对账、灰度切流和可回滚快照。

## Delivery Status

| 状态 | 能力 |
|---|---|
| Current | 当前仓库仍运行原 Web、Admin、Study Legacy API 与 Worker；Web、Admin 与现有 QuizCraft 前端现已统一到 pnpm Workspace，这些现状仍是迁移输入，不是目标 Console 已经存在的证据。 |
| Current after this docs-only change | Context、ADR、替代计划、执行规格和验收 Gate 成为后续工作的决策基线；本变更不创建运行时、不迁移数据库，也不切生产流量。 |
| Planned | pnpm Workspace、Study Legacy Admin 物理拆分、HENUKit Console、Console Gateway、Platform Core、六个 Active Product Module 接入，以及 QuizCraft React + Go 重构与可回滚迁移。 |
| Out of scope for V1 | Portal CMS/部署控制、积分、会员、排行奖励、QuizCraft Electron/Android、AI 生成解析、旧身份自动绑定、跨题库自定义收藏夹和 QuizCraft 美食转盘。 |

下文 Implementation Decisions 均描述 Planned 目标态，除非某条明确标记为 Current 或 Out of scope。实际交付状态只能由已验证代码、契约、Migration 和运行证据更新，不能由现在时措辞推断。

## User Stories

1. As an HENU Kit operator, I want one Console Overview with six clearly named modules, so that I can understand the active product family without seeing retired Study features.
2. As an HENU Kit operator, I want Portal and Platform Operations to be separate modules, so that public-site health is not confused with account and infrastructure administration.
3. As an HENU Kit operator, I want module visibility to reflect my permission codes and Scope, so that I only see products and actions I am responsible for.
4. As a scoped operator, I want out-of-scope reads and writes to be rejected by the server, so that a hidden button is never the authorization boundary.
5. As a frozen or revoked operator, I want access to stop within the defined revocation window, so that old Console Sessions cannot retain authority.
6. As a Console user, I want to sign in through the Platform Core account center and return to the Console, so that I use one identity without sharing a cross-domain cookie.
7. As a Console user, I want my management session stored in a secure HttpOnly cookie, so that long-lived tokens are not exposed to browser code.
8. As a Console user, I want a clear signed-out or expired-session state, so that I can reauthenticate without losing the destination I intended to visit.
9. As an operator, I want the Overview to return available modules even when one product is down, so that one failure does not block all operations.
10. As an operator, I want unavailable module cards identified explicitly, so that missing data is not mistaken for zero activity.
11. As an operator, I want stale summaries labeled with their observation time and last successful refresh, so that cached data is never presented as current.
12. As an operator, I want empty data, partial failure and request identifiers represented consistently, so that I can distinguish normal absence from incidents.
13. As an operator using a phone, I want critical Console flows to work at 390px width, so that urgent operations do not require a desktop.
14. As an operator using assistive technology, I want charts accompanied by text summaries or data tables, so that metrics remain understandable without visual chart interpretation.
15. As a Portal operator, I want to see the deployed version, commit, deployment time and readiness, so that I can verify which public site release is serving users.
16. As a Portal operator, I want to see key-page probes, recent incidents, feedback summaries and product-entry health, so that I can identify public navigation problems.
17. As a Portal maintainer, I want content and navigation changes to remain code-reviewed Portal Configuration, so that the Console does not become an ungoverned CMS.
18. As a Portal maintainer, I want deployment, rollback and version switching absent from Console V1, so that protected CI/CD remains the only production release authority.
19. As a Platform Operations administrator, I want to manage accounts, permission codes and Scope, so that access can be granted precisely across products.
20. As a Platform Operations administrator, I want to inspect and revoke active sessions, so that compromised or obsolete access can be terminated.
21. As a Platform Operations operator, I want mail delivery infrastructure and state visible without exposing secrets, so that I can diagnose delivery work safely.
22. As an auditor, I want management actions correlated by actor, request and target resource, so that changes can be reconstructed across services.
23. As an Operations Inbox operator, I want cross-product work items with owner, priority, SLA, status and resource reference, so that I can coordinate work without copying product-owned content.
24. As a product operator, I want full feedback text to remain in the source product, so that Operations Inbox does not become a second business database.
25. As a Notice operator, I want to manage notice sources, immutable content versions, review, audience and distribution, so that campus notices have an auditable lifecycle.
26. As a Notice reviewer, I want approval and distribution to respect optimistic versions and idempotency, so that retries or concurrent edits cannot silently duplicate publication.
27. As a Library operator, I want to manage courses, materials, downloads, submissions, reviews and corrections, so that the current material service remains operable during extraction.
28. As a Library operator, I want Study Legacy behavior reached only through a controlled Compatibility Adapter during migration, so that old data access does not leak into the Console model.
29. As an HENU Kit user, I want Library to exclude quiz, community, moments, payment, points and membership features, so that its purpose remains clear.
30. As a QuizCraft operator, I want Console to show QuizCraft health, bank and feedback summaries with a deep link, so that I can orient quickly without duplicating the workshop.
31. As a QuizCraft editor, I want Question Bank Workshop in QuizCraft's own administration surface, so that bank creation and review stay with the data owner.
32. As a QuizCraft editor, I want to create, edit, version, import, validate, publish, unpublish and roll back banks, so that question content has a controlled lifecycle.
33. As a QuizCraft editor, I want workshop access governed by shared permission codes and Scope, so that an independent admin token is unnecessary.
34. As a learner, I want to choose a question bank and practice random, difficult or chapter questions, so that the core study modes remain available.
35. As a learner, I want single-choice, multiple-choice, true-or-false and fill-in questions, so that migrated banks retain their supported question types.
36. As a learner, I want server-confirmed results and explanations, so that scoring does not depend on a client assertion.
37. As a signed-in learner, I want wrong answers and learning progress synchronized to my account, so that I can continue across devices.
38. As a guest, I want to practice without creating an account, so that trying QuizCraft remains frictionless.
39. As a guest attempting to save learning state, I want to sign in and return to the same question, so that authentication does not discard my intent.
40. As a signed-in learner, I want to favorite or unfavorite a question, so that I can deliberately build a review set.
41. As a signed-in learner, I want favorites automatically grouped by question bank, so that each bank retains its content boundary without manual folder setup.
42. As a signed-in learner, I want a Favorites Overview showing each bank's count and entry, so that I can choose what to review.
43. As a signed-in learner, I want to practice from one Per-Bank Favorites Folder, so that favorite practice reuses the normal Practice Core without mixing banks.
44. As a learner, I want unavailable favorites retained but excluded from practice and stripped of question text, so that my history is honest without exposing withdrawn content.
45. As a learner, I want the excluded favorite count shown before practice, so that I understand why the session contains fewer questions.
46. As a learner, I want a stable favorite to follow the latest valid question version, so that normal content corrections do not break my folder.
47. As a learner, I want to submit a correction from the question, so that content issues reach QuizCraft with a stable bank and question reference.
48. As a competitive learner, I want Overall Weekly Ranking to be the default leaderboard, so that current cross-bank activity is immediately visible.
49. As a competitive learner, I want Overall Lifetime, Bank Weekly and Bank Lifetime rankings, so that I can compare both current and long-term performance.
50. As a competitive learner, I want each correct answer in a new valid Practice Session to increase my Correct Answer Count, so that repeated practice remains rewarding.
51. As a learner, I want request retries and concurrent duplicate submissions to count only once per session question, so that network behavior cannot inflate rankings.
52. As a learner, I want the public ranking to show only my chosen nickname and system avatar, so that account identifiers remain private.
53. As a learner, I want to opt out of Public Ranking, so that practicing does not require public participation.
54. As a platform operator, I want ranking nicknames constrained against abuse and official impersonation, so that public competition remains safe.
55. As an auditor, I want rankings derived from immutable Scored Attempt facts, so that every total can be explained by bank-level events.
56. As a future product designer, I want a Ranking Settlement Event without rewards, so that a later reward decision has an auditable fact but no hidden points exist today.
57. As a returning learner, I want pre-migration rankings preserved as a read-only Legacy Ranking Snapshot, so that history is not falsely attached to a new identity.
58. As an account owner, I want legacy identity claims to require a separate verified process, so that generated old IDs are never guessed into my account.
59. As a Food operator, I want to review submissions, handle anomaly tickets and confirm tier adjustments in Console, so that Food operations have a focused surface.
60. As a Food data owner, I want every operation to use the Food API, so that Console cannot bypass Food invariants through database access.
61. As a QuizCraft learner, I want the food wheel removed from QuizCraft, so that food entertainment belongs to the Food product instead of the quiz domain.
62. As a product owner, I want points and membership hidden from navigation, dashboards, APIs and rewards, so that undecided capabilities are not accidentally launched.
63. As a Study Legacy operator, I want the existing admin behavior available as an independent build during migration, so that current operations continue while the new Console is built.
64. As a Console developer, I want no Study Legacy route, Element Plus dependency or legacy API type in the Console bundle, so that physical separation is enforceable.
65. As a service owner, I want my product to keep its database credentials and migrations, so that Console Gateway cannot become a shared-data monolith.
66. As a service owner, I want API contracts frozen before implementation, so that code follows accepted product boundaries instead of redefining them.
67. As an API consumer, I want generated TypeScript clients and necessary Go types checked for drift, so that contract changes fail before deployment.
68. As an operator performing a write, I want an idempotency key and operation-status lookup, so that an unknown timeout is resolved without blind resubmission.
69. As a product service owner, I want final idempotency, optimistic concurrency, transactions and audits enforced by the data owner, so that Gateway forwarding cannot weaken invariants.
70. As an operator, I want read aggregation bounded by per-module and overall timeouts, so that a slow product cannot indefinitely delay Overview.
71. As a maintainer, I want only safe idempotent reads retried once with jitter, so that retries do not duplicate mutations or amplify incidents.
72. As a security engineer, I want service-to-service requests authenticated with short-lived controlled credentials, so that internal APIs are not trusted solely by network location.
73. As a database engineer, I want explicit PostgreSQL SQL and versioned migrations, so that constraints, locks, transactions and rollbacks are reviewable.
74. As a reliability engineer, I want Redis limited to replaceable coordination and caches, so that loss of Redis cannot erase the only business fact.
75. As a mail operator, I want verification-code hashes, expiration, consumption and mail outbox persisted reliably, so that Redis is not mistaken for the mail sender or source of truth.
76. As a migration operator, I want question banks, questions, types, answers, chapters and hashes reconciled before cutover, so that content loss or mutation is detected.
77. As a migration operator, I want unresolvable feedback references recorded as migration exceptions, so that records are never silently discarded.
78. As a release operator, I want the new QuizCraft API shadow-read and compared before receiving production writes, so that behavioral differences are measurable.
79. As a release operator, I want incremental events caught up before traffic moves, so that the new database is not stale at cutover.
80. As a release operator, I want gradual read and controlled write cutover with a verified rollback snapshot, so that migration failure remains recoverable.
81. As a release operator, I want the old QuizCraft service held read-only for at least one release cycle, so that rollback evidence remains available.
82. As a maintainer, I want the frontend package-manager migration verified independently, so that lockfile replacement does not hide application regressions.
83. As a maintainer, I want directory movement separated from behavior changes, so that physical split failures are easy to diagnose and revert.
84. As a reviewer, I want each pull request to carry one primary migration risk, so that architecture, behavior, data and production cutover are not approved as one opaque change.
85. As a project maintainer, I want the superseded PR closed only after replacement architecture work is linked and foundational gates pass, so that its reusable history remains discoverable without implying it should merge.

## Implementation Decisions

- The target remains a development Monorepo with independently deployable products and services; repository consolidation does not imply a shared runtime or database.
- HENUKit Console and Study Legacy Admin are separate applications, bundles, route trees, navigation models, builds and deployment units.
- HENUKit Console uses Vue 3, TypeScript, Vite, shadcn-vue, Reka UI, Tailwind CSS v4 and HENU Kit Design Tokens. It does not install Element Plus.
- TanStack Vue Query owns remote data state. Pinia is limited to Console Session, Console Access Context and small cross-page UI state. ECharts is loaded on demand and charts require a non-visual equivalent.
- Frontend workspaces use pnpm. Migration creates and verifies the workspace and lockfile before retiring npm lockfiles.
- Console Overview contains exactly Portal, Platform Operations, Notice, Library, QuizCraft and Food. Points and membership remain Candidate Capabilities and are not exposed.
- Console Gateway is an independently deployed Go service using `net/http` and `chi`. It owns no business data and has no product-database credentials or business migrations.
- Platform Core is an independently deployed Go service using `net/http`, `chi`, PostgreSQL, `pgx`, `sqlc` and versioned SQL migrations.
- PostgreSQL is the durable source of truth. Redis is limited to rate limiting, nonce and replay defense, temporary locks, short-lived coordination and rebuildable caches.
- Each data owner maintains an OpenAPI 3.1 contract. Generated clients and types are checked into or deterministically generated by CI, with lint, example, implementation and breaking-change checks.
- Console authentication uses a one-time authorization code exchanged server-side by Console Gateway. The resulting Console Session uses HttpOnly, Secure and SameSite cookies and does not expose a long-lived browser token.
- Authorization uses permission codes plus Scope and defaults to deny. Client-supplied roles, legacy `isAdmin` and a single role string are not authorization evidence.
- Console Gateway concurrently aggregates six module summaries. The default module timeout is two seconds and the Overview deadline is three seconds.
- Only idempotent reads receive at most one jittered retry. A failed module does not fail the Overview; cached summaries are limited to five minutes and explicitly marked stale.
- Persisting writes carry an Idempotency-Key. Gateway never automatically retries writes and preserves the request identifier, idempotency key and actor context. The data owner enforces final idempotency, optimistic versions, transactions and audit.
- An unknown write timeout produces a queryable operation state; the client does not blindly resubmit.
- Platform Core owns accounts, permission codes, Scope, sessions, Operations Inbox, mail infrastructure and platform audit. Product content remains with its owner.
- Portal Module V1 is read-only. Portal Configuration changes through Git, review and CI/CD; Console has no content editor, deploy, rollback or version switch.
- Notice owns sources, immutable versions, review, audiences and distribution.
- Library initially reaches existing Study behavior through a bounded Compatibility Adapter, then extracts courses, materials, downloads, submissions, review and correction to its own service.
- Food owns submissions, entries, calibration, anomaly tickets and tier adjustment. Console operations call its API and never its database.
- QuizCraft Console integration is summary plus deep link. Question Bank Workshop remains a QuizCraft-owned administration surface.
- QuizCraft retains React, TypeScript and Vite while the FastAPI backend is replaced incrementally by a modular Go service.
- QuizCraft production data is modeled around stable bank IDs, stable question IDs and explicit question versions. Runtime JSON fallback becomes an explicit import path rather than a production data source.
- Practice supports random, difficult and chapter selection and all four accepted question types. Guests can practice; favorites, wrong answers and progress require authentication.
- A Favorite Question is a stable reference owned by a user. Per-Bank Favorites Folders are automatic and cannot be renamed, shared or mixed across banks in V1.
- Unavailable Favorites retain the relationship but expose no question body and are excluded from Favorites Practice. Valid stable references follow the latest available question version.
- Each Practice Session, user and question produces at most one Scored Attempt. New valid sessions may score the same question again.
- Correct Answer Count is the ranking metric. Rankings provide Overall and Bank views over Weekly and Lifetime periods, defaulting to Overall Weekly.
- Public Ranking exposes only a controlled nickname and system avatar through Ranking Profile and supports opt-out. It does not expose account identifiers.
- Ranking Settlement Events record final standings but do not grant points, membership or rewards.
- Old generated QuizCraft identities are not automatically mapped. Previous standings are retained as a read-only Legacy Ranking Snapshot.
- QuizCraft content migration proceeds through schema and contract freeze, full reconciliation, shadow reads, incremental catch-up, limited read traffic, controlled write cutover and a read-only rollback observation period.
- The QuizCraft food wheel and food data are removed without a compatibility entry; Food owns that experience.
- Electron and Android wrappers, AI-generated explanations, visible or hidden ranking rewards and legacy identity claiming are not first-wave migration requirements.
- Delivery is dependency-ordered: documentation, pnpm migration, Admin physical split, Console foundation, Platform Core, Console Gateway, Portal and Platform Operations, Notice, Library, Food, QuizCraft shadow implementation, QuizCraft cutover, then superseded PR closure.
- Each delivery unit must isolate one primary risk and keep production cutover separate from structural and data migration work.

## Testing Decisions

- Tests assert externally observable contracts and user behavior, not internal helper structure. The preferred seams are, in order, OpenAPI contract compatibility, service HTTP boundaries, browser-visible Console and QuizCraft flows, and a small number of cross-service end-to-end paths.
- OpenAPI tests cover lint, valid and invalid examples, generated-client drift, implementation conformance and breaking changes.
- Go HTTP tests cover authentication exchange, secure session behavior, default-deny authorization, Scope escape, frozen accounts, revocation, forged client roles, idempotency, optimistic conflicts, operation-status lookup, audit correlation, timeout and partial-failure envelopes.
- Database integration tests use temporary real PostgreSQL and Redis instances. SQLite is not an acceptable substitute.
- Migration tests cover an empty database, an existing supported database, repeated Up, Down and Up again, constraints under concurrency, and recoverability from a verified snapshot.
- Cross-owner boundary tests prove that each service uses separate database credentials and that direct queries into another owner's data fail.
- Console component and page tests use Vitest and Vue Test Utils for loading, empty, partial, stale, unavailable, permission and error states.
- Console browser tests use Playwright at desktop and 390px widths for sign-in return, session expiry, module visibility, Overview degradation, accessible chart alternatives and representative safe writes.
- Bundle boundary tests fail if Console includes Element Plus, Study Legacy routes or legacy business API types.
- Gateway tests prove the two-second module timeout, three-second overall deadline, maximum one read retry, no write retry, stale metadata and request tracing.
- Platform Core tests cover authorization-code single use, session revocation, permission propagation, verification-code lifecycle, durable mail outbox and Redis-loss recovery.
- QuizCraft HTTP and browser tests cover guest practice, authenticated learning state, all question types, duplicate submission races, repeated scoring in a new session, favorites lifecycle, unavailable favorites, per-bank practice, ranking periods, ranking opt-out and public-field privacy.
- QuizCraft migration tests compare bank count, question count, question types, answers, chapters and content hashes; unresolved references must appear in an exception report.
- Shadow-read tests compare old and new QuizCraft responses before cutover. Cutover acceptance requires observed production health and rollback evidence, not merely a started workflow.
- Existing legacy build and smoke tests remain regression evidence during the physical split. Directory movement must not alter Study Legacy behavior.
- Every implementation ticket defines its own focused acceptance commands and receives a separate standards-and-spec code review before dependents begin.

## Out of Scope

- A runtime CMS or visual page builder for Portal Configuration.
- Console controls for Portal deployment, rollback or version switching.
- Points, membership, paid entitlements or ranking rewards, whether visible or hidden.
- A Console-hosted copy of Question Bank Workshop.
- QuizCraft Electron and Android wrapper migration in the first wave.
- AI-generated QuizCraft explanations and their moderation model.
- Automatic mapping of legacy QuizCraft generated IDs to HENU Kit accounts.
- Cross-bank custom favorite folders, folder sharing, renaming or mixed Favorites Practice.
- Restoring the QuizCraft food wheel or keeping a compatibility route for it.
- Combining product databases, sharing product credentials with Gateway, or moving product content into Operations Inbox.
- A big-bang rewrite or a single production cutover containing frontend, backend, schema and traffic changes.
- Final production domains, infrastructure orchestration and secret values; these require environment-specific delivery decisions.

## Further Notes

- This specification executes the accepted replacement plan and ADRs; it does not describe the new architecture as already deployed.
- `CONTEXT.md` 是领域词汇及语义的规范来源；本规格和替代计划为了验收而复述行为，修改概念时必须先更新所属 Context，再消除下游差异。
- The current Admin implementation and its associated old unified-dashboard issues are superseded inputs, not the implementation base for HENUKit Console.
- Reusable work from the superseded PR is limited to boundary-reviewed UI primitives, contract-test patterns and low-level test scenarios. Legacy navigation, route ownership, authorization and migrations are not reusable.
- The superseded PR remains Draft until a replacement architecture pull request links back to it and foundational checks pass; it is then closed without deleting its branch or history.
- Unresolved future decisions are limited to AI explanations, points and membership, Portal's eventual implementation detail, and environment-specific production infrastructure. None blocks the ordered V1 work defined here.

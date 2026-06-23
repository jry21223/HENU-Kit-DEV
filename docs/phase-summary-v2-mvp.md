# V2 MVP Phase Summary

Date: 2026-06-23

This document summarizes the current V2 MVP implementation state against the original greenfield V2 plan. It is intentionally conservative: an item is marked complete only when the current repository contains working code and the change has been checked by build, tests, or direct file inspection.

## Current Phase

V2 MVP implementation and hardening.

The project has moved beyond the initial skeleton. The current work is filling verifiable product loops on top of the Go API, Next.js Web app, Vue Admin app, PostgreSQL schema, Redis worker foundation, and Docker Compose development environment.

## What Was Done In The Latest Work

Recent pushed commits:

- `6042bcf Add wiki report action`
  - Added a report action to the public Wiki detail page.
  - Uses the existing server-side `wiki_entry` report target.
  - Does not expose draft, pending, rejected, or private review metadata.

- `33588f9 Add public package listing page`
  - Added Web `/packages`.
  - Lists only published course packages from the Go API.
  - Links to existing package detail pages.
  - Does not mark orders paid, fake ownership, or grant entitlement from the frontend.

- `0ef3f2e Add web wrong question book`
  - Added Web `/me/wrong-questions`.
  - Shows only the current user's wrong questions and per-course weakness totals.
  - Fetches question detail through public question DTOs that do not include answers.
  - Supports deleting the current user's own wrong-question rows.
  - Added `/me` shortcuts for wrong questions, downloads, notifications, and forum submissions.

- `2147af9 Guard stale wiki proposal approvals`
  - Shows stale Wiki edit proposals in Vue Admin.
  - Blocks stale proposal approval in the UI.
  - Keeps the Go API as the final enforcement layer for stale-version rejection.

Additional implementation after the listed commits:

- Web `/wiki/[id]` now includes a creator/admin-only edit proposal form.
- The form submits to `POST /api/v1/wiki/entries/:id/proposals`.
- Normal users and guests see the review boundary instead of the editable form.
- Submitted proposals remain pending until reviewer/admin approval; public Wiki content is not changed by the frontend.
- Go API now includes `POST /api/v1/payments/wechat/native` for development/test mock Native code URL creation.
- Mock Native creation moves eligible owned orders from `pending` to `paying`, but does not mark orders paid or grant entitlement.
- Production rejects mock WeChat Pay configuration, and live WeChat API calls remain unimplemented.

## Verification Run For Latest Work

Commands run successfully during the latest implementation sequence:

```bash
cd apps/web
npm run lint
npm run build

cd apps/admin
npm run lint
npm run build

cd services/api
go test ./...

docker compose -f docker-compose.dev.yml config --quiet
git diff --check
```

Notes:

- Vite reports a large chunk warning for Admin build. It is a bundle-size warning, not a build failure.
- PowerShell may display Chinese text as mojibake depending on terminal encoding. The checked files are committed as repository text, and TypeScript/Vite/Next builds pass.

## Original Plan Comparison

Legend:

- Complete: implemented and locally verified.
- Partial: usable foundation exists, but the original plan asked for more.
- Not started: no meaningful implementation yet.
- Deferred / changed: intentionally postponed or superseded by later product direction.

| Original Stage | Original Target | Current Status | Evidence / Notes |
| --- | --- | --- | --- |
| Stage 0: cleanup and engineering skeleton | Monorepo, old code archived, env examples, Docker Compose | Complete | `apps/web`, `apps/admin`, `services/api`, `services/worker`, `legacy/v1-next-prisma`, Docker Compose files, `.env.example`, docs exist. |
| Stage 1: Go API foundation | Gin, config, PostgreSQL, Redis, GORM, middleware, health/version | Complete | `services/api`, `GET /healthz`, `GET /api/v1/healthz`, `GET /api/v1/version`, `go test ./...` passes. |
| Stage 2: database schema | Full V2 schema, migration, seed | Partial | `services/api/migrations/0001_v2_schema.sql`, GORM models, and seed command exist. Not every planned domain has full production behavior. |
| Stage 3: auth and permissions | Email code login, JWT RS256, roles, frozen user checks | Partial / mostly complete | Email-code login, JWT cookies/tokens, roles, admin/reviewer/creator guards, and frozen-user write/download guards exist. Full production email provider configuration remains deployment work. |
| Stage 4: org and course materials | School/college/major/course APIs, materials, upload, download, watermark | Partial / strong MVP | Public org/course/material APIs, admin CRUD, upload guardrails, download permissions, audit logs, and PDF watermarking exist. OSS/S3 is still only a future boundary. |
| Stage 5: quiz system | Question types, submission, attempts, wrong questions, weakness report | Partial | Single/multiple/true-false/fill/short-answer structures and judging foundation exist; Web course quiz and Web wrong-question book exist. Advanced scoring and richer practice sessions remain incomplete. |
| Stage 6: AI infrastructure and worker | Redis Streams worker, mock/real LLM, AI tasks, draft review | Partial | Mock AI task flow, worker completion, usage logs, draft creation, and Admin draft review exist. Real LLM, RAG, and publish-to-resource flows remain incomplete. |
| Stage 7: points and membership | Points logs/rules, memberships, redemption, member benefits | Partial | Model and some points ledger behavior exist, especially forum reward escrow/settlement. Full member purchase/redeem UX and benefits enforcement are not complete. |
| Stage 8: payment system | Original V2 text said Yipay; later direction changed to WeChat Native | Partial / direction changed | Pending course-package orders exist with server-side pricing. Development/test mock Native code URL creation exists and moves orders to `paying`. Real WeChat Native API calls, notify verification, paid status update, and automatic entitlement issuance are not complete. Yipay is not the target path after product direction changed. |
| Stage 9: Wiki co-creation | Creator application, entries, proposals, history, review | Partial / strong MVP | Public Wiki read pages, creator/admin submission, Web edit proposal form, review queues, history writes, stale-version protection, and stale UI guard exist. Creator application flow remains incomplete. |
| Stage 10: blog, moments, forum, relations | Blog, dynamic moments, forum, follow/block | Partial | Blog public pages and review flow exist. Forum list/detail/create/reply/resubmission/reward best-answer basics exist. Moments and user relation features are not implemented. |
| Stage 11: notifications, reports, search, leaderboards | Notifications, reports, search, leaderboards | Partial | User notification inbox, review notifications, report API, Web report buttons, Admin report handling, and report analytics exist. Search and leaderboards are not implemented. |
| Stage 12: Next.js main site | Main user-facing pages | Partial | Core pages exist: home, login, courses, materials, quiz, packages, Wiki, Blog, forum, profile, downloads, notifications, wrong questions. AI, papers, memberships, points, moments, leaderboards, and user profile pages remain incomplete. |
| Stage 13: Vue Admin | Admin pages for users, content review, AI, points/member, orders, logs | Partial | Admin login, dashboard, users, access grants, courses, materials, packages, orders, downloads, reviews, reports, analytics, operation logs, and AI drafts exist. Points/member/system config pages are not complete. |
| Stage 14: Docker Compose | Local one-command stack | Partial / usable | `docker-compose.dev.yml` config validates. Full end-to-end runtime still depends on local env and seed setup. |
| Stage 15: seed data and demo accounts | Admin/user/creator/reviewer accounts and sample content | Partial | Seed command exists with demo organization/content/accounts. Production data import and real material mounting remain deployment work. |
| Stage 16: tests and quality | Broad backend/frontend/admin checks | Partial | Backend tests cover auth, materials, quiz, AI, wiki, blog, forum, reports, orders, analytics, notifications, and admin flows. Web/Admin typecheck and builds pass. More E2E/browser tests are still missing. |
| Stage 17: documentation | Architecture, API, database, development, deployment, security docs | Partial | Core docs exist in `docs/`. They need ongoing updates as payment, membership, AI, and deployment mature. |

## Currently Verifiable Product Areas

- Greenfield monorepo layout.
- Go API health, version, auth, roles, and core middleware.
- PostgreSQL/GORM model foundation and SQL migration.
- School, college, major, course, material, and package APIs.
- Material upload guardrails, download permissions, download audit logs, and PDF watermarking.
- Course package catalog, package grants, public package list/detail pages, and pending-order creation.
- Development/test WeChat Native mock code URL creation with production mock guard and no entitlement side effects.
- Question listing/detail without answer leakage.
- Quiz submission, wrong-question recording, current-user wrong-question page, and basic weakness totals.
- Wiki public list/detail, review-first creation, Web edit proposal form, review queue, history writes, stale protection.
- Blog public list/detail and review-first backend flow.
- Forum public list/detail, post/reply submission, current-user resubmission, reward escrow/refund/settlement, best-answer action.
- User notifications for review/report outcomes.
- Report API, Web report buttons for material/wiki/blog/forum, and Admin report handling.
- Vue Admin user management, manual grants, course/material/package management, orders, downloads, analytics, operation logs, and review pages.
- Mock AI task + worker + draft review flow.
- LangBot sales-agent prototype exists, but it is not a production payment or delivery authority.

## Not Done Yet

High-priority gaps:

- Real WeChat Native payment integration:
  - no live WeChat API code URL creation,
  - no signed/decrypted notify callback,
  - no paid status transition from payment provider,
  - no automatic entitlement issuance from successful callback.

- Full membership and points product:
  - points ledger is used in some forum reward flows,
  - membership purchase/redeem/benefit enforcement is not a complete product loop.

- Real AI:
  - current worker is mock-oriented,
  - no production LLM key path has been verified here,
  - no RAG,
  - no AI-generated content auto-publish, by design.

- Search, leaderboards, moments, user relations, public user profile pages.

- Full E2E testing:
  - current evidence is mostly unit/integration/build,
  - browser-level flows and mobile screenshots still need systematic coverage.

- Production deployment hardening:
  - real env/secrets,
  - HTTPS/reverse proxy,
  - object storage,
  - observability,
  - backups,
  - payment merchant configuration.

## Security And Product Boundaries Still Preserved

- Paid material downloads must go through the Go API.
- Public material/package/Wiki/Blog/Forum APIs filter unpublished content.
- Question list/detail endpoints do not return answers.
- AI drafts do not auto-publish into official content.
- Admin UI changes do not replace server-side authorization or review checks.
- Pending orders do not imply paid status and do not grant entitlement.
- Real course PDFs, private keys, payment secrets, and LLM keys should not be committed.

## Recommended Next Steps

1. Continue payment hardening around WeChat Native.
   - Implement config validation first.
   - Then Native order creation.
   - Then notify verification/decryption/idempotency.
   - Only after that grant entitlement.

2. Add E2E smoke tests for:
   - login,
   - course package browsing,
   - quiz submission and wrong-question creation,
   - report submission,
   - admin review.

3. Add creator application flow for Wiki contributors.

4. Build the missing membership/points UI only after payment direction is stable.

5. Keep README and docs as real-state documents, not marketing claims.

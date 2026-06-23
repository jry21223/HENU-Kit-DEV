# Roadmap

## First Deliverable

- V2 monorepo skeleton
- Go API health/version endpoints
- Go API email-code auth and role middleware
- GORM model coverage for the V2 table set
- Material download permission checks
- Material download audit logs
- Dynamic PDF watermarking during download without mutating source files
- User and admin download-history APIs
- Quiz submission and wrong-question persistence
- Basic weak-point counts
- Admin organization/course/material CRUD
- Local material upload guardrails
- Web course list/detail, material detail, and quiz practice pages
- Web forum list/detail pages with post creation, reply submission, and best-answer action entry
- Web personal download-history page
- Web personal forum submission tracking page for current user's posts/replies and review status
- Web personal forum edit/resubmit flow for draft/pending/needs_changes/rejected posts and replies
- Web personal notification inbox with user-scoped read/read-all state
- Web profile entitlement summary for active material/package grants
- Web course package cards with price and included material links
- Web email-code login form
- Web profile page with school/major/grade binding
- Vue admin dashboard, login guard, course management, and material upload pages
- Vue admin user management page for filtered user listing, role updates, active/frozen status changes, self-lockout prevention, and super_admin protection
- Vue admin all-status course listing and course edit dialog
- Vue admin material metadata edit dialog with server-side type/access validation
- Vue admin material draft/pending/published/archive status operations
- Vue admin download audit page
- Vue admin AI task visibility and draft approve/reject page with reviewer-role access, review notes, and one-way review state checks
- Vue admin material review queue with reviewer-role access, approve/reject actions, review notes, and one-way pending review checks
- Vue admin wiki review queue with reviewer-role access, approve/reject actions, review notes, public-only published wiki APIs, and initial edit-history capture
- Vue admin wiki edit proposal review queue with reviewer-role access, stale-version protection, live-entry version updates, and edit-history capture
- Vue admin blog review queue with reviewer-role access, approve/reject actions, review notes, and public-only published blog APIs
- Vue admin forum review queue with reviewer-role access, approve/reject actions, review notes, public-only published forum post APIs, and published-board checks
- Vue admin forum reply review queue with reviewer-role access, approve/reject actions, review notes, public-only published reply APIs, and once-only comment-count updates
- Forum reward posts with server-side point escrow, review visibility, rejection refunds, and points ledger rows
- Forum best-answer selection with one-answer-per-post guardrails and escrowed reward settlement to the selected reply author
- Review notifications for forum post/reply, material, wiki entry/proposal, blog post, and AI draft approve/reject outcomes
- Basic report submission, Web material/forum report buttons, Vue admin report handling page, operation logs, and reporter result notifications
- Vue admin read-only analytics page for download trend, course demand, and report distribution
- Vue admin read-only operation-log browser for high-risk admin mutations
- Vue admin operation-log filtering, CSV export, and read-only retention policy panel
- Mock AI task API, worker completion, usage log, and reviewable draft creation
- Course package catalog and package-level material grants
- Server-side operation logs for user management, organization, course, material, upload/status/archive, material review, wiki entry/proposal review, blog review, forum post/reply review, forum best-answer selection, and AI draft review mutations
- Demo seed command
- Go Worker skeleton
- Next.js Web shell
- Vue Admin shell
- Docker Compose local stack

## Next Deliverable

- Richer content review workflows for wiki conflict-resolution UX and additional notification sources for payment and membership
- Admin analytics expansion for page visits, search intent, course request voting, payment, and membership conversion metrics
- Real LLM/RAG integration and AI draft publish-to-resource flows

## Later Deliverables

- AI Worker flows
- Points and membership
- Wiki creator workflow refinements and richer revision diff tooling
- Blog, moments, forum
- Notifications, richer reports, leaderboards
- Production deployment and monitoring

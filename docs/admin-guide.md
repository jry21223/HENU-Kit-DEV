# Admin Guide

The Vue admin console is intentionally narrow during the V2 MVP. It only exposes flows that are backed by server-side role checks in the Go API.

## Current Pages

- `/dashboard`: module status summary.
- `/courses`: organization-backed course creation and archiving.
- `/materials`: local material upload, all-status material listing, and material status operations.
- `/downloads`: successful material download audit logs.
- `/ai/drafts`: AI task visibility and AI draft approve/reject review operations.
- `/analytics`: read-only material download and course demand analytics.

## Material Operations

`/materials` calls `GET /api/v1/admin/materials` and requires an authenticated `admin` or `super_admin` role.

Status flow used by the MVP:

- `draft`: stored but hidden from public pages
- `pending`: submitted for review, still hidden from public pages
- `published`: visible to public material list/detail and eligible for server-side download permission checks
- `archived`: hidden from public pages

Important boundaries:

- Upload and manual create default to `draft` when no status is provided.
- The admin UI can move materials to `pending`, `published`, `draft`, or `archived`.
- Public APIs only expose `published` materials.
- Invalid statuses are rejected by the Go API.
- The UI does not display `storage_key`; downloads still go through `GET /api/v1/materials/:id/download`.

## Download Audit

`/downloads` calls `GET /api/v1/admin/downloads` and requires an authenticated `admin` or `super_admin` role.

The table shows:

- material title and material id
- access level
- user id, or anonymous when the download did not require login
- IP
- User-Agent
- download time

Filters:

- `materialId`
- `userId`

Important boundaries:

- The admin UI only reads audit logs; it does not grant access or mutate download records.
- Failed authorization, unsafe storage keys, and missing files are not recorded as successful downloads.
- `storage_key` is not returned in material JSON and is not displayed in the admin UI.

## AI Draft Review

`/ai/drafts` calls:

- `GET /api/v1/admin/ai/tasks`
- `GET /api/v1/admin/ai/drafts`
- `POST /api/v1/admin/ai/drafts/:id/approve`
- `POST /api/v1/admin/ai/drafts/:id/reject`

The page shows recent AI tasks and reviewable AI drafts created by the worker.

Important boundaries:

- All endpoints require an authenticated `admin` or `super_admin` role.
- Approving or rejecting a draft only changes the draft review status.
- The MVP does not automatically publish AI drafts as materials, questions, wiki entries, or papers.
- The UI displays task input/result and draft content for review, but it does not call any LLM directly.
- Real LLM/RAG, reviewer comments, and publish-to-resource flows remain later work.

## Analytics

`/analytics` calls `GET /api/v1/admin/analytics/overview` and requires an authenticated `admin` or `super_admin` role.

The page shows:

- total users, courses, materials, published materials, course packages, and successful downloads
- 14-day successful material download trend
- top downloaded materials
- access-level download breakdown
- course demand rows sorted by download count and material supply

Important boundaries:

- Analytics are derived from server-side successful download logs and current course/material/package records.
- Denied downloads are not counted as demand because they are not successful delivery events.
- The page is read-only and does not grant access, mutate material status, or expose `storage_key`.
- This MVP analytics view does not yet include page visits, search intent, payment conversion, or course request voting.

## Planned Areas

- Users and roles
- Richer content review
- Points and memberships
- Orders
- Reports
- System config
- Operation logs

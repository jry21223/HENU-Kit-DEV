# Admin Guide

The Vue admin console is intentionally narrow during the V2 MVP. It only exposes flows that are backed by server-side role checks in the Go API.

## Current Pages

- `/dashboard`: module status summary.
- `/courses`: organization-backed course creation and archiving.
- `/materials`: local material upload, material listing, and material archiving.
- `/downloads`: successful material download audit logs.

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

## Planned Areas

- Users and roles
- Content review
- AI tasks and drafts
- Points and memberships
- Orders
- Reports
- System config
- Operation logs

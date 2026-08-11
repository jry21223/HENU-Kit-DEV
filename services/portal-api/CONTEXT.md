# Portal API Context

## Owns

- User-facing read API for all Portal product data (Library, Food, Practice, Campus).
- Mock data serving that exactly matches the Portal frontend interfaces.
- Data filtering (by type, subject, campus, category, search query).

## Does not own

- Authentication or session management (owned by Portal Gateway).
- Admin operations or write APIs (owned by individual product services).
- Product databases (will own when connected to real data sources).
- Portal frontend implementation (owned by apps/portal).
- Library download eligibility, signed grants, Download Start facts, and their
  aggregates (owned by Library under ADR-0027).

## Current boundary

Portal API serves mock data that is a 1:1 translation of the Portal frontend's TypeScript mock interfaces into Go structs. The API contract (`packages/api-contracts/openapi/portal-api.yaml`) is derived entirely from the frontend mock data — the frontend is the contract truth.

All endpoints are public (no auth required). The Portal Gateway proxies product data requests to this service.

The legacy Library material list/detail reads do not authorize a download.
ADR-0027's exact download route is intercepted by Portal Gateway and sent to
Library; Portal API must not translate `storage_key` into a URL, sign OSS
requests, record a download, or provide a fallback grant.

## Key terms

- **Material**: A library resource (note, exam, mock paper, learning path, lab report) with full content pages.
- **Post**: A food review with content blocks, shop location, likes, stars, and campus classification.
- **Question**: A practice question with stem, 4 options, answer, explanation, and difficulty rating.
- **School/Major/Subject hierarchy**: The browsing structure for practice question banks.
- **Item**: A campus marketplace listing (help request or sell offer) with status lifecycle.

## Relationships

- **Portal Gateway → Portal API**: Proxies all product data GET requests. No auth needed.
- **Portal API → (future) product databases**: Will replace mock data with real database queries.

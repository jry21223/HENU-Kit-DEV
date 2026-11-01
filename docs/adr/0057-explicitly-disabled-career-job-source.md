# Keep Career available while its job source is disabled

The operator requested whole-stack automatic releases from successful main
builds and explicitly directed us to ignore getWork because WSL networking is
unavailable. Waiting for that node no longer describes the desired deployment.

## Decision

`CAREER_GETWORK_ENABLED=0` disables job scanning at the Career application
boundary. Validate the flag at startup; reject unknown values. Authenticate
requests normally, then reject new searches with `503 JOB_SOURCE_DISABLED`
before creating tasks or charging rate limits. Existing queued tasks fail with
the same typed reason and emit neither successful empty results nor digests.
Health reports `job_source_enabled: false`. Preserve history, profiles, resume
tools and their existing data and authentication requirements.

The primary prebuilt runtime defaults this flag to `0` and excludes the crawler
and WSL relay. Remove Career's relay dependency so this mode starts without WSL.
The production release gate requires an explicit `0` or `1`; only disabled mode
waives the unused getWork token requirement. Other production credentials,
database, restore, source verification and rollback gates remain mandatory.
Retain the canonical image inventory and its provenance checks byte for byte.
The getWork build smoke uses a GitHub runner and does not contact WSL.

This source mode is independent of adopting the existing mixed production
release as a full-stack baseline. It does not authorize removing scoped
receipts, fabricating a healthy baseline, merging a PR or skipping acceptance.
The canonical public release-check origin and GitHub webhook use `henukit.cn`.

---
status: accepted
amends: 0022, 0024, 0025, 0027, 0028, 0029
---

# Public materials are OSS download-only with online preview disabled

The accepted materials chain previously coupled raw reviewed files to a
LibreOffice/Python Slides conversion path. Production preparation showed that
this derived work consumed substantial host resources before any OSS publish
or activation. The product requirement is narrower: users need the reviewed
original file through the established private-OSS download owner, not an
online Slides viewer.

## Decision

- Preparation mirrors and verifies only reviewed raw assets. It has no Python,
  LibreOffice, converter, or Slides output option. Sealing records
  `slides.status = "disabled"`; it does not describe conversion as deferred.
- Activation creates and verifies a real but empty `slides/` directory and a
  canonical `derived-inventory.json` whose `assets` array is empty. The fixed
  activation wrapper and installer expose no converter path. Any derived
  preview asset, non-empty derived digest, converter argument, or legacy
  `--slides-dir` importer argument fails closed.
- Runtime import writes `materials.slides = NULL` for every material, including
  source files whose catalog type remains `slides`. That type is a source-file
  classification only and grants no browser preview capability.
- Library activation accepts only a sealed `disabled` Slides state and the
  canonical empty derived digest. It still verifies the complete raw OSS
  release, exact Object keys/VersionIds/bytes/SHA-256, and private-object
  boundary before atomically activating the owner catalog.
- Portal presents one file action: its same-origin
  `GET /api/v1/library/materials/{material_id}/download` owner façade. Portal
  Gateway obtains Library's exact-version grant and returns a no-store `303` to
  `henukit.oss-cn-beijing.aliyuncs.com`; page code never receives, constructs,
  stores, or logs the signed URL. No material renders an online reader, Slides
  viewer, “立即阅读”, or “免费试读” action. The legacy `/library/read/{id}` and
  `/library/slides/{id}` routes redirect to the material detail.
- OSS publication and activation retain ADR-0026 through ADR-0029's existing
  signed-manifest, exact-version, role, no-ACL/no-Delete, journal, audit, and
  forward-rollback gates. Disabling preview does not authorize a public
  Bucket, static AccessKey, local-file fallback, proxy download surface, or
  runner enablement.

## Consequences

- Production does not build or convert materials and cannot consume host
  resources for online previews. Only reviewed original bytes are published.
- A source PPT/PPTX remains downloadable from OSS like any other reviewed
  asset, but it has no browser Slides representation.
- A real acceptance test begins at the Portal owner route, observes the bounded
  OSS redirect without exposing its query, downloads the exact object, and
  compares bytes/SHA-256 with sealed evidence. A local `/materials/` success or
  an OSS URL embedded in page data is not acceptance evidence.

---
status: accepted
amends: 0025, 0028, 0029, 0031, 0037
---

# Library uses canonical public-material types and owner-only activation

HENU-Final-Review `main` at
`fcd9e86b60856188b81868e5c96f26a8720b18db` defines eight canonical roles:
seven publishable content roles plus `待复核资料`. PR #20 also establishes the
electronic-textbook authorization evidence used by this boundary.

## Decision

- Library accepts exactly `复习讲义`, `往年真题`, `课件`, `题库练习`, `答案解析`,
  `笔记总结`, `电子版教材`, and `待复核资料`. The four upstream legacy aliases
  normalize to `课件` or `待复核资料`; other substring-derived roles fail closed.
- The seven public API types are `handout`, `exam`, `slides`, `exercise`,
  `answer`, `note`, and `textbook`. `待复核资料` is never activated.
- General-material `reviewStatus` and `licenseStatus` are advisory, matching
  the upstream validator. Safety enforcement remains exact for personal-info,
  review-only uncertainty, the historical teacher exception, and textbooks.
- An `电子版教材` is publishable only with `reviewStatus=verified`,
  `licenseStatus=authorized-redistribution`, and a non-empty `sourceNote`.
  `public-review-only` and `public_review_only` are not redistribution
  authorization for a textbook.
- OSS publication and Library activation validate the exact Object key using
  the OSS UTF-8 1–1023 byte boundary, a bounded non-control VersionId, exact
  SHA-256 metadata, and an upstream-compatible safe download filename.
- The authenticated materials webhook supplies one exact Git SHA. Preparation
  and sealing independently resolve the configured ref to that SHA. The
  retired moving-branch sync scripts always exit without writing.
- Activation switches the complete Library owner catalog only. It no longer
  packages or executes the retired Study importer. A historical
  `database_running` journal remains fenced for manual reconciliation;
  `database_committed` may only finish its already-durable marker transition.
- Historical snapshots and their activation digests remain immutable. The
  schema permits their legacy types, while exact identity matching supports
  replay/rollback of pre-canonical releases; new releases use only seven types.

## Consequences

- Merging a manifest is not enough to upload a pending or unauthorized
  textbook. Failure occurs before an OSS call and preserves the prior active
  Library release.
- A production publication still requires the signed runtime, an authenticated
  exact-SHA webhook delivery, complete OSS receipts, Library activation, and
  observed download/rollback evidence.

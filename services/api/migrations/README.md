# Study Legacy API migrations

`0001_v2_schema.sql` is the historical baseline anchor. Production keeps
`AUTO_MIGRATE=false`; every later schema change uses ordered Up/Down SQL and is
applied by an operator before the corresponding runtime path is enabled.

- `0002_henukit_materials_sync_expand`: additive columns/index plus the
  transaction marker required by the canonical materials sync. See its
  companion Markdown file for prerequisites, lock impact, verification and
  rollback.

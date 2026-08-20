---
status: accepted
amends: 0036
---

# Ranking identity reuses the platform profile

ADR-0036 defined the public ranking privacy contract — entries contain only
`rank / nickname / system_avatar / correct_answer_count`, user UUIDs never
appear, learners without a visible Ranking Profile are absent, and empty
nicknames render the neutral `匿名学习者` label. This ADR replaces the
user-uploaded Ranking Profile mechanism with platform identity: a ranking
entry is the learner's platform `users.display_name`, resolved at read time by
Portal Gateway through a new Platform Core batch boundary, with a neutral
`游客x` label for guests and learners without a display name. Guests may now
rank; there is no opt-in/opt-out.

## Decision

- **Nickname source.** The public ranking `nickname` is the Platform Core
  `users.display_name` (NULL or 1-80 characters), resolved in real time by
  Portal Gateway via the new read-only batch interface
  `POST /api/v1/users/display-names` (five-line HMAC, reusing the existing
  `PLATFORM_CORE_URL` / `PLATFORM_CLIENT_ID` / `PLATFORM_SECRET` /
  `PLATFORM_KEY_ID` credentials; no session token). An unset/whitespace name,
  an unknown id, or a guest learner renders the neutral label `游客x`, where
  `x` is a stable number derived from the internal identity key
  (`FNV-1a(identity key) % 9000 + 1000`, e.g. `游客417`); the same identity
  keeps the same number across weeks.
- **Ranking Profile mechanism removed.** The `PATCH /api/v1/ranking-profile`
  route, its handler, and its SQL are deleted; `quizcraft_ranking_profiles`
  is frozen, not dropped (baseline-list compatible, reversible). There is no
  visibility opt-in/opt-out: every learner with a correct answer ranks.
- **Guests rank.** Anonymous learners are attributed through
  `COALESCE(a.user_id, s.actor_key)` on `quizcraft_practice_attempts` joined
  to their immutable `quizcraft_practice_sessions`, so guest attempts (old
  data included) count once per session/question; settlement idempotency is
  unchanged (`UNIQUE(period_start, period_end, scope, bank_id)`). The
  internal ranking contract carries `user_id` (nullable — null for guests);
  residual rows with neither `user_id` nor an `actor_key` are excluded.
- **system_avatar.** The external `system_avatar` field is kept and derived
  deterministically from the internal identity key (`FNV-1a(identity key) % 4`
  over the four patterns `scholar-blue`, `coder-green`, `reader-amber`,
  `owl-purple`). The derivation is stable, not user-controlled, and carries no
  personal data.
- **Privacy boundary unchanged (ADR-0036 §2, amended below).** The browser
  response never contains `user_id` or any UUID. The Go core → Gateway
  internal contract may carry `user_id`; Portal Gateway is the single
  stripping point and validates that every entry carries the field
  (present but nullable, no duplicates) so an old-contract Core response
  fails closed (502) instead of being silently mislabelled as guests.
- **Display-name cache and degradation.** Portal Gateway caches resolved
  display names in-process (10-minute TTL, bounded entries, singleflight on
  concurrent misses). When the Platform Core boundary is unavailable, ranking
  stays available: every entry degrades to `游客x` and the failure is logged.
  There is no active invalidation channel; a rename takes effect within the
  TTL (rankings are not a strong-consistency surface).

## 游客稳定编号（guest_key）

- **背景。** 游客条目的 `user_id` 为 null；若 Gateway 只按 `user_id` 派生
  展示名，所有游客会共享同一个「游客x」编号与头像。为让不同游客各自拥有
  稳定编号，Go core 内部排行契约在条目上额外携带可空的 `guest_key`：游客
  （`user_id` 为 null）输出其不可变 session `actor_key` 的文本身份键
  （`guest:<uuid>`），登录用户为 null。
- **边界。** `guest_key` 是 `x-internal` 匿名标识：**不落库、不对外下发**。
  Portal Gateway 仅将其用作 FNV-1a 派生的输入（身份键 = `user_id` ?? 
  `guest_key`，登录用 `user_id`、游客用 `guest_key`），对外响应结构与
  ADR-0036 隐私契约完全不变（不含 `user_id`/`guest_key` 任何键）。
- **Gateway 契约纪律。** 每个条目必须携带 `user_id` 与 `guest_key` 字段
  （present but nullable）；游客条目（`user_id` 为 null）必须携带非空且
  唯一的 `guest_key`，缺失或重复即 502；登录条目的 `guest_key` 必须为
  null。缺 `guest_key` 的旧契约 Core 响应一律 502，绝不当作游客混排。
- **编号稳定性与风险。** 同一游客（同一 cookie 身份）的编号跨周稳定；
  游客 cookie 轮换或清除后身份键变化、编号随之变化，仍属接受风险（与
  匿名归属模型一致，见研究文档 R3）。

## Amendment to ADR-0036 §2 (privacy contract)

- "Learners without a visible profile are absent from the ranking" is
  **removed** — there is no profile concept; guests and learners without a
  display name both rank with the neutral `游客x` label.
- "empty nicknames render the neutral label `匿名学习者`" becomes "an unset
  `users.display_name` (or a guest learner) renders the neutral `游客x`
  label".
- Added: the internal contract may carry `user_id`; Portal Gateway must strip
  it and derive nickname/system_avatar itself; the external contract is
  unchanged.

## Consequences

- The Gateway ranking read path gains one read-only Platform Core dependency;
  a Platform Core outage degrades nicknames to `游客x` (ranking availability
  outranks nickname freshness) and is logged.
- `display_name` renames propagate within the 10-minute cache TTL; rankings
  are not strongly consistent.
- Settlement events stay immutable; new event standings are shaped
  `{user_id (nullable), rank, correct_answer_count}` and names are resolved at
  display time by the consuming layer.
- The old `quizcraft_ranking_profiles` rows are inert: no read path consumes
  them, and the frozen table can be dropped later behind a dedicated
  migration plus baseline-list update if desired.

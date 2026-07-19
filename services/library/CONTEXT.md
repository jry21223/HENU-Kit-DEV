# Library Compatibility Context

## Language

**Library Module**:
HENUKit Console 中管理课程、资料、下载、投稿、审核和纠错的 Active Product Module；当前边界不包含刷题、社区、动态、支付、积分或会员。
_Avoid_: Study Legacy Admin、学习平台后台

**Compatibility Adapter**:
迁移期连接 Study Legacy API 的受限兼容层；它支撑连续运营与回滚，但不是 HENUKit Console 的导航概念，也不能扩展 Library 的产品边界。
_Avoid_: 旧功能入口、临时新功能、旧数据库直连

## Owns

- The bounded HTTP translation from Library terms to existing Study Legacy API operations.
- Durable 24-hour idempotency results and append-only adapter audit events.
- Explicit degraded state when one or more legacy Library sources are unavailable.

## Does not own

- Courses, materials, downloads, submissions, or correction business facts; Study Legacy API remains their migration-era owner.
- Platform accounts, Console Sessions, permissions, Scope, or Console presentation state.
- Quiz, community, payment, points, membership, package, entitlement, or commerce capabilities.

## Current boundary

HC-14 exposes only courses, materials, downloads, material submissions/reviews, and course/material corrections. Console Gateway verifies `library.*` with product Scope `library`, signs the actor context, and never connects to the legacy database. The Adapter holds a dedicated legacy API credential and maps old commercial access values to the opaque `restricted` state instead of exposing payment or membership concepts.

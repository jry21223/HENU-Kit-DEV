# Library Context

## Language

**Library Module**:
HENUKit Console 中管理课程、资料、下载、投稿、审核和纠错的 Active Product Module；当前边界不包含刷题、社区、动态、支付、积分或会员。
_Avoid_: Study Legacy Admin、学习平台后台

**Compatibility Adapter**:
迁移期连接 Study Legacy API 的受限兼容层；它支撑连续运营与回滚，但不是 HENUKit Console 的导航概念，也不能扩展 Library 的产品边界。
_Avoid_: 旧功能入口、临时新功能、旧数据库直连

**Active Public Material**:
当前活动 OSS release 中由 Library 拥有的 `published`、`public_free` 资料记录；它绑定稳定 material ID 与精确 release、receipt、Object key、VersionId、bytes、SHA-256 和安全文件名。
_Avoid_: storage_key 直链、当前路径、OSS 目录项

**Signed Download Grant**:
Library 对一个 Active Public Material 成功校验后签发的精确版本、仅 GET、最长 60 秒的临时下载能力；它不是文件已下载完成的证明。
_Avoid_: OSS 公共直链、永久下载地址、下载完成

**Download Start**:
Library 已持久化一个成功签发 grant 的不可变业务事实；重复成功请求分别计数，失败请求不计数。
_Avoid_: OSS 请求数、SLS 下载数、唯一下载人数

**Owner Catalog Activation**:
Library 对完整审核 manifest、sealed receipt、同 release 派生摘要与精确 OSS VersionId 全部验证后，在一个事务中切换的活动公开免费资料目录；失败保留上一完整活动版本，回滚是重新激活受审保留版本。
_Avoid_: 逐行导入、当前 OSS 目录、部分激活、覆盖回滚

**Public Material Catalog Snapshot**:
Library 在一个数据库读快照中返回的完整活动资料集合、稳定逐资料 Download Start 次数与全局累计次数；它不接受或应用浏览器搜索和筛选。
_Avoid_: 当前筛选结果、Portal 汇总值、硬编码下载量

## Owns

- The bounded HTTP translation from Library terms to existing Study Legacy API operations.
- Durable 24-hour idempotency results and append-only adapter audit events.
- Explicit degraded state when one or more legacy Library sources are unavailable.
- The active public-free OSS material snapshot, download-grant decision,
  append-only Download Start ledger, material/global aggregates, and atomic
  Owner Catalog Activation history.

## Does not own

- Legacy courses, submissions, corrections, and legacy operational downloads;
  Study Legacy API remains their migration-era owner.
- Reviewed source manifests, sealed releases, OSS object bytes, Bucket policy,
  or OSS/SLS access logs.
- Platform accounts, Console Sessions, permissions, Scope, or Console presentation state.
- Quiz, community, payment, points, membership, package, entitlement, or commerce capabilities.

## Current boundary

HC-14 exposes only courses, materials, downloads, material submissions/reviews, and course/material corrections. Console Gateway verifies `library.*` with product Scope `library`, signs the actor context, and never connects to the legacy database. The Adapter holds a dedicated legacy API credential and maps old commercial access values to the opaque `restricted` state instead of exposing payment or membership concepts.

ADR-0027 adopts #250 option A only for the public-free OSS download surface.
Portal Gateway may request a grant through one exact anonymous service route;
it does not restore a generic Portal Library owner client or move legacy
operations out of the Compatibility Adapter. Issue #333 uses only a controlled
active fixture, #334 owns production catalog activation, and #335 owns
production download and rollback evidence.
ADR-0029 makes the complete verified owner catalog and its unfiltered aggregate
read atomic. Its activation command remains unwired to a public HTTP route;
passing local tests is not production activation evidence.

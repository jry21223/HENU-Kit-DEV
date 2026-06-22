# Security

## Hard Rules

- The plugin is only a sales front desk and conversation adapter.
- final-review-platform is the only authority for package price, order status, payment confirmation, entitlement, delivery, download, and watermark.
- The plugin must not store WeChat Pay merchant private key, API v3 key, platform certificates, or course PDFs.
- The plugin must not send paid PDFs in group or private chat.
- The plugin must not grant access based on frontend polling or user screenshots.

## Agent API Authentication

Round one is mock-only. The planned real API calls must include:

- `Authorization: Bearer <FINAL_REVIEW_AGENT_SECRET>`
- `X-Agent-Id: langbot-final-review-sales`
- `X-Request-Id: <uuid>`

HMAC headers are reserved for the next version:

- `X-Timestamp`
- `X-Nonce`
- `X-Signature`

## Prompt Guard

`final_review_sales/prompts.py` blocks unsafe claims:

- 包过
- 必中
- 押题必中
- 内部资料
- 泄题
- 原题
- 保过
- 老师给的
- 百分百

Blocked output is replaced with a conservative statement that materials are only official platform packages and do not guarantee exam outcomes.

## Delivery Guard

`create_delivery` and `resend_delivery` can only return links after backend/mock order status is `paid`. Repeated delivery returns the same link.

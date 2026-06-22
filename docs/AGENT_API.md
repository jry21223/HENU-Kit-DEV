# Agent API

Planned API for the LangBot sales plugin. These endpoints are not implemented in the current backend round.

Authentication headers:

- `Authorization: Bearer <LANGBOT_AGENT_SECRET>`
- `X-Agent-Id: langbot-final-review-sales`
- `X-Request-Id: <uuid>`

Endpoints:

- `GET /api/agent/catalog/search`
- `GET /api/agent/packages/:id`
- `POST /api/agent/orders`
- `GET /api/agent/orders/:id/status`
- `POST /api/agent/deliveries/create`
- `POST /api/agent/deliveries/resend`
- `POST /api/agent/conversations/log`

Security rules:

- Plugin must not send amount or price when creating orders.
- Backend reads price from the published package.
- Backend creates the WeChat Native order and returns `codeUrl`.
- Plugin polling must not grant entitlement.
- Delivery can only be created for paid orders.
- Delivery links must be short-lived and auditable.

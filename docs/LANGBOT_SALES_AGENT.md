# LangBot Sales Agent

`final-review-sales-agent/` is a LangBot SDK plugin for the first sales-front round.

Current status:

- Plugin skeleton is implemented with official LangBot `manifest.yaml`, `main.py`, Command, Tool, and Event Listener components.
- All package/order/delivery behavior is mock-only inside the plugin.
- final-review-platform Agent API is not implemented yet.
- The platform remains the only authority for package price, WeChat Pay notify, paid status, entitlement, delivery, download, and PDF watermark.

Next backend round:

1. Add `/api/agent/catalog/search`.
2. Add `/api/agent/packages/[id]`.
3. Add `/api/agent/orders`.
4. Add `/api/agent/orders/[id]/status`.
5. Add `/api/agent/deliveries/create`.
6. Add `/api/agent/deliveries/resend`.
7. Add `/api/agent/conversations/log`.

The plugin must not receive WeChat merchant keys or course PDFs.

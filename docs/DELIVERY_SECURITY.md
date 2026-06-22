# Delivery Security

Delivery is a backend responsibility. The LangBot plugin only requests a delivery link after payment is confirmed by final-review-platform.

Required backend rules:

- Only paid orders can create delivery links.
- One order should have at most one active delivery.
- Store delivery token hashes, not plaintext tokens.
- Delivery links expire.
- Opening a delivery link must require student email login or account binding.
- Paid material download still checks entitlement server-side.
- PDF watermarking remains in the platform download path.
- Group chat must never receive paid PDFs.
- Re-send returns an existing valid delivery link instead of creating unlimited delivery records.

The first plugin round implements these rules only in mock tests. Real enforcement must be implemented in the backend Agent API round.

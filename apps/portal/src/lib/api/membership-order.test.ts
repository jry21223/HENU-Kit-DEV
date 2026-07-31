import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const ORDER = {
  id: "9f1c2a44-6f3d-4c2b-9a71-2b6d5e8c4a10",
  plan: "lifetime" as const,
  amount_cents: 990,
  status: "pending_payment" as const,
  version: 2,
  created_at: "2026-07-31T00:00:00Z",
  updated_at: "2026-07-31T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("createAccountMembershipOrder", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("sends no amount or merchant order number, so the browser cannot influence the charge", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse(
        {
          data: { order: ORDER, checkout_url: "weixin://wxpay/bizpayurl?pr=A1b2C3d" },
          request_id: "req_membership_order",
        },
        201
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createAccountMembershipOrder } = await import("./client");
    const response = await createAccountMembershipOrder("membership-key-0001");

    expect(response.data.checkout_url).toBe("weixin://wxpay/bizpayurl?pr=A1b2C3d");

    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/account/membership-orders");
    expect(init.method).toBe("POST");
    expect((init.headers as Record<string, string>)["Idempotency-Key"]).toBe("membership-key-0001");
    expect(JSON.parse(String(init.body))).toEqual({});
  });

  it("accepts an order that has no checkout handle rather than inventing one", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ data: { order: ORDER }, request_id: "req_membership_order" }, 201)
    );
    vi.stubGlobal("fetch", fetch);

    const { createAccountMembershipOrder } = await import("./client");
    const response = await createAccountMembershipOrder("membership-key-0002");

    // An absent handle means "no scannable code", never "assume it worked".
    expect(response.data.checkout_url).toBeUndefined();
    expect(response.data.order.status).toBe("pending_payment");
  });

  it("surfaces a disabled payment provider as an error rather than a created order", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse(
        { error: "Membership payment is not available", request_id: "req_membership_order" },
        503
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createAccountMembershipOrder } = await import("./client");
    await expect(createAccountMembershipOrder("membership-key-0003")).rejects.toBeDefined();
  });
});

describe("fetchAccountMembershipOrders", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("reads the user's own orders without caching them", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ data: { orders: [ORDER] }, request_id: "req_membership_orders" })
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchAccountMembershipOrders } = await import("./client");
    const response = await fetchAccountMembershipOrders();

    expect(response.data.orders).toHaveLength(1);
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/account/membership-orders");
    expect(init.cache).toBe("no-store");
  });
});

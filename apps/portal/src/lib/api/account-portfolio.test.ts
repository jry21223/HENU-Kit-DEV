import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("fetchAccountSummary", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("returns the Gateway's persisted zero state without a mock fallback", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            points_balance: 0,
            plan: "free",
            lifetime: false,
            unread_notification_count: 0,
            open_ticket_count: 0,
          },
          request_id: "req_account_summary",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchAccountSummary } = await import("./client");
    await expect(fetchAccountSummary()).resolves.toEqual({
      data: {
        points_balance: 0,
        plan: "free",
        lifetime: false,
        unread_notification_count: 0,
        open_ticket_count: 0,
      },
      request_id: "req_account_summary",
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/account/summary",
      expect.objectContaining({ credentials: "include" })
    );
  });

  it("surfaces a dependency failure instead of fabricating a successful summary", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "account_portfolio_unavailable", request_id: "req_owner_down" }),
          { status: 503, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { fetchAccountSummary, PortalHttpError } = await import("./client");
    await expect(fetchAccountSummary()).rejects.toBeInstanceOf(PortalHttpError);
  });
});

describe("fetchSession", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("never permits browser caches to retain a Portal Session response", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          user_id: "11111111-1111-4111-8111-111111111111",
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchSession } = await import("./client");
    await expect(fetchSession()).resolves.toMatchObject({ display_name: "小河同学" });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/session",
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });
});

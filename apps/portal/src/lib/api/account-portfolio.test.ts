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

  it("reads the durable membership entitlement from the Gateway without a session fallback", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: { plan: "lifetime", lifetime: true },
          request_id: "req_membership_lifetime",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchAccountMembership } = await import("./client");
    await expect(fetchAccountMembership()).resolves.toEqual({
      data: { plan: "lifetime", lifetime: true },
      request_id: "req_membership_lifetime",
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/account/membership",
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });

  it("reads a real bounded point-ledger page and preserves the opaque continuation cursor", async () => {
    const cursor = "plc1.b9Nl4wX2vJm9_0DK-cW1H3s9pQm8aXoZr2LtE5yYv7g";
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            balance: 90,
            entries: [{ id: "11111111-1111-4111-8111-111111111111", amount: 30, reason: "运营补偿", created_at: "2026-07-28T00:00:00Z" }],
            next_cursor: cursor,
          },
          request_id: "req_points",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchAccountPoints } = await import("./client");
    await expect(fetchAccountPoints()).resolves.toMatchObject({
      data: { balance: 90, entries: [expect.objectContaining({ amount: 30 })], next_cursor: cursor },
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/account/points?limit=20",
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });

  it("fails locally for an invalid point-ledger cursor instead of issuing a different request", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { fetchAccountPoints, PortalApiError } = await import("./client");
    await expect(fetchAccountPoints(" ")).rejects.toBeInstanceOf(PortalApiError);
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("Account Portfolio ticket and notification commands", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("uses the Gateway for persistent ticket reads and writes with the supplied idempotency key", async () => {
    const ticketID = "11111111-1111-4111-8111-111111111111";
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: {
              tickets: [
                {
                  id: ticketID,
                  reference: "HKT-11111111-1111-4111-8111-111111111111",
                  title: "练习记录问题",
                  category: "practice",
                  status: "open",
                  version: 1,
                  created_at: "2030-01-01T00:00:00Z",
                  updated_at: "2030-01-01T00:00:00Z",
                },
              ],
            },
            request_id: "req_tickets",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: {
              ticket: {
                id: ticketID,
                reference: "HKT-11111111-1111-4111-8111-111111111111",
                title: "练习记录问题",
                category: "practice",
                status: "open",
                version: 1,
                created_at: "2030-01-01T00:00:00Z",
                updated_at: "2030-01-01T00:00:00Z",
              },
            },
            request_id: "req_ticket_created",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } }
        )
      );
    vi.stubGlobal("fetch", fetch);

    const { createAccountTicket, fetchAccountTickets } = await import("./client");
    await expect(fetchAccountTickets()).resolves.toMatchObject({
      data: { tickets: [expect.objectContaining({ id: ticketID, version: 1 })] },
    });
    await expect(
      createAccountTicket(
        { title: "练习记录问题", category: "practice", body: "请帮我核对这次作答。" },
        "portal-ticket:retry-0001"
      )
    ).resolves.toMatchObject({ data: { ticket: expect.objectContaining({ id: ticketID }) } });

    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/account/tickets",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          "Idempotency-Key": "portal-ticket:retry-0001",
        }),
        body: JSON.stringify({
          title: "练习记录问题",
          category: "practice",
          body: "请帮我核对这次作答。",
        }),
      })
    );
  });

  it("does not turn a ticket version conflict into a successful local update", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "ticket_version_conflict", request_id: "req_conflict" }),
          { status: 409, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { createAccountTicketFollowUp } = await import("./client");
    await expect(
      createAccountTicketFollowUp(
        "11111111-1111-4111-8111-111111111111",
        { body: "补充说明", expected_version: 1 },
        "portal-followup:retry-0001"
      )
    ).rejects.toMatchObject({ status: 409 });
  });

  it("marks only the persistent notification returned by the Gateway as read", async () => {
    const notificationID = "22222222-2222-4222-8222-222222222222";
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            notification: {
              id: notificationID,
              title: "工单有新回复",
              body: "请查看客服回复。",
              kind: "ticket_operator_reply",
              created_at: "2030-01-01T00:00:00Z",
              read_at: "2030-01-01T00:02:00Z",
            },
          },
          request_id: "req_notice_read",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { markAccountNotificationRead } = await import("./client");
    await expect(
      markAccountNotificationRead(notificationID, "portal-notification:retry-0001")
    ).resolves.toMatchObject({
      data: { notification: expect.objectContaining({ id: notificationID, read_at: expect.any(String) }) },
    });
    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/account/notifications/${notificationID}/read`,
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({ "Idempotency-Key": "portal-notification:retry-0001" }),
      })
    );
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

describe("logout", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("requires a successful Gateway cookie-clear response", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ status: "signed_out" }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { logout } = await import("./client");
    await expect(logout()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/session/logout",
      expect.objectContaining({ method: "POST", cache: "no-store", credentials: "include" })
    );
  });

  it("revokes the Platform Core session after the Gateway cookie-clear", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({ status: "signed_out" }),
            { status: 200, headers: { "Content-Type": "application/json" } }
          )
        )
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({ revoked: true }),
            { status: 200, headers: { "Content-Type": "application/json" } }
          )
        )
    );

    const { logout } = await import("./client");
    await expect(logout()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/account-auth/api/v1/sessions/revoke",
      expect.objectContaining({
        method: "POST",
        cache: "no-store",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ all_sessions: false }),
      })
    );
  });

  it("still completes local logout when Core revocation fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({ status: "signed_out" }),
            { status: 200, headers: { "Content-Type": "application/json" } }
          )
        )
        .mockRejectedValueOnce(new TypeError("core revoke network failure"))
    );

    const { logout } = await import("./client");
    await expect(logout()).resolves.toBeUndefined();
  });

  it("does not report logout success when the Gateway is unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "portal_gateway_unavailable" }),
          { status: 503, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { logout, PortalHttpError } = await import("./client");
    await expect(logout()).rejects.toBeInstanceOf(PortalHttpError);
  });
});

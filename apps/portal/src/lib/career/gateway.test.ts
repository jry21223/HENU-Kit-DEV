import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const SEARCH_ID = "11111111-1111-4111-8111-111111111111";
const ACTOR_USER_ID = "22222222-2222-4222-8222-222222222222";

describe("Career client", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("creates a search with the supplied idempotency key and returns the queued search", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          search: {
            id: SEARCH_ID,
            status: "queued",
            stage: "crawling",
            user_id: ACTOR_USER_ID,
            has_email: false,
            created_at: "2030-01-01T00:00:00Z",
          },
          request_id: "req_career_create",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createCareerSearch } = await import("../api/client");
    await expect(
      createCareerSearch(
        { target_roles: "后端开发", job_type: "daily_intern" },
        "career:retry-0001"
      )
    ).resolves.toMatchObject({
      search: expect.objectContaining({ id: SEARCH_ID, status: "queued" }),
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/career/searches",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          "Idempotency-Key": "career:retry-0001",
        }),
        body: JSON.stringify({
          profile: { target_roles: "后端开发", job_type: "daily_intern" },
        }),
      })
    );
  });

  it("maps the 403 lifetime_required gate to a branchable forbidden error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              error: "lifetime_required",
              message: "求职雷达需要 Lifetime VIP 会员",
              request_id: "req_gate_read",
            }),
            { status: 403, headers: { "Content-Type": "application/json" } }
          )
        )
      )
    );

    const { createCareerSearch, PortalForbiddenError } = await import("../api/client");
    const {
      CAREER_LIFETIME_REQUIRED_CODE,
      isCareerLifetimeRequiredError,
    } = await import("./gateway");

    const err = await createCareerSearch(
      { target_roles: "x" },
      "career:retry-0002"
    ).catch((e) => e);
    expect(err).toBeInstanceOf(PortalForbiddenError);
    expect(err.status).toBe(403);
    expect(err.errorCode).toBe(CAREER_LIFETIME_REQUIRED_CODE);
    expect(isCareerLifetimeRequiredError(err)).toBe(true);
    expect(isCareerLifetimeRequiredError(new Error("unrelated"))).toBe(false);
    expect(isCareerLifetimeRequiredError(null)).toBe(false);
  });

  it("maps 401 to PortalUnauthorizedError for profile reads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "unauthorized", request_id: "req_anon" }),
          { status: 401, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { getCareerProfile, PortalUnauthorizedError } = await import("../api/client");
    await expect(getCareerProfile()).rejects.toBeInstanceOf(
      PortalUnauthorizedError
    );
  });

  it("reads an empty profile (only required fields) without inventing defaults", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          profile: {
            user_id: ACTOR_USER_ID,
            updated_at: "2030-01-01T00:00:00Z",
          },
          request_id: "req_empty_profile",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { getCareerProfile } = await import("../api/client");
    const resp = await getCareerProfile();
    expect(resp.profile.user_id).toBe(ACTOR_USER_ID);
    expect(resp.profile.target_roles).toBeUndefined();
    expect(resp.profile.email_notification_enabled).toBeUndefined();
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/career/profile",
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });
});

describe("Career gateway", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("surfaces a 503 as an error string instead of falling back to mock", async () => {
    // init 会并发发起 profile + searches 两次读，每次都要独立的 Response
    //（同一 Response 的 body 只能消费一次）。
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async () =>
        new Response(
          JSON.stringify({ error: "career_unavailable", request_id: "req_down" }),
          { status: 503, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { loadCareerData } = await import("./gateway");
    const result = await loadCareerData();
    expect(result.profile).toBeNull();
    expect(result.searches).toEqual([]);
    expect(result.error).toBe("服务暂时不可用，请稍后再试。");
  });

  it("reports the lifetime gate message when the gateway rejects profile reads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async () =>
        new Response(
          JSON.stringify({
            error: "lifetime_required",
            message: "求职雷达需要 Lifetime VIP 会员",
            request_id: "req_gate_read",
          }),
          { status: 403, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { loadCareerData } = await import("./gateway");
    const result = await loadCareerData();
    expect(result.profile).toBeNull();
    expect(result.error).toBe("求职雷达需要 Lifetime VIP 会员，开通后即可使用");
  });
});

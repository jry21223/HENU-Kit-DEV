import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const SESSION_USER_ID = "22222222-2222-4222-8222-222222222222";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** 按路径分发 mock 响应；未命中的路径一律 404。 */
function routeFetch(routes: Record<string, () => Response>): ReturnType<typeof vi.fn> {
  return vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    const handler = routes[url];
    if (!handler) {
      return jsonResponse({ error: "not_found", request_id: "req_miss" }, 404);
    }
    return handler();
  });
}

const fullProfile = {
  user_id: SESSION_USER_ID,
  target_roles: "后端开发",
  tech_stack: "go,postgres",
  locations: "郑州",
  job_type: "daily_intern" as const,
  graduation_year: 2027,
  resume_text: "校内项目经历",
  email_notification_enabled: true,
  updated_at: "2026-08-15T00:00:00Z",
};

describe("resolveCareerView", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("resolves anonymous when the session endpoint says not authenticated", async () => {
    const fetch = routeFetch({
      "/api/v1/session": () => jsonResponse({ error: "unauthorized", request_id: "req_anon" }, 401),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toEqual({ kind: "anonymous" });
    // 匿名态不得触碰会员或求职数据。
    expect(
      fetch.mock.calls.every((call) => String(call[0]) === "/api/v1/session")
    ).toBe(true);
  });

  it("resolves free for a signed-in non-lifetime member and never reads career data", async () => {
    const fetch = routeFetch({
      "/api/v1/session": () =>
        jsonResponse({
          user_id: SESSION_USER_ID,
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
      "/api/v1/account/membership": () =>
        jsonResponse({
          data: { plan: "free", lifetime: false },
          request_id: "req_membership",
        }),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toEqual({ kind: "free" });
    expect(
      fetch.mock.calls.every((call) => !String(call[0]).includes("/career"))
    ).toBe(true);
  });

  it("resolves lifetime-no-profile when the profile is empty", async () => {
    const fetch = routeFetch({
      "/api/v1/session": () =>
        jsonResponse({
          user_id: SESSION_USER_ID,
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
      "/api/v1/account/membership": () =>
        jsonResponse({
          data: { plan: "lifetime", lifetime: true },
          request_id: "req_membership",
        }),
      "/api/v1/career/profile": () =>
        jsonResponse({
          profile: { user_id: SESSION_USER_ID, updated_at: "2026-08-15T00:00:00Z" },
          request_id: "req_profile",
        }),
      "/api/v1/career/searches": () =>
        jsonResponse({ searches: [], request_id: "req_searches" }),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toEqual({
      kind: "lifetime-no-profile",
    });
  });

  it("resolves lifetime-ready with profile and searches", async () => {
    const search = {
      id: "11111111-1111-4111-8111-111111111111",
      status: "queued" as const,
      user_id: SESSION_USER_ID,
      has_email: false,
      created_at: "2026-08-15T00:00:00Z",
    };
    const fetch = routeFetch({
      "/api/v1/session": () =>
        jsonResponse({
          user_id: SESSION_USER_ID,
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
      "/api/v1/account/membership": () =>
        jsonResponse({
          data: { plan: "lifetime", lifetime: true },
          request_id: "req_membership",
        }),
      "/api/v1/career/profile": () =>
        jsonResponse({ profile: fullProfile, request_id: "req_profile" }),
      "/api/v1/career/searches": () =>
        jsonResponse({ searches: [search], request_id: "req_searches" }),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toEqual({
      kind: "lifetime-ready",
      profile: fullProfile,
      searches: [search],
    });
  });

  it("surfaces a membership read failure as an error state", async () => {
    const fetch = routeFetch({
      "/api/v1/session": () =>
        jsonResponse({
          user_id: SESSION_USER_ID,
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
      "/api/v1/account/membership": () =>
        jsonResponse({ error: "portfolio_unavailable", request_id: "req_down" }, 503),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toMatchObject({
      kind: "error",
      message: "服务暂时不可用，请稍后再试。",
    });
  });

  it("surfaces the lifetime gate message when career reads are rejected", async () => {
    const fetch = routeFetch({
      "/api/v1/session": () =>
        jsonResponse({
          user_id: SESSION_USER_ID,
          display_name: "小河同学",
          expires_at: "2030-01-01T00:00:00Z",
        }),
      "/api/v1/account/membership": () =>
        jsonResponse({
          data: { plan: "lifetime", lifetime: true },
          request_id: "req_membership",
        }),
      "/api/v1/career/profile": () =>
        jsonResponse(
          {
            error: "lifetime_required",
            message: "求职雷达需要 Lifetime VIP 会员",
            request_id: "req_gate",
          },
          403
        ),
      "/api/v1/career/searches": () =>
        jsonResponse(
          {
            error: "lifetime_required",
            message: "求职雷达需要 Lifetime VIP 会员",
            request_id: "req_gate",
          },
          403
        ),
    });
    vi.stubGlobal("fetch", fetch);

    const { resolveCareerView } = await import("./page-state");
    await expect(resolveCareerView()).resolves.toEqual({
      kind: "error",
      message: "求职雷达需要 Lifetime VIP 会员，开通后即可使用",
    });
  });
});

describe("isCareerProfileReady", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("is false for missing or blank target_roles", async () => {
    const { isCareerProfileReady } = await import("./page-state");
    expect(isCareerProfileReady({ user_id: "u", updated_at: "" })).toBe(false);
    expect(
      isCareerProfileReady({ user_id: "u", updated_at: "", target_roles: "  " })
    ).toBe(false);
  });

  it("is true when target_roles has content", async () => {
    const { isCareerProfileReady } = await import("./page-state");
    expect(
      isCareerProfileReady({ user_id: "u", updated_at: "", target_roles: "后端开发" })
    ).toBe(true);
  });
});

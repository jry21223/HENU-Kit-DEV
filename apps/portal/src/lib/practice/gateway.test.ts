import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * The Practice gateway decides whether the Practice surface is allowed to
 * render. The property that matters is the one Portal Gateway enforces
 * server-side: when the gateway is required, a failed load must leave the
 * module not-ready rather than quietly ready on mock data.
 */

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function requireGateway() {
  vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "https://gateway.example.edu");
}

function allowMockWithoutGateway() {
  vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
}

describe("Practice gateway", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("loads schools and banks from the gateway", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/practice/schools")) {
          return jsonResponse({ schools: [{ id: "s-1", name: "河南大学" }] });
        }
        return jsonResponse({ banks: [{ id: "b-1", name: "离散数学" }] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(gateway.getPracticeGatewayError()).toBeNull();
    expect(gateway.isPracticeReady()).toBe(true);
    expect(gateway.getGatewaySchools()).toHaveLength(1);
    expect(gateway.getGatewayBanks()).toHaveLength(1);
  });

  it("stays ready when only the school hierarchy resolves", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/practice/schools")) {
          return jsonResponse({ schools: [{ id: "s-1", name: "河南大学" }] });
        }
        return jsonResponse({ error: { code: "UPSTREAM" } }, 502);
      })
    );

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(gateway.isPracticeReady()).toBe(true);
    expect(gateway.getGatewaySchools()).toHaveLength(1);
    expect(gateway.getGatewayBanks()).toBeNull();
  });

  it("refuses to become ready when both reads fail and mock is not allowed", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: { code: "UPSTREAM" } }, 502))
    );

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(gateway.isPracticeReady()).toBe(false);
    expect(gateway.getGatewaySchools()).toBeNull();
    expect(gateway.getGatewayBanks()).toBeNull();
    expect(gateway.getPracticeGatewayError()).not.toBeNull();
  });

  // Neither a gateway URL nor an opt-in to mock: the surface must explain the
  // misconfiguration instead of rendering anything. Note an empty URL with
  // REQUIRE_GATEWAY=1 is *not* this case — that is the supported same-origin
  // deploy, where fetches go to /api/v1/... on the current host.
  it("reports a configuration error when neither a gateway nor mock is configured", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(gateway.isPracticeReady()).toBe(false);
    expect(gateway.getPracticeGatewayError()).toMatch(
      /NEXT_PUBLIC_PORTAL_GATEWAY_URL/
    );
  });

  it("treats an empty gateway URL with REQUIRE_GATEWAY as the same-origin deploy", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input).startsWith("/api/")).toBe(true);
      if (String(input).includes("/practice/schools")) {
        return jsonResponse({ schools: [{ id: "s-1", name: "河南大学" }] });
      }
      return jsonResponse({ banks: [] });
    });
    vi.stubGlobal("fetch", fetch);

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(fetch).toHaveBeenCalled();
    expect(gateway.isPracticeReady()).toBe(true);
  });

  it("is ready without a gateway only when mock is explicitly allowed", async () => {
    allowMockWithoutGateway();
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();

    expect(gateway.isPracticeReady()).toBe(true);
    expect(gateway.getPracticeGatewayError()).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("does not reload once it has succeeded", async () => {
    requireGateway();
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/practice/schools")) {
        return jsonResponse({ schools: [{ id: "s-1", name: "河南大学" }] });
      }
      return jsonResponse({ banks: [] });
    });
    vi.stubGlobal("fetch", fetch);

    const gateway = await import("./gateway");
    await gateway.initPracticeGateway();
    const callsAfterFirstLoad = fetch.mock.calls.length;
    await gateway.initPracticeGateway();

    expect(fetch.mock.calls.length).toBe(callsAfterFirstLoad);
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * getCampusItemOrFallback is the interesting one: the detail page reuses it,
 * and it must not reach into the fixture store for an item the gateway does
 * not know about unless mock is explicitly allowed.
 */

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const item = {
  id: "i-1",
  type: "sell",
  category: "book",
  title: "离散数学教材",
  desc: "",
  price: 20,
  seller: "同学",
  credit: 80,
  dealsDone: 0,
  wants: 0,
  place: "明伦",
  status: "open",
  time: "",
};

function requireGateway() {
  vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "https://gateway.example.edu");
}

describe("Campus gateway", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("loads items and categories from the gateway", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/campus/categories")) {
          return jsonResponse({ categories: [{ id: "book", name: "教材" }] });
        }
        return jsonResponse({ items: [item] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initCampusGateway();

    expect(gateway.isCampusReady()).toBe(true);
    expect(gateway.getCampusGatewayError()).toBeNull();
    expect(gateway.getGatewayItems()).toHaveLength(1);
    expect(gateway.getGatewayCategories()).toHaveLength(1);
  });

  it("keeps items when the optional category read fails", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/campus/categories")) {
          return jsonResponse({ error: { code: "UPSTREAM" } }, 502);
        }
        return jsonResponse({ items: [item] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initCampusGateway();

    expect(gateway.isCampusReady()).toBe(true);
    expect(gateway.getGatewayItems()).toHaveLength(1);
    expect(gateway.getGatewayCategories()).toBeNull();
  });

  it("refuses to become ready when the item read fails and mock is forbidden", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: { code: "UPSTREAM" } }, 502))
    );

    const gateway = await import("./gateway");
    await gateway.initCampusGateway();

    expect(gateway.isCampusReady()).toBe(false);
    expect(gateway.getGatewayItems()).toBeNull();
    expect(gateway.getCampusGatewayError()).not.toBeNull();
  });

  it("resolves a detail item from the gateway and reports no fixture messages", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/campus/categories")) {
          return jsonResponse({ categories: [] });
        }
        return jsonResponse({ items: [item] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initCampusGateway();

    expect(gateway.getCampusItemOrFallback("i-1")).toMatchObject({
      item: { id: "i-1" },
      messages: [],
    });
  });

  it("does not fall back to the fixture store for an unknown item when mock is forbidden", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/campus/categories")) {
          return jsonResponse({ categories: [] });
        }
        return jsonResponse({ items: [item] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initCampusGateway();

    expect(gateway.getCampusItemOrFallback("not-in-the-gateway")).toEqual({
      item: null,
      messages: [],
    });
  });

  it("serves fixture items and their messages only when mock is allowed", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    vi.stubGlobal("fetch", vi.fn());

    const gateway = await import("./gateway");
    const { campusStore } = await import("./mock");
    await gateway.initCampusGateway();

    expect(gateway.isCampusReady()).toBe(true);
    const seeded = campusStore.get().items[0];
    expect(gateway.getCampusItemOrFallback(seeded.id).item).toMatchObject({
      id: seeded.id,
    });
  });
});

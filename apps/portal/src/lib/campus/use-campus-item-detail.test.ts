import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("loadCampusItemDetail", () => {
  beforeEach(() => {
    vi.stubEnv("NODE_ENV", "test");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "0");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
  });

  afterEach(async () => {
    (await import("./gateway")).resetCampusGatewayCache();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it.each([
    ["HTTP 404", () => Response.json({ error: "not_found" }, { status: 404 })],
    ["hidden detail", () => Response.json({ item: { id: "h-01", status: "hidden" }, messages: [] })],
  ] as const)("ignores and evicts a cached visible item on %s", async (_label, response) => {
    const { getCampusItemOrFallback, getGatewayItems, rememberCampusItems } = await import("./gateway");
    const { campusStore } = await import("./mock");
    const { loadCampusItemDetail } = await import("./use-campus-item-detail");
    const cached = campusStore.get().items.find((item) => item.id === "h-01")!;
    const other = campusStore.get().items.find((item) => item.id === "h-02")!;
    rememberCampusItems([cached, other], null);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(response()).mockRejectedValueOnce(new TypeError("Offline")));

    expect(getCampusItemOrFallback(cached.id).item).toBe(cached);
    expect(await loadCampusItemDetail(cached.id, vi.fn())).toMatchObject({ loadState: "not-found", item: null, messages: [] });
    expect(getCampusItemOrFallback(cached.id).item).toBeNull();
    expect(getGatewayItems()).toEqual([other]);
    expect(await loadCampusItemDetail(cached.id, vi.fn())).toMatchObject({ loadState: "error", item: null });
  });

  it("only falls back to visible mock items in local mock mode", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "0");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { getCampusItemOrFallback, rememberCampusItems } = await import("./gateway");
    const { loadCampusItemDetail } = await import("./use-campus-item-detail");

    const visible = getCampusItemOrFallback("h-01").item!;
    expect(await loadCampusItemDetail(visible.id, vi.fn())).toMatchObject({ loadState: "ready", item: visible });
    for (const id of ["campus-unknown", "s-05"]) {
      expect(await loadCampusItemDetail(id, vi.fn())).toMatchObject({ loadState: "not-found", item: null, messages: [] });
    }
    rememberCampusItems([{ ...visible, status: "hidden" }], null);
    expect(getCampusItemOrFallback(visible.id)).toEqual({ item: null, messages: [] });
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each([
    ["503", () => Promise.resolve(Response.json({ error: "upstream_unavailable" }, { status: 503 }))],
    ["HTML", () => Promise.resolve(new Response("<!doctype html><html></html>", { headers: { "Content-Type": "text/html" } }))],
    ["network error", () => Promise.reject(new TypeError("Failed to fetch"))],
  ] as const)("keeps %s retryable unless a visible list item is cached", async (_label, fetchResponse) => {
    vi.stubGlobal("fetch", vi.fn(fetchResponse));
    const { rememberCampusItems } = await import("./gateway");
    const { campusStore } = await import("./mock");
    const { loadCampusItemDetail } = await import("./use-campus-item-detail");
    const cached = campusStore.get().items.find((item) => item.id === "h-01")!;
    const retry = vi.fn();

    expect(await loadCampusItemDetail(cached.id, retry)).toMatchObject({ loadState: "error", item: null, retry });
    rememberCampusItems([{ ...cached, status: "hidden" }], null);
    expect(await loadCampusItemDetail(cached.id, retry)).toMatchObject({ loadState: "error", item: null, retry });
    rememberCampusItems([cached], null);
    expect(await loadCampusItemDetail(cached.id, retry)).toMatchObject({ loadState: "ready", item: cached });
  });
});

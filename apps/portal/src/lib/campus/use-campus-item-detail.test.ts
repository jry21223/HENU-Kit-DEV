import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * 互助单详情在列表缓存也回退不到这条单子时，分清“单子不存在”（只显示 404 页）和
 * “暂时读不到”（说明原因并给重试），与资料详情一致。
 */
describe("campusItemIsMissing", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("treats a 404 from the item endpoint as missing", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(Response.json({ error: "not_found" }, { status: 404 }))
    );
    const { fetchCampusItemDetail } = await import("@/lib/api/client");
    const { campusItemIsMissing } = await import("./use-campus-item-detail");

    const error = await fetchCampusItemDetail("campus-gone").catch((cause: unknown) => cause);
    expect(campusItemIsMissing(error)).toBe(true);
  });

  it("treats an empty answer as missing: local demo data without the item has nothing to retry", async () => {
    // 本地演示模式：没有接口地址、允许演示数据。列表缓存和演示数据都没有这条，重试也不会有。
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "0");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { fetchCampusItemDetail } = await import("@/lib/api/client");
    const { campusItemIsMissing } = await import("./use-campus-item-detail");

    const error = await fetchCampusItemDetail("campus-unknown").catch((cause: unknown) => cause);
    expect(fetch).not.toHaveBeenCalled();
    expect(campusItemIsMissing(error)).toBe(true);
  });

  it("keeps outages, non-JSON pages and network failures retryable", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    const html = "<!DOCTYPE html><html><body>verify you are human</body></html>";
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(Response.json({ error: "upstream_unavailable" }, { status: 503 }))
        .mockResolvedValueOnce(new Response(html, { status: 200, headers: { "Content-Type": "text/html" } }))
        .mockRejectedValueOnce(new TypeError("Failed to fetch"))
    );
    const { fetchCampusItemDetail } = await import("@/lib/api/client");
    const { campusItemIsMissing } = await import("./use-campus-item-detail");

    for (const failure of ["503", "HTML page", "network"]) {
      const error = await fetchCampusItemDetail("campus-item").catch((cause: unknown) => cause);
      expect(error, failure).toBeInstanceOf(Error);
      expect(campusItemIsMissing(error), failure).toBe(false);
    }
  });
});

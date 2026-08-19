import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * The Library gateway must never serve STATIC_MATERIALS as if it were real
 * catalogue data. When the gateway is required, a failed load leaves the
 * surface empty and not-ready so the page can show its error banner.
 */

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const material = {
  id: "m-1",
  type: "book",
  subject: "离散数学",
  title: "离散数学讲义",
  author: "教研室",
  intro: "",
  price: 0,
  rating: 4.5,
  downloads: 10,
  favs: 2,
  downloadAvailable: true,
  fileSize: "1MB",
};

function requireGateway() {
  vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "https://gateway.example.edu");
}

describe("Library gateway", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("maps gateway materials into the Library model", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/library/courses")) {
          return jsonResponse({ courses: [{ id: "m-1" }] });
        }
        return jsonResponse({ materials: [material] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initGateway();

    expect(gateway.isLibraryReady()).toBe(true);
    expect(gateway.getLibraryGatewayError()).toBeNull();
    const materials = gateway.getMaterials();
    expect(materials).toHaveLength(1);
    expect(materials[0]).toMatchObject({ id: "m-1", title: "离散数学讲义" });
    // toc, pages and slides are not part of the API payload and must default
    // rather than arrive undefined.
    expect(materials[0].toc).toEqual([]);
    expect(materials[0].pages).toEqual([]);
    expect(gateway.getAvailableCourseIds().has("m-1")).toBe(true);
  });

  it("falls back to the material ids when the course hint is unavailable", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/library/courses")) {
          return jsonResponse({ error: { code: "UPSTREAM" } }, 502);
        }
        return jsonResponse({ materials: [material] });
      })
    );

    const gateway = await import("./gateway");
    await gateway.initGateway();

    expect(gateway.isLibraryReady()).toBe(true);
    expect(gateway.getAvailableCourseIds().has("m-1")).toBe(true);
  });

  it("serves nothing rather than STATIC_MATERIALS when the materials read fails", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: { code: "UPSTREAM" } }, 502))
    );

    const gateway = await import("./gateway");
    await gateway.initGateway();

    expect(gateway.isLibraryReady()).toBe(false);
    expect(gateway.getMaterials()).toEqual([]);
    expect(gateway.getMaterialOrFallback("m-1")).toBeUndefined();
    expect(gateway.getLibraryGatewayError()).not.toBeNull();
  });

  it("uses STATIC_MATERIALS only when mock is explicitly allowed", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    vi.stubGlobal("fetch", vi.fn());

    const gateway = await import("./gateway");
    await gateway.initGateway();

    expect(gateway.isLibraryReady()).toBe(true);
    expect(gateway.getMaterials().length).toBeGreaterThan(0);
  });

  it("toggles a favourite without mutating the caller's array", async () => {
    requireGateway();
    const { toggleFavViaGateway } = await import("./gateway");

    const favourites = ["m-1"];
    expect(toggleFavViaGateway("m-2", favourites)).toEqual(["m-1", "m-2"]);
    expect(toggleFavViaGateway("m-1", favourites)).toEqual([]);
    expect(favourites).toEqual(["m-1"]);
  });
});

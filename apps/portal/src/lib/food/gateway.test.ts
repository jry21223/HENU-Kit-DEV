import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/**
 * loadFoodPosts is the single entry point the homepage board and /food share,
 * so its mock/live decision is the one that decides whether a reader can be
 * shown fixture posts as if they were real submissions.
 */

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const post = {
  id: "p-1",
  campus: "minglun",
  title: "胡辣汤",
  excerpt: "",
  blocks: [],
  author: "楼下的猫",
  likes: 1,
  stars: 5,
  tags: [],
  shop: { name: "Shop A", lat: 0, lng: 0 },
  time: "",
  hidden: false,
};

function requireGateway() {
  vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "");
  vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "https://gateway.example.edu");
}

describe("Food gateway", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("loads posts and per-campus venues from the gateway", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/food/venues")) {
          const campus = new URL(url, "https://gateway.example.edu").searchParams.get(
            "campus"
          );
          return jsonResponse({ venues: [{ id: `v-${campus}`, name: "Shop A" }] });
        }
        return jsonResponse({ posts: [post] });
      })
    );

    const gateway = await import("./gateway");
    const result = await gateway.loadFoodPosts();

    expect(result.error).toBeNull();
    expect(result.posts).toHaveLength(1);
    expect(gateway.isFoodReady()).toBe(true);
    expect(gateway.getVenues("minglun")).toHaveLength(1);
    expect(gateway.getVenues("jinming")).toHaveLength(1);
  });

  it("keeps posts when the optional venue reads fail", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/food/venues")) {
          return jsonResponse({ error: { code: "UPSTREAM" } }, 502);
        }
        return jsonResponse({ posts: [post] });
      })
    );

    const gateway = await import("./gateway");
    const result = await gateway.loadFoodPosts();

    expect(result.error).toBeNull();
    expect(result.posts).toHaveLength(1);
    expect(gateway.getVenues("minglun")).toBeNull();
  });

  it("returns an error and no posts when the read fails and mock is forbidden", async () => {
    requireGateway();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: { code: "UPSTREAM" } }, 502))
    );

    const gateway = await import("./gateway");
    const result = await gateway.loadFoodPosts();

    expect(result.posts).toEqual([]);
    expect(result.error).toBeTruthy();
    expect(gateway.isFoodReady()).toBe(false);
    expect(gateway.getGatewayPosts()).toBeNull();
  });

  it("serves the fixture store only when mock is explicitly allowed", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    vi.stubEnv("NEXT_PUBLIC_PORTAL_GATEWAY_URL", "");
    vi.stubGlobal("fetch", vi.fn());

    const gateway = await import("./gateway");
    const result = await gateway.loadFoodPosts();

    expect(result.error).toBeNull();
    expect(result.posts.length).toBeGreaterThan(0);
  });

  it("does not refetch once loaded", async () => {
    requireGateway();
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/food/venues")) {
        return jsonResponse({ venues: [] });
      }
      return jsonResponse({ posts: [post] });
    });
    vi.stubGlobal("fetch", fetch);

    const gateway = await import("./gateway");
    await gateway.loadFoodPosts();
    const afterFirst = fetch.mock.calls.length;
    await gateway.loadFoodPosts();

    expect(fetch.mock.calls.length).toBe(afterFirst);
  });
});

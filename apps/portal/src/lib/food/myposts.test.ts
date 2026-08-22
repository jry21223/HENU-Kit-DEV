import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const MY_POST = {
  id: "55555555-5555-4555-8555-555555555555",
  campus: "jinming",
  title: "西门鸡腿饭",
  excerpt: "下课就来排队的鸡腿饭。",
  blocks: [{ type: "p", text: "人均 ¥12。" }],
  author: "小河同学",
  likes: 0,
  stars: 0,
  tags: ["食堂", "NPC"],
  shop: { name: "西门鸡腿饭" },
  time: "2030-01-02T03:04:05Z",
  hidden: false,
  images: [],
};

describe("fetchMyFoodPosts", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("requests the owner-scoped mine endpoint without caching and with session cookies", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ posts: [MY_POST], request_id: "req_my_posts" }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchMyFoodPosts } = await import("./myposts");
    await expect(fetchMyFoodPosts()).resolves.toEqual({
      posts: [MY_POST],
      request_id: "req_my_posts",
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/food/posts/mine",
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });

  it("turns a Gateway 401 into PortalUnauthorizedError instead of an empty list", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ error: "not_authenticated", request_id: "req_my_posts_401" }),
          { status: 401, headers: { "Content-Type": "application/json" } }
        )
      )
    );

    const { fetchMyFoodPosts } = await import("./myposts");
    const { PortalUnauthorizedError } = await import("../api/client");
    await expect(fetchMyFoodPosts()).rejects.toBeInstanceOf(PortalUnauthorizedError);
  });

  it("returns a durable empty list when the owner has no posts yet", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ posts: [], request_id: "req_my_posts_empty" }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchMyFoodPosts } = await import("./myposts");
    await expect(fetchMyFoodPosts()).resolves.toEqual({
      posts: [],
      request_id: "req_my_posts_empty",
    });
  });
});

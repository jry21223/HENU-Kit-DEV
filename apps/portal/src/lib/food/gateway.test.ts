import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FoodPost } from "@/lib/api/types";

function post(id: string, title: string): FoodPost {
  return {
    id,
    campus: "minglun",
    title,
    excerpt: "真实投稿内容",
    blocks: [],
    author: "同学",
    likes: 0,
    stars: 0,
    tags: ["夯"],
    shop: { name: title },
    time: "2030-01-01T00:00:00Z",
    hidden: false,
  };
}

describe("Food gateway cache", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("merges a successful publish that occurs while an older list request is in flight", async () => {
    let resolvePosts!: (response: Response) => void;
    const pendingPosts = new Promise<Response>((resolve) => {
      resolvePosts = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path === "/api/v1/food/posts") return pendingPosts;
        return new Response(JSON.stringify({ campus: "minglun", venues: [], request_id: "req_venues" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      })
    );

    const { getGatewayPosts, initFoodGateway, rememberCreatedFoodPost } = await import("./gateway");
    const initializing = initFoodGateway();
    await Promise.resolve();
    rememberCreatedFoodPost(post("new-post", "新投稿"));
    resolvePosts(
      new Response(JSON.stringify({ posts: [post("old-post", "旧投稿")], request_id: "req_posts" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );
    await initializing;

    expect(getGatewayPosts()?.map((item) => item.id)).toEqual(["new-post", "old-post"]);
  });
});

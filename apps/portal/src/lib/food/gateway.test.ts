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

  it("updates an older post in place before and after the owner list loads", async () => {
    const newer = {...post("newer", "新投稿"), time: "2030-01-02T00:00:00Z"};
    const older = post("older", "旧投稿");
    const fetch = vi.fn().mockImplementation(async () => new Response(JSON.stringify({posts: [newer, older], request_id: "req_posts"})));
    vi.stubGlobal("fetch", fetch);
    const {rememberUpdatedFoodPost, initFoodGateway, getGatewayPosts} = await import("./gateway");
    rememberUpdatedFoodPost({...older, title: "修改后的旧投稿"});
    await initFoodGateway();
    expect(getGatewayPosts()?.map((item) => item.id)).toEqual(["newer", "older"]);
    expect(getGatewayPosts()?.[1].title).toBe("修改后的旧投稿");
    rememberUpdatedFoodPost({...older, title: "再次修改的旧投稿"});
    expect(getGatewayPosts()?.map((item) => item.id)).toEqual(["newer", "older"]);
    expect(getGatewayPosts()?.[1].title).toBe("再次修改的旧投稿");
  });

  it("loads the board with the posts request alone, without waiting on venue reads", async () => {
    const fetch = vi.fn().mockImplementation(async (input: RequestInfo | URL) =>
      String(input) === "/api/v1/food/posts"
        ? new Response(JSON.stringify({ posts: [post("old-post", "旧投稿")], request_id: "req_posts" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        : new Response(JSON.stringify({ campus: "minglun", venues: [], request_id: "req_venues" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
    );
    vi.stubGlobal("fetch", fetch);

    const { loadFoodPosts } = await import("./gateway");
    await expect(loadFoodPosts()).resolves.toMatchObject({ error: null });

    expect(fetch.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/food/posts"]);
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

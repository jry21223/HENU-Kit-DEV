import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FoodPostCreateInput } from "./submit";

const CREATE_INPUT: FoodPostCreateInput = {
  venue_name: "西门小馆",
  campus: "minglun",
  tier: "hang",
  review_text: "味道稳，分量足，赶课保底。",
  price_reference: "人均 ¥18",
  hours_reference: "11:00–21:00",
  dishes: [
    { name: "羊肉烩面", price: "¥12", reason: "汤浓面筋道" },
    { name: "糖醋里脊", price: "", reason: "" },
  ],
  images: [{ content_type: "image/jpeg", data: "QUJDRA==" }],
};

const IDEMPOTENCY_KEY = "portal-food-post:11111111-1111-4111-8111-111111111111";

function createdPost() {
  return {
    id: "post-1",
    campus: "minglun",
    title: CREATE_INPUT.venue_name,
    excerpt: CREATE_INPUT.review_text,
    blocks: [],
    author: "小河同学",
    likes: 0,
    stars: 0,
    tags: ["夯"],
    shop: { name: CREATE_INPUT.venue_name, lat: 0, lng: 0 },
    time: "2030-01-01T00:00:00Z",
    hidden: false,
    images: [],
  };
}

describe("createFoodPost", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("POSTs the create input with credentials, JSON content type and the Idempotency-Key header", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          post: createdPost(),
          request_id: "req_food_create",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createFoodPost } = await import("./submit");
    const response = await createFoodPost(CREATE_INPUT, IDEMPOTENCY_KEY);

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, init] = fetch.mock.calls[0];
    expect(url).toBe("/api/v1/food/posts");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(init.headers["Content-Type"]).toBe("application/json");
    expect(init.headers["Idempotency-Key"]).toBe(IDEMPOTENCY_KEY);
    expect(JSON.parse(init.body)).toEqual(CREATE_INPUT);
    expect(response.post.id).toBe("post-1");
    expect(response.request_id).toBe("req_food_create");
  });

  it("maps the 429 daily-cap error code to the Chinese cap message", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            code: "DAILY_POST_CAP_REACHED",
            message: "daily submission cap reached",
          },
          request_id: "req_food_cap",
        }),
        { status: 429, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const {
      createFoodPost,
      foodPostDailyCapMessage,
      isFoodPostDailyCapError,
    } = await import("./submit");

    const error = await createFoodPost(CREATE_INPUT, IDEMPOTENCY_KEY).catch(
      (cause: unknown) => cause
    );

    expect(isFoodPostDailyCapError(error)).toBe(true);
    expect(foodPostDailyCapMessage()).toBe("今天已经投满 3 条，明天再来吧");
  });

  it("rejects an out-of-range Idempotency-Key before any network call", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const { createFoodPost } = await import("./submit");
    const { PortalApiError } = await import("../api/client");

    const error = await createFoodPost(CREATE_INPUT, "short").catch(
      (cause: unknown) => cause
    );
    expect(error).toBeInstanceOf(PortalApiError);
    expect(error).toMatchObject({
      code: "PORTAL_INVALID_FOOD_POST_IDEMPOTENCY_KEY",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("passes through other HTTP failures as PortalHttpError without cap mapping", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "food_unavailable",
          request_id: "req_food_down",
        }),
        { status: 503, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createFoodPost, isFoodPostDailyCapError } = await import(
      "./submit"
    );
    const { PortalHttpError } = await import("../api/client");

    const error = await createFoodPost(CREATE_INPUT, IDEMPOTENCY_KEY).catch(
      (cause: unknown) => cause
    );

    expect(error).toBeInstanceOf(PortalHttpError);
    expect(isFoodPostDailyCapError(error)).toBe(false);
  });
});

describe("foodPostImageFromFile", () => {
  class FakeFileReader {
    result: string | null = null;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    readAsDataURL(file: File) {
      queueMicrotask(() => {
        this.result = `data:${file.type};base64,QUJDRA==`;
        this.onload?.();
      });
    }
  }

  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("converts an image File to a prefix-free base64 payload", async () => {
    vi.stubGlobal("FileReader", FakeFileReader);

    const { foodPostImageFromFile } = await import("./submit");
    const file = new File(["dish"], "dish.jpg", { type: "image/jpeg" });

    await expect(foodPostImageFromFile(file)).resolves.toEqual({
      content_type: "image/jpeg",
      data: "QUJDRA==",
    });
  });

  it("rejects a file above 2MiB without reading it", async () => {
    const { foodPostImageFromFile } = await import("./submit");
    const big = new File([new ArrayBuffer(2 * 1024 * 1024 + 1)], "big.jpg", {
      type: "image/jpeg",
    });

    await expect(foodPostImageFromFile(big)).rejects.toThrow(
      "图片大小不能超过 2MB"
    );
  });

  it("rejects a non-whitelisted image type", async () => {
    const { foodPostImageFromFile } = await import("./submit");
    const gif = new File(["gif"], "animated.gif", { type: "image/gif" });

    await expect(foodPostImageFromFile(gif)).rejects.toThrow(
      "仅支持 JPG / PNG / WebP 图片"
    );
  });
});

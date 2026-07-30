import { describe, expect, it } from "vitest";
import type { FoodPost } from "@/lib/api/types";
import { buildFoodVenueDetail } from "./detail";

const BASE_POST: FoodPost = {
  id: "food-1",
  campus: "minglun",
  title: "鼓楼夜市",
  excerpt: "第一次体验开封夜市的代表性入口。",
  blocks: [
    { type: "p", text: "选择多、烟火气足，人均 ¥25–50。" },
    { type: "h2", text: "推荐菜品" },
    {
      type: "list",
      items: ["灌汤包：开封代表性面点", "杏仁茶：适合搭配小吃"],
    },
    { type: "img", src: "https://example.com/environment.jpg" },
  ],
  author: "学生编辑部",
  likes: 90,
  stars: 20,
  tags: ["夜市", "夯"],
  shop: {
    name: "鼓楼夜市",
    lat: 34.7972,
    lng: 114.3073,
  },
  time: "2026-07-16",
  hidden: false,
  images: ["https://example.com/cover.jpg"],
};

describe("buildFoodVenueDetail", () => {
  it("presents only facts already carried by the production FoodPost contract", () => {
    const detail = buildFoodVenueDetail(BASE_POST);

    expect(detail.tier?.label).toBe("夯");
    expect(detail.location).toBe("鼓楼夜市");
    expect(detail.coordinates).toBe("34.7972, 114.3073");
    expect(detail.priceReference).toBe("人均 ¥25–50");
    expect(detail.hoursReference).toBeNull();
    expect(detail.source).toEqual({
      author: "学生编辑部",
      publishedAt: "2026-07-16",
    });
    expect(detail.reasons).toEqual([BASE_POST.excerpt]);
    expect(detail.dishes[0]).toEqual({
      name: "灌汤包",
      note: "开封代表性面点",
    });
    expect(detail.gallery).toEqual([
      "https://example.com/cover.jpg",
      "https://example.com/environment.jpg",
    ]);
    expect(detail.mapUrl).toContain(
      encodeURIComponent(BASE_POST.shop.name)
    );
  });

  it("does not reinterpret an arbitrary editorial list as recommended dishes", () => {
    const detail = buildFoodVenueDetail({
      ...BASE_POST,
      blocks: [
        {
          type: "list",
          items: ["必拿：宽粉", "慎拿：冻豆腐"],
        },
      ],
      tags: ["待核验"],
    });

    expect(detail.tier).toBeNull();
    expect(detail.dishes).toEqual([]);
  });

  it("keeps dish semantics when gallery images sit between the heading and list", () => {
    const detail = buildFoodVenueDetail({
      ...BASE_POST,
      blocks: [
        { type: "h2", text: "点什么" },
        { type: "img", src: "https://example.com/menu.jpg" },
        {
          type: "list",
          items: ["招牌烩面：汤底浓郁", "糖醋里脊：酸甜口"],
        },
      ],
    });

    expect(detail.dishes).toEqual([
      { name: "招牌烩面", note: "汤底浓郁" },
      { name: "糖醋里脊", note: "酸甜口" },
    ]);
  });
});

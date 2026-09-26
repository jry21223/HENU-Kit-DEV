import { expect, type Locator, type Page } from "@playwright/test";

/**
 * 可读性检查（#536）共用的页面和网关 mock：tests/color-contrast.spec.ts 查文字对比度，
 * tests/typography.spec.ts 查字号与中文字距，两处打开同一组页面、看同样的内容。
 *
 * 网关按有内容的回应 mock，列表、筛选、卡片和档位标签都渲染出来。
 */

/** 加载失败的投稿照片：列表里换成回退图块，图块上的小字也要检查。 */
const BROKEN_PHOTO = "/readability-broken-photo.jpg";

const FOOD_POSTS = [
  { id: "hang-1", campus: "minglun", tags: ["夜市", "夯"], shop: { name: "鼓楼夜市" }, images: [BROKEN_PHOTO] },
  { id: "top-1", campus: "jinming", tags: ["夜市", "顶级"], shop: { name: "西司夜市" } },
  { id: "elite-1", campus: "minglun", tags: ["老字号", "人上人"], shop: { name: "第一楼" } },
  { id: "npc-1", campus: "longzihu", tags: ["校内", "NPC"], shop: { name: "龙子湖食堂" } },
  { id: "bad-1", campus: "longzihu", tags: ["待复核", "拉完了"], shop: { name: "待复核餐饮圈" } },
].map((post) => ({
  title: post.shop.name,
  excerpt: "学生视角的一句评价。",
  blocks: [{ type: "p", text: "正文。" }],
  author: "学生编辑部",
  likes: 10,
  stars: 5,
  time: "07-16",
  hidden: false,
  images: [],
  ...post,
}));

const CAMPUS_ITEMS = [
  {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  },
  {
    id: "campus-bookcase", type: "sell", category: "flea", title: "九成新书架", desc: "宿舍搬家出",
    price: 20, seller: "同学乙", credit: 0, dealsDone: 0, wants: 0, place: "金明校区", status: "open", time: "2026-09-21",
  },
];

const LIBRARY_MATERIALS = [
  {
    id: "library-limits", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
  {
    id: "library-final", type: "exam", subject: "高等数学",
    title: "高数期末真题", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 30, downloadAvailable: true, fileSize: 8192,
  },
];

/** 未登录；美食、互助、资料库给出有内容的回应，其余接口不可用。 */
export async function mockGatewayWithContent(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_readability_unavailable" },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({ json: { posts: FOOD_POSTS, request_id: "req_readability_food" } })
  );
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_readability_campus" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_readability_categories" } })
  );
  await page.route(`**${BROKEN_PHOTO}`, (route) => route.fulfill({ status: 404 }));
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      json: {
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 42,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_readability_library",
      },
    })
  );
}

export async function waitForHydration(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

/** 打开首页，等到美食榜和资料库模块的数据都渲染出来。 */
export async function gotoHomeWithContent(page: Page) {
  await page.goto("/");
  await waitForHydration(page);
  await expect(page.locator("[data-rank-row]")).toHaveCount(5);
  await expect(page.locator("section").getByRole("heading", { name: "笔记总结" })).toBeVisible();
}

/** 桌面与手机两种宽度。 */
export const VIEWPORTS = [
  { label: "1440", width: 1440, height: 900 },
  { label: "390", width: 390, height: 844 },
] as const;

/** 五个子站首页和登录页，以及各自内容落定后才出现的元素。 */
export const SUB_SITES: Array<{ route: string; ready: (page: Page) => Locator }> = [
  { route: "/library", ready: (page) => page.getByRole("link", { name: /极限复习笔记/ }) },
  // 默认构建里题库目录关闭，正文落在空状态。生产构建开着目录：题库卡片和加载失败提示
  // 由 quizcraft-catalog.spec.ts 在目录开启的 dev server 上检查（test:e2e:quizcraft-catalog）。
  { route: "/practice", ready: (page) => page.getByText("暂无题库") },
  // 等到第一条的照片加载失败、换成回退图块：图块的小字压在缩略图框的 4% 墨色底上。
  { route: "/food", ready: (page) => page.getByText("T-01 / 暂无图片") },
  { route: "/campus", ready: (page) => page.getByRole("heading", { name: "代取快递到南门" }) },
  // 等到落定的未登录介绍页，而不是先出现的加载块（它也带 data-career-state）。
  { route: "/career", ready: (page) => page.locator("[data-career-state='anonymous']") },
  { route: "/account/login", ready: (page) => page.getByRole("button", { name: "发送验证码" }) },
];

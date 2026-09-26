import { expect, test, type Page } from "@playwright/test";
import { expectTouchTargets } from "./support/touch-targets";

/**
 * 触控目标不小于 44×44px（DESIGN_SYSTEM §11、§13；#543）。在 390px 手机上，下列页面里
 * 每个可见的 a[href]、button、input、select 都要达标；例外规则见 support/touch-targets.ts。
 */

/** 登录与注册前的同意告知（LegalConsent）：两个协议链接在句子中间。 */
const CONSENT_LINKS = ["<a> 《用户协议》", "<a> 《隐私政策》"];

async function waitForHydration(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

const FOOD_POSTS = [
  { id: "hang-1", campus: "minglun", tags: ["夜市", "夯"], shop: { name: "鼓楼夜市" } },
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
];

/** 未登录，接口一律不可用：页面落在各自的出错状态。 */
async function mockUnavailableGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
        request_id: "req_targets_unavailable",
      }),
    })
  );
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
  );
}

/** 美食、互助、资料库给出有内容的回应，让列表和筛选里的控件都渲染出来；其余接口不可用。 */
async function mockGatewayWithContent(page: Page) {
  await mockUnavailableGateway(page);
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({ json: { posts: FOOD_POSTS, request_id: "req_targets_food" } })
  );
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_targets_campus" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_targets_categories" } })
  );
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      json: {
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 12,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_targets_library",
      },
    })
  );
}

test.use({ viewport: { width: 390, height: 844 }, contextOptions: { reducedMotion: "reduce" } });

test.describe("有内容时", () => {
  test.beforeEach(async ({ page }) => {
    await mockGatewayWithContent(page);
  });

  test("首页：汉堡按钮、03 美食榜单行和其余控件都不小于 44×44", async ({ page }) => {
    await page.goto("/");
    await waitForHydration(page);
    await expect(page.locator("[data-rank-row]")).toHaveCount(5);
    await expectTouchTargets(page, "首页");

    // 菜单面板打开后的每一行同样要达标。
    await page.getByRole("button", { name: "打开菜单" }).click();
    await expect(page.getByRole("button", { name: "关闭菜单" })).toBeVisible();
    await expectTouchTargets(page, "首页（菜单展开）");
  });

  for (const { route, ready } of [
    { route: "/library", ready: (page: Page) => page.getByRole("link", { name: /极限复习笔记/ }) },
    // 默认构建里题库目录关闭，正文落在空状态；目录开启时的卡片由 quizcraft-catalog.spec.ts 检查。
    { route: "/practice", ready: (page: Page) => page.getByText("暂无题库") },
    { route: "/food", ready: (page: Page) => page.getByRole("link", { name: /鼓楼夜市/ }).first() },
    { route: "/campus", ready: (page: Page) => page.getByRole("heading", { name: "代取快递到南门" }) },
    // 等到落定的未登录介绍页，而不是先出现的加载块（它也带 data-career-state）。
    { route: "/career", ready: (page: Page) => page.locator("[data-career-state='anonymous']") },
  ] as const) {
    test(`${route}：子站页头、筛选和正文控件都不小于 44×44`, async ({ page }) => {
      await page.goto(route);
      await waitForHydration(page);
      await expect(ready(page)).toBeVisible();
      await expectTouchTargets(page, route);
    });
  }
});

test("首页榜单加载失败时，重新加载按钮不小于 44×44", async ({ page }) => {
  await mockUnavailableGateway(page);
  await page.goto("/");
  await waitForHydration(page);
  await expect(page.getByText("榜单暂时加载不出来，请稍后刷新试试。")).toBeVisible();
  await expectTouchTargets(page, "首页（榜单出错）");
});

test("已登录时，子站页头的账户入口不小于 44×44", async ({ page }) => {
  await mockGatewayWithContent(page);
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    })
  );
  await page.goto("/library");
  await waitForHydration(page);
  await expect(page.locator('header a[href="/account"]:visible')).toBeVisible();
  await expectTouchTargets(page, "/library（已登录）");
});

test("终身会员的 /career：扫描历史入口和岗位链接不小于 44×44", async ({ page }) => {
  const userID = "11111111-1111-4111-8111-111111111111";
  const search = {
    id: "66666666-6666-4666-8666-666666666666",
    status: "completed",
    user_id: userID,
    has_email: false,
    created_at: "2026-09-20T08:00:00Z",
    result: {
      source_count: 1,
      job_count: 1,
      matched_count: 1,
      summary: "扫描已完成，找到 1 个相关岗位。",
      sources: [{ key: "getwork.henu", status: "success", found: 1 }],
      jobs: [
        {
          source_key: "getwork.henu",
          company: "河南大学",
          title: "后端开发实习生",
          location: "郑州",
          url: "https://example.com/jobs/backend-intern",
          match_score: 90,
          match_reasons: ["目标岗位：后端开发"],
        },
      ],
    },
  };
  await mockUnavailableGateway(page);
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      json: { user_id: userID, display_name: "小河同学", expires_at: "2030-01-01T00:00:00Z" },
    })
  );
  await page.route("**/api/v1/account/membership", (route) =>
    route.fulfill({ json: { data: { plan: "lifetime", lifetime: true }, request_id: "req_targets_membership" } })
  );
  await page.route("**/api/v1/career/profile", (route) =>
    route.fulfill({
      json: {
        profile: { user_id: userID, target_roles: "后端开发", updated_at: "2026-09-20T00:00:00Z" },
        request_id: "req_targets_profile",
      },
    })
  );
  await page.route("**/api/v1/career/searches", (route) =>
    route.fulfill({ json: { searches: [search], request_id: "req_targets_searches" } })
  );
  await page.route(`**/api/v1/career/searches/${search.id}`, (route) =>
    route.fulfill({ json: { search, request_id: "req_targets_search" } })
  );

  await page.goto("/career");
  await waitForHydration(page);
  await expect(page.locator("[data-career-scan-status='completed']")).toBeVisible();
  await expect(page.getByRole("link", { name: "查看官方岗位 →" })).toBeVisible();
  await expectTouchTargets(page, "/career（终身会员，扫描完成）");
});

// 桌面导航从 md（768px）起出现，平板上同样靠手指点。
for (const width of [768, 1024]) {
  test(`${width}px 平板的首页页头：桌面导航链接和账户入口不小于 44×44`, async ({ page }) => {
    await mockGatewayWithContent(page);
    await page.setViewportSize({ width, height: 1024 });
    await page.goto("/");
    await waitForHydration(page);
    await expect(page.locator("header").getByRole("link", { name: /资料库/ })).toBeVisible();
    await expectTouchTargets(page, `首页页头（${width}px）`, { within: "header" });
  });
}

test.describe("登录页", () => {
  test.beforeEach(async ({ page }) => {
    await mockUnavailableGateway(page);
  });

  test("登录注册切换、登录方式、发送验证码和找回入口都不小于 44×44", async ({ page }) => {
    await page.goto("/account/login");
    await waitForHydration(page);
    await expect(page.getByRole("button", { name: "发送验证码" })).toBeVisible();
    await expectTouchTargets(page, "验证码登录", { inSentence: CONSENT_LINKS });

    await page.getByRole("button", { name: "密码登录" }).click();
    await expect(page.getByLabel("密码 / PASSWORD")).toBeVisible();
    await expectTouchTargets(page, "密码登录", { inSentence: CONSENT_LINKS });

    await page.getByRole("button", { name: "注册", exact: true }).click();
    await expect(page.getByLabel("确认密码 / CONFIRM")).toBeVisible();
    await expectTouchTargets(page, "注册", { inSentence: CONSENT_LINKS });
  });

  test("登录链接失效时，重新开始和返回首页都不小于 44×44", async ({ page }) => {
    await page.goto("/account/login?continuation_error=expired");
    await waitForHydration(page);
    await expect(page.getByRole("heading", { name: "登录链接已过期或不可继续" })).toBeVisible();
    await expectTouchTargets(page, "登录链接失效");
  });
});

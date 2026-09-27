import { expect, test, type Page } from "@playwright/test";

/**
 * 模块数据按路由按需加载（#546）：根布局不再在每个页面预取资料库、美食、互助、求职
 * 四个模块。记录页面发出的 /api/v1/* 请求：登录页不请求任何模块数据；首页对资料全量
 * 最多请求一次（加载成功或失败都一样）；未登录访问任何页面都不请求求职数据。
 * 资料详情的“相关资料”在进入详情时才读全量目录；从列表进入时复用列表刚读到的目录。
 * 互助详情接口失败时，仍能回退到列表刚读到的那条单子。
 */

const MODULE_DATA = /^\/api\/v1\/(library|food|campus|career)\//;
const CAREER = /^\/api\/v1\/career\//;
const MATERIALS = "/api/v1/library/materials";

const NOTE = {
  id: "11111111-1111-4111-8111-111111111111", type: "note", subject: "高等数学",
  title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
  price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
};
const EXAM = {
  ...NOTE,
  id: "22222222-2222-4222-8222-222222222222", type: "exam", title: "高数期末真题",
};

const CATALOG = {
  materials: [NOTE, EXAM],
  statistics: {
    releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
    materialCount: 2,
    downloadStarts: 12,
    countingSince: "2026-08-11T00:00:00Z",
    asOf: "2026-08-11T01:00:00Z",
  },
  request_id: "req_requests_catalog",
};

/** 未登录，其余接口一律不可用；只统计请求，不关心页面拿到了什么。 */
async function mockGuestGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
        request_id: "req_requests_unavailable",
      }),
    })
  );
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
  );
}

/** 记录页面发出的每一个 /api/v1/* 请求路径（不含查询串）。 */
function recordApiRequests(page: Page): string[] {
  const paths: string[] = [];
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (pathname.startsWith("/api/v1/")) paths.push(pathname);
  });
  return paths;
}

/** 等水合完成、页面发起的请求都落地：挂载后才发的请求也要计入。 */
async function settle(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
  await page.waitForLoadState("networkidle");
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGuestGateway(page);
});

test("/account/login requests no module data", async ({ page }) => {
  const requests = recordApiRequests(page);
  await page.goto("/account/login");
  await settle(page);

  expect(requests.filter((path) => MODULE_DATA.test(path))).toEqual([]);
});

test("home reads the full catalog once when it loads", async ({ page }) => {
  await page.route("**/api/v1/library/materials", (route) => route.fulfill({ json: CATALOG }));
  const requests = recordApiRequests(page);
  await page.goto("/");
  const section = page.locator("section").filter({ has: page.getByRole("heading", { name: "资料库", level: 2 }) });
  await expect(section.getByRole("heading", { name: "笔记总结" })).toBeVisible();
  await settle(page);

  expect(requests.filter((path) => path === MATERIALS)).toHaveLength(1);
});

test("home reads the full catalog once when it fails", async ({ page }) => {
  const requests = recordApiRequests(page);
  await page.goto("/");
  const section = page.locator("section").filter({ has: page.getByRole("heading", { name: "资料库", level: 2 }) });
  await expect(section.getByRole("alert")).toBeVisible();
  await settle(page);

  expect(requests.filter((path) => path === MATERIALS)).toHaveLength(1);
});

test.describe("library detail related materials", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/library/materials", (route) => route.fulfill({ json: CATALOG }));
    await page.route(`**/api/v1/library/materials/${NOTE.id}`, (route) =>
      route.fulfill({ json: { material: NOTE, request_id: "req_requests_detail" } })
    );
  });

  const related = (page: Page) =>
    page.locator("section").filter({ hasText: "同科目或同类型的资料" }).getByRole("link", { name: /高数期末真题/ });

  test("load the catalog once on entry", async ({ page }) => {
    const requests = recordApiRequests(page);
    await page.goto(`/library/item/${NOTE.id}`);
    await expect(page.getByRole("heading", { name: NOTE.title, level: 1 })).toBeVisible();
    await expect(related(page)).toBeVisible();
    await settle(page);

    expect(requests.filter((path) => path === MATERIALS)).toHaveLength(1);
  });

  test("reuse the catalog the list just read", async ({ page }) => {
    const requests = recordApiRequests(page);
    await page.goto("/library");
    await settle(page);
    await page.getByRole("link", { name: /极限复习笔记/ }).click();
    await expect(page.getByRole("heading", { name: NOTE.title, level: 1 })).toBeVisible();
    await expect(related(page)).toBeVisible();
    await settle(page);

    expect(requests.filter((path) => path === MATERIALS)).toHaveLength(1);
  });
});

test("campus detail falls back to the item the list just read", async ({ page }) => {
  const item = {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  };
  await page.route("**/api/v1/campus/items", (route) => route.fulfill({ json: { items: [item], request_id: "req_requests_campus" } }));
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_requests_categories" } })
  );
  await page.goto("/campus");
  await settle(page);
  await page.getByRole("link", { name: /代取快递到南门/ }).click();

  await expect(page.getByRole("heading", { name: item.title, level: 1 })).toBeVisible();
});

// /account/profile 是求职之外唯一读求职资料的页面：未登录时账户控制台先跳去登录，不渲染它。
for (const path of [
  "/",
  "/library",
  "/food",
  "/campus",
  "/practice",
  "/career",
  "/career/history",
  "/account/profile",
  "/account/login",
]) {
  test(`guest visit to ${path} never requests career data`, async ({ page }) => {
    const requests = recordApiRequests(page);
    await page.goto(path);
    await settle(page);

    expect(requests.filter((request) => CAREER.test(request))).toEqual([]);
  });
}

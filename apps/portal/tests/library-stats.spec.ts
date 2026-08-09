import { expect, test, type Page, type Route } from "@playwright/test";

type Material = ReturnType<typeof material>;

function material(id: string, title: string, downloads: number) {
  return {
    id,
    type: "note" as const,
    subject: "高等数学",
    title,
    author: "HENU Kit",
    intro: `${title}简介`,
    toc: [],
    pages: [],
    price: 0,
    previewPages: 0,
    rating: 4.8,
    downloads,
    favs: 0,
  };
}

function counter(page: Page, label: string) {
  return page.getByText(label, { exact: true }).locator("..").locator("[data-counter-value]");
}

async function fulfillMaterials(route: Route, materials: Material[]) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ request_id: "req_library_stats", materials }),
  });
}

async function fulfillUnavailable(route: Route) {
  await route.fulfill({
    status: 503,
    contentType: "application/json",
    body: JSON.stringify({ error: "library unavailable" }),
  });
}

async function routeEmptyCourses(page: Page) {
  await page.route("**/api/v1/library/courses*", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ request_id: "req_library_courses", courses: [] }),
    });
  });
}

test.describe("Portal Library collection statistics", () => {
  test("a delayed collection updates both cards and unfiltered statistics after every refresh", async ({ page }) => {
    let releaseInitial: (() => void) | undefined;
    const initialGate = new Promise<void>((resolve) => {
      releaseInitial = resolve;
    });
    let materials = [
      material("material-a", "资料甲", 1_500),
      material("material-b", "资料乙", 2_500),
    ];
    await page.route("**/api/v1/library/materials*", async (route) => {
      await initialGate;
      await fulfillMaterials(route, materials);
    });

    await page.goto("/library", { waitUntil: "domcontentloaded" });
    await expect(counter(page, "收录资料")).toContainText("加载中");
    await expect(counter(page, "累计下载")).toContainText("加载中");

    releaseInitial?.();
    await expect(page.getByText("资料甲", { exact: true })).toBeVisible();
    await expect(page.getByText("资料乙", { exact: true })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("2");
    await expect(counter(page, "累计下载")).toContainText("4,000");

    await page.getByPlaceholder(/搜索/).fill("资料甲");
    await expect(page.getByText("资料乙", { exact: true })).toBeHidden();
    await expect(counter(page, "收录资料")).toContainText("2");
    await expect(counter(page, "累计下载")).toContainText("4,000");

    materials = [material("material-c", "资料丙", 7)];
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByText("资料丙", { exact: true })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("1");
    await expect(counter(page, "累计下载")).toContainText("7");
  });

  test("a successful empty collection shows real zeroes instead of loading or failure", async ({ page }) => {
    await page.route("**/api/v1/library/materials*", (route) => fulfillMaterials(route, []));

    await page.goto("/library", { waitUntil: "domcontentloaded" });
    await expect(page.getByText(/EMPTY/)).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("0");
    await expect(counter(page, "累计下载")).toContainText("0");
    await expect(counter(page, "收录资料")).not.toContainText("加载中");
    await expect(counter(page, "收录资料")).not.toContainText("未知");
  });

  test("a failed request keeps statistics unknown and a successful retry updates cards and totals together", async ({ page }) => {
    let failing = true;
    await page.route("**/api/v1/library/materials*", async (route) => {
      if (failing) {
        await fulfillUnavailable(route);
        return;
      }
      await fulfillMaterials(route, [material("material-retry", "重试资料", 321)]);
    });
    await routeEmptyCourses(page);

    await page.goto("/library", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[role="alert"]').filter({ hasText: "ERROR" })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("未知");
    await expect(counter(page, "累计下载")).toContainText("未知");

    failing = false;
    await page.getByRole("button", { name: "重试" }).click();
    await expect(page.getByText("重试资料", { exact: true })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("1");
    await expect(counter(page, "累计下载")).toContainText("321");
  });

  test("a failed current request never promotes a warm collection cache to confirmed statistics", async ({ page }) => {
    let phase: "warm" | "failure" = "warm";
    let warmMaterialResponses = 0;
    await page.route("**/api/v1/library/materials*", async (route) => {
      if (phase === "failure") {
        await fulfillUnavailable(route);
        return;
      }
      await fulfillMaterials(route, [material("material-warm", "预热资料", 88)]);
      warmMaterialResponses += 1;
    });
    await routeEmptyCourses(page);

    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect.poll(() => warmMaterialResponses).toBeGreaterThanOrEqual(2);
    await expect(page.getByText("1 FILES INDEXED", { exact: true })).toBeVisible();

    phase = "failure";
    await page.locator('a[href="/library"]').filter({ hasText: "进入模块" }).click();
    await expect(page).toHaveURL(/\/library$/);
    await expect(page.locator('[role="alert"]').filter({ hasText: "ERROR" })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("未知");
    await expect(counter(page, "累计下载")).toContainText("未知");
    await expect(page.getByText("预热资料", { exact: true })).toHaveCount(0);
  });

  test("reduced motion on a 390px viewport reaches the same final statistics", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.route("**/api/v1/library/materials*", (route) =>
      fulfillMaterials(route, [
        material("material-mobile-a", "移动资料甲", 1_500),
        material("material-mobile-b", "移动资料乙", 2_500),
      ]),
    );

    await page.goto("/library", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("移动资料甲", { exact: true })).toBeVisible();
    await expect(counter(page, "收录资料")).toContainText("2");
    await expect(counter(page, "累计下载")).toContainText("4,000");
    expect(await page.locator("html").evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    )).toBeTruthy();
  });
});

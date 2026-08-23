import { expect, test, type Page } from "@playwright/test";

const MATERIALS = [
  {
    id: "11111111-1111-4111-8111-111111111111", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
  {
    id: "22222222-2222-4222-8222-222222222222", type: "exam", subject: "计算机网络",
    title: "期末试卷", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 34, downloadAvailable: true, fileSize: 8192,
  },
];

function catalog(materials = MATERIALS, downloadStarts = 99) {
  return {
    materials,
    statistics: {
      releaseId: materials.length ? "0123456789abcdef0123456789abcdef01234567-0123456789abcdef" : null,
      materialCount: materials.length,
      downloadStarts,
      countingSince: "2026-08-11T00:00:00Z",
      asOf: "2026-08-11T01:00:00Z",
    },
    request_id: "req_library_browser",
  };
}

function counter(page: Page, label: string) {
  return page.getByText(label, { exact: true }).locator("..").locator('span[aria-hidden="true"]');
}

test("owner statistics stay unknown until the complete catalog loads and ignore filters", async ({ page }) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v1/library/materials", async (route) => {
    await gate;
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog()) });
  });
  await page.goto("/library", { waitUntil: "domcontentloaded" });
  await expect(counter(page, "收录资料")).toHaveText("—");
  await expect(counter(page, "累计下载")).toHaveText("—");
  release();
  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("99");
  await page.getByPlaceholder("搜索：真题 / 高数 / 课件").fill("计算机网络");
  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toHaveCount(0);
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("99");
});

test("empty success is zero while a failed request stays unknown and retry recovers", async ({ page }) => {
  let shouldFail = true;
  await page.route("**/api/v1/library/materials", async (route) => {
    if (shouldFail) {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "LIBRARY_TEMPORARILY_UNAVAILABLE", message: "资料库暂时无法加载，请稍后重试。" }) });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog([], 0)) });
  });
  await page.goto("/library", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("alert").filter({ hasText: "ERROR / 数据源不可用" })).toContainText("资料库暂时无法加载，请稍后重试。");
  await expect(counter(page, "收录资料")).toHaveText("—");
  await expect(counter(page, "累计下载")).toHaveText("—");
  shouldFail = false;
  await page.getByRole("button", { name: "重试" }).click();
  await expect(page.getByText("资料库当前暂无公开资料 / EMPTY", { exact: true })).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("0");
  await expect(counter(page, "累计下载")).toHaveText("0");
  await page.getByPlaceholder("搜索：真题 / 高数 / 课件").fill("高等数学");
  await expect(page.getByText("无匹配资料 / EMPTY", { exact: true })).toBeVisible();
});

test("reduced motion reaches the same owner totals", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route("**/api/v1/library/materials", (route) => route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog()) }));
  await page.goto("/library");
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("99");
});

test("returning after one download refreshes the owner ledger aggregate", async ({ page, context }) => {
  let downloadStarts = 99;
  await page.route("**/api/v1/library/materials", (route) => route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog(MATERIALS, downloadStarts)) }));
  await page.route("**/api/v1/library/materials/11111111-1111-4111-8111-111111111111", (route) => route.fulfill({ contentType: "application/json", body: JSON.stringify({ material: MATERIALS[0], request_id: "req_detail" }) }));
  await context.route("**/api/v1/library/materials/11111111-1111-4111-8111-111111111111/download", async (route) => {
    downloadStarts += 1;
    await route.fulfill({
      status: 200,
      headers: { "Content-Type": "application/pdf", "Content-Disposition": 'attachment; filename="material.pdf"' },
      body: "object bytes",
    });
  });
  await page.goto("/library");
  await expect(counter(page, "累计下载")).toHaveText("99");
  await page.getByRole("link", { name: /极限复习笔记/ }).click();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("link", { name: /下载资料/ }).click();
  await downloadPromise;
  await page.goBack({ waitUntil: "domcontentloaded" });
  await expect(counter(page, "累计下载")).toHaveText("100");
});

test("390px catalog has no horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/v1/library/materials", (route) => route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog()) }));
  await page.goto("/library");
  await expect(counter(page, "收录资料")).toHaveText("2");
  const sizes = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
  expect(sizes.scroll).toBeLessThanOrEqual(sizes.client);
});

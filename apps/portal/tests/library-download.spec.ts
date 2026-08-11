import { expect, test } from "@playwright/test";

const MATERIAL = {
  id: "mat-01",
  type: "note",
  subject: "软件工程",
  title: "软件工程复习讲义",
  author: "HENU Kit",
  intro: "公开免费资料",
  toc: ["第一章"],
  pages: [],
  price: 0,
  previewPages: 0,
  rating: 4.9,
  downloads: 7,
  favs: 0,
  fileSize: 24,
  downloadAvailable: true,
};

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} Library download uses the owner route and recovers after failure`, async ({ page, context }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/library/courses", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify({ courses: [], request_id: "req_courses" }) })
    );
    await page.route("**/api/v1/library/materials", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify({ materials: [MATERIAL], request_id: "req_materials" }) })
    );
    await page.route("**/api/v1/library/materials/mat-01", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify({ material: MATERIAL, request_id: "req_material" }) })
    );

    let attempts = 0;
    await context.route("**/api/v1/library/materials/mat-01/download", async (route) => {
      attempts += 1;
      expect(route.request().method()).toBe("GET");
      if (attempts === 1) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "DOWNLOAD_TEMPORARILY_UNAVAILABLE", message: "暂时无法生成下载链接，请稍后重试。" }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "application/pdf",
          "Content-Disposition": 'attachment; filename="material.pdf"',
          "Cache-Control": "no-store",
          "Referrer-Policy": "no-referrer",
          "X-Content-Type-Options": "nosniff",
        },
        body: "original object bytes",
      });
    });

    await page.goto("/library/item/mat-01", { waitUntil: "domcontentloaded" });
    const downloadLink = page.getByRole("link", { name: /下载资料/ });
    await expect(downloadLink).toBeVisible();
    await expect(downloadLink).toHaveAttribute("href", "/api/v1/library/materials/mat-01/download");
    const failureWindowPromise = page.waitForEvent("popup");
    await downloadLink.click();
    const failureWindow = await failureWindowPromise;
    await expect(failureWindow.getByText("暂时无法生成下载链接，请稍后重试。")).toBeVisible();
    await failureWindow.close();

    const downloadEvent = page.waitForEvent("download");
    await downloadLink.click();
    await downloadEvent;
    expect(attempts).toBe(2);
    await expect(page.locator("body")).not.toContainText("oss-cn-beijing");

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} owner detail failure never uses cached catalog and retry recovers`, async ({ page }) => {
    await page.setViewportSize(viewport);
    let catalogAttempts = 0;
    await page.route("**/api/v1/library/courses", (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify({ courses: [], request_id: "req_courses" }) })
    );
    await page.route("**/api/v1/library/materials", async (route) => {
      catalogAttempts += 1;
      if (catalogAttempts === 1) {
        await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "LIBRARY_TEMPORARILY_UNAVAILABLE", message: "资料库暂时无法加载，请稍后重试。" }) });
        return;
      }
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ materials: [MATERIAL], statistics: { releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef", materialCount: 1, downloadStarts: 7, countingSince: "2026-08-11T00:00:00Z", asOf: "2026-08-11T01:00:00Z" }, request_id: "req_materials" }) });
    });
    let detailUnavailable = true;
    await page.route("**/api/v1/library/materials/mat-01", async (route) => {
      if (detailUnavailable) {
        await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "LIBRARY_TEMPORARILY_UNAVAILABLE", message: "资料详情暂时无法加载，请稍后重试。" }) });
        return;
      }
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ material: MATERIAL, request_id: "req_material" }) });
    });

    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect.poll(() => catalogAttempts).toBeGreaterThanOrEqual(2);
    await page.goto("/library/item/mat-01", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[role="alert"]').filter({ hasText: "ERROR / 数据源不可用" })).toContainText("资料详情暂时无法加载，请稍后重试。");
    await expect(page.getByText("404 / NOT FOUND")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: MATERIAL.title })).toHaveCount(0);

    detailUnavailable = false;
    await page.getByRole("button", { name: "重试" }).click();
    await expect(page.getByRole("heading", { name: MATERIAL.title })).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("unavailable slides do not promise or expose an original-file download", async ({ page }) => {
  const unavailableSlides = { ...MATERIAL, type: "slides", price: 50, downloadAvailable: false, slides: [] };
  await page.route("**/api/v1/library/materials/mat-01", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify({ material: unavailableSlides, request_id: "req_material" }) })
  );

  await page.goto("/library/slides/mat-01", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("原文件暂时无法下载，请返回详情查看其他内容。")).toBeVisible();
  await expect(page.getByText("可直接下载原文件查看完整内容。")).toHaveCount(0);
  await expect(page.locator('a[href="/api/v1/library/materials/mat-01/download"]')).toHaveCount(0);
});

test("slides owner failure stays retryable and never masquerades as a 404", async ({ page }) => {
  const slides = { ...MATERIAL, type: "slides", price: 0, downloadAvailable: false, slides: [] };
  let unavailable = true;
  await page.route("**/api/v1/library/materials/mat-01", (route) =>
    unavailable
      ? route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ message: "资料详情暂时无法加载，请稍后重试。" }) })
      : route.fulfill({ contentType: "application/json", body: JSON.stringify({ material: slides, request_id: "req_material" }) })
  );

  await page.goto("/library/slides/mat-01", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[role="alert"]').filter({ hasText: "资料详情暂时无法加载" })).toBeVisible();
  await expect(page.getByText("404 / NOT FOUND")).toHaveCount(0);
  unavailable = false;
  await page.getByRole("button", { name: "重试" }).click();
  await expect(page.getByText("原文件暂时无法下载，请返回详情查看其他内容。")).toBeVisible();
});

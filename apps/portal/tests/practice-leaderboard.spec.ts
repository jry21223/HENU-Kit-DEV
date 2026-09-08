import { expect, test } from "@playwright/test";

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} practice leaderboard stays dark before #166`, async ({ page }) => {
    await page.setViewportSize(viewport);
    let rankingRequests = 0;
    page.on("request", (request) => {
      if (/\/api\/v1\/(?:rankings(?:\/|$)|banks\/[^/]+\/rankings(?:\/|$))/.test(request.url())) rankingRequests += 1;
    });

    await page.goto("/practice/leaderboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "排行榜", exact: true })).toBeVisible();
    await expect(page.getByText(/排行榜数据暂未开放/)).toBeVisible();
    await expect(page.locator("body")).not.toContainText("QuizCraft");
    await expect(page.locator("nav").getByRole("link", { name: /排行榜/ })).toHaveCount(0);
    expect(rankingRequests).toBe(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

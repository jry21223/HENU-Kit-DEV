import { expect, test } from "@playwright/test";

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} practice leaderboard renders real Gateway facts after #166`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const periods: string[] = [];
    await page.route("**/api/v1/rankings/overall?period=*", async (route) => {
      const period = new URL(route.request().url()).searchParams.get("period") ?? "weekly";
      periods.push(period);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          request_id: `req_${period}`,
          data: {
            scope: "overall",
            period,
            metric: "correct_answer_count",
            entries: period === "weekly" ? [
              { rank: 1, nickname: "匿名学习者", system_avatar: "scholar-blue", correct_answer_count: 12 },
              { rank: 1, nickname: "匿名学习者", system_avatar: "coder-green", correct_answer_count: 12 },
            ] : [],
          },
        }),
      });
    });

    await page.goto("/practice/leaderboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "排行榜", exact: true })).toBeVisible();
    await expect(page.getByText("匿名学习者", { exact: true })).toHaveCount(2);
    await expect(page.getByText("12 题", { exact: true })).toHaveCount(2);
    await expect(page.locator("nav").getByRole("link", { name: /排行榜/ })).toBeVisible();
    await page.getByRole("button", { name: "总榜" }).click();
    await expect(page.getByText(/当前周期尚无公开排行事实/)).toBeVisible();
    expect(periods).toEqual(["weekly", "lifetime"]);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

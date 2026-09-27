import { expect, test, type Page } from "@playwright/test";
import { expectTouchTargets } from "./support/touch-targets";

async function mockRankings(page: Page, periods: string[] = []) {
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
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} practice leaderboard renders real Gateway facts after #166`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const periods: string[] = [];
    await mockRankings(page, periods);

    await page.goto("/practice/leaderboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "排行榜", exact: true })).toBeVisible();
    await expect(page.getByText("匿名学习者", { exact: true })).toHaveCount(2);
    await expect(page.getByText("12 题", { exact: true })).toHaveCount(2);
    await expect(page.locator("nav").getByRole("link", { name: /排行榜/ })).toBeVisible();
    // The toggles form a named group, so screen readers announce 排行榜周期.
    await page.getByRole("group", { name: "排行榜周期" }).getByRole("button", { name: "总榜" }).click();
    await expect(page.getByText("还没有人上榜", { exact: true })).toBeVisible();
    await expect(page.getByTestId("practice-leaderboard").getByRole("link", { name: "去刷题", exact: true }))
      .toHaveAttribute("href", "/practice");
    expect(periods).toEqual(["weekly", "lifetime"]);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

// The period toggles render only with V2 reads on (the release build), so the
// 44×44 check for them runs here in the stats group (#543).
test("390px leaderboard period toggles are at least 44×44", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockRankings(page);
  await page.goto("/practice/leaderboard");
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
  await expect(page.getByText("12 题", { exact: true })).toHaveCount(2);
  await expect(page.getByRole("button", { name: "本周" })).toBeVisible();
  await expectTouchTargets(page, "/practice/leaderboard (V2 reads on)");
});

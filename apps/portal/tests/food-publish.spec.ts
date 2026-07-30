import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} recommendation page hands off to the persisted Student Food Desk`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await page.goto("/food/publish", { waitUntil: "domcontentloaded" });

    await expect(
      page.getByRole("heading", { name: "你吃到的好店，投到这里。" })
    ).toBeVisible();
    await expect(page.getByText("商家名称", { exact: true })).toBeVisible();
    await expect(page.getByText("最近到店时间", { exact: true })).toBeVisible();
    await expect(page.getByText("为什么推荐", { exact: true })).toBeVisible();
    await expect(page.getByText("推荐菜品", { exact: true })).toBeVisible();

    const link = page.getByRole("link", { name: "登录学生美食台投稿" });
    await expect(link).toHaveAttribute(
      "href",
      "https://henu-campus-guide.cocoa-brush-7952.chatgpt.site/#food-submit"
    );
    await expect(page.getByRole("button", { name: "发布" })).toHaveCount(0);

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

import { expect, test } from "@playwright/test";

/**
 * 不存在的地址落在品牌化的 404 页上（#534）：返回 404，中文说明、回首页与常用入口，
 * 照常带全站页脚（DESIGN_SYSTEM §16）。create-next-app 留下的模板文件不再对外提供。
 */

const DISCLAIMER = "学生自主运营 · 非河南大学官方项目";

// 子站下的未知路径也要验：兜底页替换掉子站布局，页脚得由它自己带上，而且只有一份。
for (const path of ["/this-page-does-not-exist", "/library/this-page-does-not-exist"]) {
  test(`${path} answers 404 with the branded not-found page`, async ({ page }) => {
    const response = await page.goto(path);
    expect(response?.status()).toBe(404);
    await expect(page).toHaveTitle("页面不存在 | HENU Kit");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("页面不存在");
    await expect(page.locator("main")).toContainText("这个地址没有对应的页面");
    await expect(page.locator("body")).not.toContainText("This page could not be found");

    await expect(page.getByRole("link", { name: "回首页", exact: true })).toHaveAttribute("href", "/");
    const entries = page.getByRole("navigation", { name: "常用入口" });
    await expect(entries.getByRole("link", { name: /资料库/ })).toHaveAttribute("href", "/library");
    await expect(entries.getByRole("link", { name: /智能刷题/ })).toHaveAttribute("href", "/practice");
    await expect(entries.getByRole("link", { name: /美食榜/ })).toHaveAttribute("href", "/food");

    const footer = page.getByRole("contentinfo");
    await expect(footer).toHaveCount(1);
    await expect(footer).toContainText(DISCLAIMER);
    await expect(footer.getByRole("link", { name: "隐私政策" })).toHaveAttribute("href", "/privacy");
    await expect(footer.getByRole("link", { name: "用户协议" })).toHaveAttribute("href", "/terms");
  });
}

test("the not-found page fits a 360px screen with full-size tap targets", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto("/this-page-does-not-exist", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("页面不存在");

  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);

  const links = page.locator("main a");
  await expect(links).toHaveCount(4);
  for (const link of await links.all()) {
    const box = await link.boundingBox();
    expect(box?.height, await link.innerText()).toBeGreaterThanOrEqual(44);
  }
});

test("create-next-app template files are no longer served", async ({ request }) => {
  for (const file of ["/file.svg", "/globe.svg", "/next.svg", "/vercel.svg", "/window.svg"]) {
    const response = await request.get(file);
    expect(response.status(), file).toBe(404);
  }
});

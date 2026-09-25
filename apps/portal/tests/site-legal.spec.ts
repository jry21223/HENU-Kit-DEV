import { expect, test, type Page } from "@playwright/test";

/**
 * 每个页面都要固定展示主体声明与协议入口（DESIGN_SYSTEM §16）：首页在最后一屏的大页脚里，
 * 子站、账户中心、登录页与协议页在各自的页脚里。登录与支付还要在操作前再次说明主体，
 * 并告知继续即同意《用户协议》和《隐私政策》。
 */

const DISCLAIMER = "学生自主运营 · 非河南大学官方项目";

async function expectLegalFooter(page: Page) {
  const footer = page.getByRole("contentinfo").last();
  await expect(footer).toContainText(DISCLAIMER);
  await expect(footer.getByRole("link", { name: "隐私政策" })).toHaveAttribute("href", "/privacy");
  await expect(footer.getByRole("link", { name: "用户协议" })).toHaveAttribute("href", "/terms");
}

for (const path of [
  "/",
  "/library",
  "/practice",
  "/food",
  "/campus",
  "/career",
  "/account",
  "/account/login",
  "/account/recover",
  "/privacy",
  "/terms",
]) {
  test(`${path} carries the disclaimer and the legal links in its footer`, async ({ page }) => {
    await page.goto(path, { waitUntil: "domcontentloaded" });
    await expectLegalFooter(page);
  });
}

for (const doc of [
  {
    path: "/privacy",
    heading: "隐私政策",
    sections: [/我们收集哪些信息/, /Cookie/, /委托处理与对外提供/, /你的权利/, /联系我们/],
  },
  {
    path: "/terms",
    heading: "用户协议",
    sections: [/账户/, /用户发布的内容/, /终身会员与支付/, /联系我们/],
  },
]) {
  test(`${doc.path} is a readable public document`, async ({ page }) => {
    const response = await page.goto(doc.path);
    expect(response?.status()).toBe(200);
    await expect(page).toHaveTitle(`${doc.heading} | HENU Kit`);
    await expect(page.locator("h1")).toHaveCount(1);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(doc.heading);
    for (const section of doc.sections) {
      await expect(page.getByRole("heading", { level: 2, name: section })).toBeVisible();
    }
    await expect(page.locator("main")).toContainText("最近更新");
    await expect(page.locator("main")).toContainText("非河南大学官方");
  });
}

test("legal documents and the footer fit a 360px screen", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 740 });
  for (const path of ["/privacy", "/terms", "/library"]) {
    await page.goto(path, { waitUntil: "domcontentloaded" });
    await expectLegalFooter(page);
    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll, path).toBeLessThanOrEqual(width.client + 2);
  }
});

test("registration states the operator and asks for agreement before an account is created", async ({ page }) => {
  await page.goto("/account/login", { waitUntil: "domcontentloaded" });
  // 水合前点击只是一次无效的点击：先等客户端外壳就绪（与 sub-site-back-navigation 同一个标记）。
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
  await page.getByRole("button", { name: "注册", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "注册" })).toBeVisible();

  const consent = page.locator("[data-account-consent]");
  await expect(consent).toContainText("已阅读并同意");
  await expect(consent).toContainText("非河南大学官方项目");
  await expect(consent.getByRole("link", { name: "《用户协议》" })).toHaveAttribute("href", "/terms");
  await expect(consent.getByRole("link", { name: "《隐私政策》" })).toHaveAttribute("href", "/privacy");

  // 展示名会被公开展示，收集时就要说清楚，而且它是必填项。
  const name = page.getByLabel(/展示名/);
  await expect(name).toHaveAttribute("aria-describedby", /reg-name-hint/);
  await expect(page.locator("#reg-name-hint")).toContainText("公开显示");
  await expect(name).not.toHaveAttribute("placeholder", /可选/);
});

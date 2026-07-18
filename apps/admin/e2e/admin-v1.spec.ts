import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByPlaceholder("admin@example.com").fill(process.env.ADMIN_E2E_EMAIL ?? "admin@example.com");
  await page.getByRole("button", { name: "发送验证码" }).click();
  await expect(page.getByText("开发环境验证码", { exact: false })).toBeVisible();
  await page.getByPlaceholder("123456").fill(process.env.ADMIN_E2E_CODE ?? "123456");
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

test("fixed six-domain dashboard exposes honest metrics and trend", async ({ page }) => {
  await login(page);
  await expect(page.locator("[data-domain-card]")).toHaveCount(6);
  for (const domain of ["users", "notice", "mail", "feedback", "food", "system"]) {
    await expect(page.locator(`[data-domain-card="${domain}"]`)).toBeVisible();
  }
  await expect(page.locator("canvas")).toBeVisible();
  await expect(page.getByText("数据时间", { exact: false }).first()).toBeVisible();
});

test("all connected domain operation pages remain reachable", async ({ page }) => {
  await login(page);
  for (const [path, heading] of [["/users", "用户管理"], ["/notices", "校园通知"], ["/mail", "邮件投递"], ["/feedback", "反馈中心"], ["/food", "美食榜单"], ["/system", "系统运行"]] as const) {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByText("数据加载失败", { exact: false })).toHaveCount(0);
  }
});

test("keyboard focus and mobile dashboard stay operable", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-390", "mobile accessibility check");
  await login(page);
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toBeVisible();
  await expect(page.locator('[data-domain-card="users"]')).toBeVisible();
  expect(await page.locator("body").evaluate((element) => element.scrollWidth)).toBeLessThanOrEqual(390);
});

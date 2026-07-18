import { expect, test } from "@playwright/test";

const moduleNames = ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"];

test("desktop overview exposes six modules, degradation states, and chart alternative", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "产品运行概览" })).toBeVisible();
  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  for (const name of moduleNames) await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  for (const state of ["empty", "partial", "stale", "unavailable", "denied"]) {
    await expect(page.locator(`[data-state="${state}"]`)).toHaveCount(1);
  }
  await page.getByRole("button", { name: "查看表格数据" }).click();
  await expect(page.getByRole("table", { name: "Portal 探针成功次数表格" })).toBeVisible();
  await expect(page.getByText("积分", { exact: true })).toHaveCount(0);
  await expect(page.getByText("会员", { exact: true })).toHaveCount(0);
});

test("390px overview keeps every module and mobile navigation usable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  await page.getByRole("button", { name: "打开产品导航" }).click();
  const navigation = page.getByRole("navigation", { name: "移动端产品模块" });
  await expect(navigation.getByRole("link")).toHaveCount(6);
  for (const name of moduleNames) await expect(navigation.getByRole("link", { name })).toBeVisible();

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

test("loading scenario marks all six modules busy without fake metrics", async ({ page }) => {
  await page.goto("/?scenario=loading");
  await expect(page.locator("section[aria-busy='true']")).toBeVisible();
  await expect(page.locator("[data-state='loading']")).toHaveCount(6);
  await expect(page.locator(".metric-tile")).toHaveCount(0);
});

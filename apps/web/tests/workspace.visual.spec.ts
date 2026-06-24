import { expect, test } from "@playwright/test";

test.use({ channel: "chrome" });

const workspaceUrl = process.env.WORKSPACE_URL ?? "http://127.0.0.1:3000/workspace";

test("workspace presents the book-desk visual system on desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(workspaceUrl, { waitUntil: "networkidle" });

  await expect(page.locator('[data-workspace-style="book-desk"]')).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "资料册工作台" })).toBeVisible();
  await expect(page.getByText("档案索引")).toBeVisible();

  const courseCards = page.locator('[data-workspace-card="course-pdf"]');
  await expect(courseCards).toHaveCount(3);

  const firstCard = courseCards.first();
  const beforeTransform = await firstCard.evaluate((element) => getComputedStyle(element).transform);
  await firstCard.hover();
  await page.waitForTimeout(260);
  const afterTransform = await firstCard.evaluate((element) => getComputedStyle(element).transform);

  expect(afterTransform).not.toBe(beforeTransform);
});

test("workspace keeps the book-desk layout usable on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto(workspaceUrl, { waitUntil: "networkidle" });

  await expect(page.locator('[data-workspace-style="book-desk"]')).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "资料册工作台" })).toBeVisible();
  await expect(page.locator("#workspace-mobile-search")).toBeVisible();
  await expect(page.locator('[data-workspace-card="course-pdf"]').first()).toBeVisible();
});

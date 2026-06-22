import { expect, test } from "@playwright/test";

test.use({ channel: "chrome" });

const homeUrl = process.env.HOME_URL ?? "http://127.0.0.1:3000/";

test("homepage renders product vision on desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();
  await expect(page.getByText("软件学院资料库").first()).toBeVisible();

  await page.evaluate(() => {
    const archive = document.querySelector<HTMLElement>('[aria-label="课程资料档案册"]');

    if (!archive) {
      throw new Error("Archive book section was not found");
    }

    const stageTop = window.scrollY + archive.getBoundingClientRect().top;
    const scrollableDistance = Math.max(archive.offsetHeight - window.innerHeight, 1);
    window.scrollTo(0, stageTop + scrollableDistance * 0.72);
  });
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(page.getByText("数据结构").first()).toBeVisible();
});

test("homepage uses simplified archive on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();

  await page.mouse.wheel(0, 850);
  await expect(page.getByRole("heading", { name: "资料册已打开" })).toBeVisible();
  await expect(page.locator('[aria-label="课程资料入口"]').getByText("数据结构").first()).toBeVisible();
});

test("homepage keeps archive content available with reduced motion", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await page.locator('[aria-label="课程资料档案册"]').scrollIntoViewIfNeeded();
  const archiveBook = page.locator('[aria-label="课程资料档案册"]');

  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(archiveBook.getByRole("link", { name: /数据结构/ })).toBeVisible();
});

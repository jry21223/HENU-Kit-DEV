import { expect, test, type Page } from "@playwright/test";

test.use({ channel: "chrome" });

const homeUrl = process.env.HOME_URL ?? "http://127.0.0.1:3000/";

async function scrollArchiveTo(page: Page, progress: number) {
  await page.evaluate((nextProgress) => {
    const archive = document.querySelector<HTMLElement>('[aria-label="课程资料档案册"]');

    if (!archive) {
      throw new Error("Archive book section was not found");
    }

    const stageTop = window.scrollY + archive.getBoundingClientRect().top;
    const scrollableDistance = Math.max(archive.offsetHeight - window.innerHeight, 1);
    window.scrollTo(0, stageTop + scrollableDistance * nextProgress);
  }, progress);
}

async function archiveBookBox(page: Page) {
  return page.getByTestId("archive-book").evaluate((element) => {
    const rect = element.getBoundingClientRect();

    return {
      bottom: rect.bottom,
      centerX: rect.left + rect.width / 2,
      height: rect.height,
      left: rect.left,
      right: rect.right,
      top: rect.top,
      width: rect.width,
    };
  });
}

async function elementBox(page: Page, testId: string) {
  return page.getByTestId(testId).evaluate((element) => {
    const rect = element.getBoundingClientRect();

    return {
      bottom: rect.bottom,
      centerX: rect.left + rect.width / 2,
      height: rect.height,
      left: rect.left,
      right: rect.right,
      top: rect.top,
      width: rect.width,
    };
  });
}

test("homepage renders product vision on desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  const startBox = await archiveBookBox(page);
  const startCoverBox = await elementBox(page, "archive-cover");
  expect(startBox.width).toBeGreaterThan(1180);
  expect(startCoverBox.width / startCoverBox.height).toBeLessThan(0.86);
  expect(startCoverBox.centerX).toBeGreaterThan(1440 * 0.62);
  expect(startCoverBox.right).toBeGreaterThan(1440);
  expect(startCoverBox.bottom).toBeGreaterThan(1100);

  await expect(page.getByTestId("archive-copy-intro")).toBeVisible();
  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();
  await expect(page.getByRole("link", { name: /进入工作区/ })).toHaveAttribute("href", "/workspace");
  await expect(page.getByRole("link", { name: /浏览课程资料/ })).toHaveAttribute("href", "/courses");
  await expect(page.getByText("软件学院资料库").first()).toBeVisible();

  await scrollArchiveTo(page, 0.6);
  await expect(page.getByTestId("archive-copy-intro")).toBeHidden();
  await expect(page.getByTestId("archive-copy-open")).toBeHidden();
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeHidden();
  await expect(page.getByTestId("archive-cover")).toBeVisible();

  const closedCoverBox = await elementBox(page, "archive-cover");
  expect(closedCoverBox.centerX).toBeGreaterThan(1440 * 0.68);
  expect(closedCoverBox.centerX).toBeLessThan(1440 * 0.86);
  expect(closedCoverBox.width / closedCoverBox.height).toBeLessThan(0.86);

  await scrollArchiveTo(page, 0.8);
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(page.getByText("数据结构").first()).toBeVisible();
  await expect(page.getByTestId("archive-copy-closing")).toBeVisible();

  const openBox = await archiveBookBox(page);
  expect(openBox.width).toBeGreaterThan(1180);
  expect(openBox.width / openBox.height).toBeGreaterThan(1.25);
  expect(Math.abs(openBox.centerX - 720)).toBeLessThan(28);
  const seamBox = await elementBox(page, "archive-seam");
  expect(Math.abs(seamBox.centerX - openBox.centerX)).toBeLessThan(8);

  await scrollArchiveTo(page, 0.88);
  await expect(page.locator('[data-home-anim="archive-page"]').first()).toBeVisible();
  await expect(page.locator('[data-home-anim="archive-directory-line"]').first()).toHaveAttribute("tabindex", "-1");

  await scrollArchiveTo(page, 0.93);
  await expect(page.getByTestId("archive-copy-closing")).toBeVisible();
  await expect(page.getByTestId("archive-copy-closing").getByText("资料合上以后，还会继续生长")).toBeVisible();
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeHidden();

  const closedAgainBox = await archiveBookBox(page);
  expect(Math.abs(closedAgainBox.centerX - openBox.centerX)).toBeLessThan(120);
});

test("homepage exposes precision animation markers", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await expect(page.locator('[data-home-anim="archive-book"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-cover"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-directory-scan"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-directory-line"]')).toHaveCount(6);
  await expect(page.locator('[data-home-anim="course-book"]')).toHaveCount(6);
  await expect(page.locator('[data-home-anim="mobile-course-book"]')).toHaveCount(6);
  await expect(page.locator('[data-home-anim="community-note"]')).toHaveCount(4);
  await expect(page.locator('[data-home-anim="practice-card"]')).toHaveCount(4);
  await expect(page.locator('[data-home-anim="membership-stamp"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="sales-note"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="guarantee-seal"]')).toHaveCount(4);
});

test("homepage preserves precision animation markers in-view rotations", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  const firstNote = page.locator('[data-home-anim="community-note"]').first();
  await firstNote.scrollIntoViewIfNeeded();
  await expect(firstNote).toBeVisible();
  await page.waitForTimeout(700);

  const transformSkew = await firstNote.evaluate((element) => {
    const matrix = new DOMMatrixReadOnly(getComputedStyle(element).transform);

    return Math.abs(matrix.b);
  });

  expect(transformSkew).toBeGreaterThan(0.03);
});

test("homepage keeps precision animation markers unprepared with reduced motion", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.addInitScript(() => {
    type PrepWrite = { propertyName: string; value: string };
    const pageWindow = window as Window & { __homeInViewPrepWrites?: PrepWrite[] };
    const originalSetProperty = CSSStyleDeclaration.prototype.setProperty;

    pageWindow.__homeInViewPrepWrites = [];
    CSSStyleDeclaration.prototype.setProperty = function setProperty(propertyName, value, priority) {
      const stringValue = String(value ?? "");

      if (
        (propertyName === "opacity" && stringValue === "0") ||
        (propertyName === "translate" && stringValue === "0 18px") ||
        (propertyName === "will-change" && stringValue === "opacity, translate")
      ) {
        pageWindow.__homeInViewPrepWrites?.push({ propertyName, value: stringValue });
      }

      return originalSetProperty.call(this, propertyName, value, priority);
    };
  });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  const firstNote = page.locator('[data-home-anim="community-note"]').first();
  await firstNote.scrollIntoViewIfNeeded();
  await expect(firstNote).toBeVisible();

  const reducedMotionState = await firstNote.evaluate((element) => {
    const pageWindow = window as Window & {
      __homeInViewPrepWrites?: Array<{ propertyName: string; value: string }>;
    };

    return {
      opacity: element.style.getPropertyValue("opacity"),
      prepWrites: pageWindow.__homeInViewPrepWrites ?? [],
      translate: element.style.getPropertyValue("translate"),
      willChange: element.style.getPropertyValue("will-change"),
    };
  });

  expect(reducedMotionState.prepWrites).toEqual([]);
  expect(reducedMotionState.opacity).toBe("");
  expect(reducedMotionState.translate).toBe("");
  expect(reducedMotionState.willChange).toBe("");
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

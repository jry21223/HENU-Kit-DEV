import { expect, test } from "@playwright/test";
import type { Locator, Page } from "@playwright/test";

const feed = {
  data: {
    notices: [
      {
        id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        title: "开学安排",
        body: "请按时返校，并留意学院后续安排。",
        source: { name: "学校办公室", url: "https://example.edu/notices/1" },
        created_at: "2026-08-01T00:00:00Z",
      },
    ],
  },
  request_id: "req_notice_feed",
};

const longTitle = "T".repeat(200);
const longSourceName = "S".repeat(120);
const longBody = "B".repeat(100000);
const longTokenFeed = {
  data: {
    notices: [
      {
        id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
        title: longTitle,
        body: longBody,
        source: { name: longSourceName, url: "https://example.edu/notices/long-token" },
        created_at: "2026-08-01T00:00:00Z",
      },
    ],
  },
  request_id: "req_notice_long_token",
};

async function expectNoPageOverflow(page: Page) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(metrics.scrollWidth, `page overflow at ${metrics.clientWidth}px`).toBeLessThanOrEqual(
    metrics.clientWidth + 1,
  );
}

async function expectTextContrastAgainstPaper(locator: Locator) {
  const ratio = await locator.evaluate((element) => {
    const context = document.createElement("canvas").getContext("2d");
    if (!context) throw new Error("Canvas 2D context is unavailable");
    context.canvas.width = 1;
    context.canvas.height = 1;
    const paper = getComputedStyle(document.body).backgroundColor;
    const renderedPixel = (color?: string) => {
      context.clearRect(0, 0, 1, 1);
      context.fillStyle = paper;
      context.fillRect(0, 0, 1, 1);
      if (color) {
        context.fillStyle = color;
        context.fillRect(0, 0, 1, 1);
      }
      return Array.from(context.getImageData(0, 0, 1, 1).data.slice(0, 3));
    };
    const luminance = (rgb: number[]) => {
      const [red, green, blue] = rgb.map((channel) => {
        const value = channel / 255;
        return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
    };
    const background = luminance(renderedPixel());
    const foreground = luminance(renderedPixel(getComputedStyle(element).color));
    return (Math.max(background, foreground) + 0.05) / (Math.min(background, foreground) + 0.05);
  });
  expect(ratio).toBeGreaterThanOrEqual(4.5);
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 900 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`notice feed renders an Owner-backed public item at ${viewport.name}`, async ({ page }) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.route("**/api/v1/notices", async (route) => {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(feed) });
    });

    await page.goto("/notice", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-testid="notice-feed-ready"]')).toBeVisible();
    const readyStatus = page.locator('[data-testid="notice-feed-ready-status"]');
    await expect(readyStatus).toHaveAttribute("role", "status");
    await expect(readyStatus).toHaveAttribute("aria-live", "polite");
    await expect(readyStatus).toHaveText("已加载 1 条通知");
    await expect(readyStatus).not.toContainText("请按时返校，并留意学院后续安排。");
    const source = page.getByRole("link", { name: "查看来源：学校办公室（新标签页打开）" });
    await source.hover();
    await expectTextContrastAgainstPaper(source);
    await expect(page.locator("footer")).toBeAttached();
    await expect(page.getByRole("navigation", { name: "页脚导航" })).toBeVisible();
    if (viewport.width >= 768) {
      await expect(page.getByRole("navigation", { name: "主导航" })).toBeVisible();
    }
    const eyebrow = page.locator("main > header > p").first();
    await expectTextContrastAgainstPaper(eyebrow);
    await expectTextContrastAgainstPaper(page.getByText("NOTICE", { exact: true }));
    const footerIndex = page.locator("footer").getByRole("link", { name: /01.*资料库/ });
    await expectTextContrastAgainstPaper(footerIndex.locator("span").first());
    await expect(page.getByText("这里展示面向全校学生的站内通知。来源链接可供核对，正文在本页展开查看。")).toHaveCSS("font-size", "16px");
    const footerDeclaration = page.getByText("学生自主运营 · 非河南大学官方项目");
    await expect(footerDeclaration).toHaveCSS("font-size", "16px");
    await expectTextContrastAgainstPaper(footerDeclaration);
    const footerCopyright = page.locator("footer p").filter({ hasText: "©" });
    await expect(footerCopyright).toHaveCSS("font-size", "16px");
    await expectTextContrastAgainstPaper(footerCopyright);
    await expect(page.getByRole("heading", { name: "开学安排" })).toBeVisible();
    await expectTextContrastAgainstPaper(page.getByText("N-01", { exact: true }));
    const intake = page.getByText("收录时间");
    await expect(intake).toBeVisible();
    await expect(intake).toHaveCSS("font-size", "14px");
    await expectTextContrastAgainstPaper(intake);
    const summary = page.locator("summary", { hasText: "查看详情" });
    if (viewport.width === 390) {
      await expect(summary).toHaveCSS("min-height", "44px");
      expect((await summary.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      const footerLinkBox = await footerIndex.boundingBox();
      expect(footerLinkBox?.width).toBeGreaterThanOrEqual(44);
      expect(footerLinkBox?.height).toBeGreaterThanOrEqual(44);
      await expect(footerIndex).toHaveCSS("min-width", "44px");
    }
    await summary.click();
    const body = page.getByText("请按时返校，并留意学院后续安排。");
    await expect(body).toBeVisible();
    await expect(body).toHaveCSS("font-size", "16px");
    await expect(source).toHaveAttribute("href", "https://example.edu/notices/1");
    await expect(source).toHaveAttribute("target", "_blank");
    await expect(source).toHaveAttribute("rel", "noreferrer");
    if (viewport.width >= 768) {
      const desktopNoticeLink = page.locator("header nav").getByRole("link", { name: /05.*通知/ });
      const box = await desktopNoticeLink.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
      await expectTextContrastAgainstPaper(desktopNoticeLink.locator("span").first());
      const desktopShortestLink = page.locator("header nav").getByRole("link", { name: /01.*资料库/ });
      await expect(desktopShortestLink).toHaveCSS("min-width", "44px");
      const homeLinkBox = await page.locator('header a[href="/"]').boundingBox();
      expect(homeLinkBox?.width).toBeGreaterThanOrEqual(44);
      expect(homeLinkBox?.height).toBeGreaterThanOrEqual(44);
      await expectTextContrastAgainstPaper(
        page.locator('header a[href="/"] > span').filter({ hasText: "KEEP IN TOUCH" }),
      );
      const signedOutAccountBox = await page.locator('header a[href="/account/login"]').boundingBox();
      expect(signedOutAccountBox?.width).toBeGreaterThanOrEqual(44);
      expect(signedOutAccountBox?.height).toBeGreaterThanOrEqual(44);
      const signedOutAccount = page.locator('header a[href="/account/login"]');
      await expectTextContrastAgainstPaper(signedOutAccount.locator("span").first());
      await signedOutAccount.hover();
      await expectTextContrastAgainstPaper(signedOutAccount);
    }
    await expectNoPageOverflow(page);
  });
}

test("notice header keeps a recovered account entry touchable", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.route("**/api/v1/notices", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(feed) });
  });
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    });
  });

  await page.goto("/notice", { waitUntil: "domcontentloaded" });
  const accountLink = page.locator('header a[href="/account"]');
  await expect(accountLink).toBeVisible();
  await accountLink.hover();
  await expectTextContrastAgainstPaper(accountLink.locator("span").first());
  const accountBox = await accountLink.boundingBox();
  expect(accountBox?.width).toBeGreaterThanOrEqual(44);
  expect(accountBox?.height).toBeGreaterThanOrEqual(44);
});

for (const viewport of [
  { name: "360px", width: 360, height: 800 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`notice feed wraps contract-max unbroken text at ${viewport.name}`, async ({ page }) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.route("**/api/v1/notices", async (route) => {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(longTokenFeed) });
    });

    await page.goto("/notice", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-testid="notice-feed-ready"] h2')).toHaveText(longTitle);
    await page.locator("summary", { hasText: "查看详情" }).click();
    await expect(page.getByText(longBody)).toBeVisible();
    await expectNoPageOverflow(page);
  });
}

test("notice loading status remains readable", async ({ page }) => {
  const pendingReleases = new Set<() => void>();
  let releaseObservedRequests = false;
  let routeCalls = 0;
  let inFlightRequests = 0;
  let observeFirstRequest!: () => void;
  const firstRequestObserved = new Promise<void>((resolve) => {
    observeFirstRequest = resolve;
  });
  await page.route("**/api/v1/notices", async (route) => {
    routeCalls += 1;
    inFlightRequests += 1;
    let releaseRequest!: () => void;
    const releaseGate = new Promise<void>((resolve) => {
      releaseRequest = () => {
        pendingReleases.delete(releaseRequest);
        resolve();
      };
      pendingReleases.add(releaseRequest);
      if (releaseObservedRequests) releaseRequest();
    });
    observeFirstRequest();
    try {
      await releaseGate;
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(feed) });
    } finally {
      inFlightRequests -= 1;
    }
  });

  await page.goto("/notice", { waitUntil: "domcontentloaded" });
  const loading = page.locator('[data-testid="notice-feed-loading"]');
  await expect(loading).toBeVisible();
  await expect(loading).toHaveAttribute("role", "status");
  await expect(loading).toHaveAttribute("aria-live", "polite");
  await expect(loading).toHaveCSS("font-size", "16px");
  await expectTextContrastAgainstPaper(loading);
  await firstRequestObserved;
  releaseObservedRequests = true;
  for (const releaseRequest of [...pendingReleases]) releaseRequest();
  await expect(page.locator('[data-testid="notice-feed-ready"]')).toBeVisible();
  await expect(page.locator('[data-testid="notice-feed-ready-status"]')).toHaveText("已加载 1 条通知");
  await expect.poll(() => pendingReleases.size).toBe(0);
  await expect.poll(() => inFlightRequests).toBe(0);
  expect(routeCalls).toBeGreaterThanOrEqual(1);
  expect(routeCalls).toBeLessThanOrEqual(2);
});

test("notice feed keeps signed-out and retryable-unavailable actions usable at 390px", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  let state: "empty" | "unavailable" | "valid" | "signed-out" | "denied" = "empty";
  await page.route("**/api/v1/notices", async (route) => {
    if (state === "empty") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: { notices: [] }, request_id: "req_empty" }) });
      return;
    }
    if (state === "unavailable") {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "notice_unavailable", request_id: "req_down" }) });
      return;
    }
    if (state === "signed-out") {
      await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "not authenticated", request_id: "req_expired" }) });
      return;
    }
    if (state === "denied") {
      await route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: "notice access denied", request_id: "req_denied" }) });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(feed) });
  });

  await page.goto("/notice", { waitUntil: "domcontentloaded" });
  const empty = page.locator('[data-testid="notice-feed-empty"]');
  await expect(empty).toBeVisible();
  await expect(empty).toHaveAttribute("role", "status");
  await expect(empty).toHaveAttribute("aria-live", "polite");
  await expect(empty).toHaveCSS("font-size", "16px");
  await expectTextContrastAgainstPaper(empty);
  await expect(page.getByText("开学安排")).toHaveCount(0);

  const menu = page.getByRole("button", { name: "打开菜单" });
  await expect(menu).toHaveCSS("min-height", "44px");
  await expect(menu).toHaveCSS("min-width", "44px");
  await menu.click();
  await expect(page.getByRole("button", { name: "关闭菜单" })).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByRole("navigation", { name: "主导航" })).toBeVisible();
  const accountLabel = page.getByText("账户", { exact: true });
  await expect(accountLabel).toBeVisible();
  await expectTextContrastAgainstPaper(accountLabel);
  const mobileNoticeLink = page.getByRole("link", { name: /05.*通知/ });
  await expect(mobileNoticeLink).toBeVisible();
  await expectTextContrastAgainstPaper(mobileNoticeLink.locator("span").first());

  state = "unavailable";
  await page.reload({ waitUntil: "domcontentloaded" });
  const unavailable = page.locator('[data-testid="notice-feed-error"]');
  await expect(unavailable).toBeVisible();
  await expect(unavailable.locator('[role="alert"]')).toHaveAttribute("aria-live", "assertive");
  await expect(page.getByText("请稍后重试。")).toHaveCSS("font-size", "16px");
  const retry = page.getByRole("button", { name: "重新加载" });
  await expect(retry).toHaveCSS("min-height", "44px");
  await expectNoPageOverflow(page);
  await expect(page.getByText("开学安排")).toHaveCount(0);

  state = "valid";
  await retry.click();
  await expect(page.locator('[data-testid="notice-feed-ready"]')).toBeVisible();
  await expect(page.locator('[data-testid="notice-feed-ready-status"]')).toHaveText("已加载 1 条通知");

  state = "denied";
  await page.reload({ waitUntil: "domcontentloaded" });
  const denied = page.locator('[data-testid="notice-feed-denied"]');
  await expect(denied).toBeVisible();
  await expect(denied).toHaveAttribute("role", "status");
  await expect(denied).toHaveAttribute("aria-live", "polite");
  await expect(page.getByText("你暂时没有查看通知的权限，请联系管理员。")).toBeVisible();

  state = "signed-out";
  await page.reload({ waitUntil: "domcontentloaded" });
  const signedOut = page.locator('[data-testid="notice-feed-signed-out"]');
  await expect(signedOut).toBeVisible();
  await expect(signedOut.locator('[role="status"]')).toHaveAttribute("aria-live", "polite");
  const signIn = page.getByRole("button", { name: "去登录" });
  await expect(signIn).toBeVisible();
  await expect(signIn).toHaveCSS("min-height", "44px");
  await expectNoPageOverflow(page);
});

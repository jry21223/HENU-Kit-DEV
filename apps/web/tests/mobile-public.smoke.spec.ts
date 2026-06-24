import { expect, test, type Page } from "@playwright/test";

test.use({
  channel: "chrome",
  hasTouch: true,
  isMobile: true,
  viewport: { width: 390, height: 844 },
});

const webBaseURL = cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? process.env.HOME_URL ?? "http://127.0.0.1:3000");

const publicRoutes = [
  { path: "/", requiredSelector: 'a[href="/me/downloads"]', shell: false },
  { path: "/login", requiredSelector: 'input[type="email"]', shell: true },
  { path: "/courses", requiredSelector: 'a[href="/login"]', shell: true },
  { path: "/packages", requiredSelector: 'a[href="/courses"]', shell: true },
  { path: "/wiki", requiredSelector: 'a[href="/wiki"]', shell: true },
  { path: "/blog", requiredSelector: 'a[href="/blog"]', shell: true },
  { path: "/forum", requiredSelector: 'a[href="/forum"]', shell: true },
  { path: "/moments", requiredSelector: 'a[href="/moments"]', shell: true },
  { path: "/search", requiredSelector: 'form[action="/search"] input[name="q"]', shell: true },
  { path: "/me", requiredSelector: 'a[href="/me/wrong-questions"]', shell: true },
  { path: "/me/relations", requiredSelector: 'a[href="/login"]', shell: true },
] as const;

test.describe("mobile public pages", () => {
  for (const route of publicRoutes) {
    test(`${route.path} fits a 390px mobile viewport`, async ({ page }) => {
      const response = await page.goto(joinURL(webBaseURL, route.path), { waitUntil: "networkidle" });
      expect(response?.status() ?? 0).toBeLessThan(500);

      await expect(page.locator("main").first()).toBeVisible();
      await expect(page.locator(route.requiredSelector).first()).toBeVisible();

      if (route.shell) {
        await expect(page.locator("header").first()).toBeVisible();
        await expect(page.locator("footer").first()).toBeVisible();
      }

      await expectNoDocumentHorizontalOverflow(page);
      await expectUsableMobileControls(page);
    });
  }
});

async function expectNoDocumentHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => {
    const viewportWidth = document.documentElement.clientWidth;
    const documentWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
    const offenders = Array.from(document.body.querySelectorAll<HTMLElement>("*"))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return {
          className: element.className.toString().slice(0, 120),
          left: Math.round(rect.left),
          right: Math.round(rect.right),
          tag: element.tagName.toLowerCase(),
          text: (element.textContent ?? "").replace(/\s+/g, " ").trim().slice(0, 80),
          width: Math.round(rect.width),
          visible:
            rect.width > 0 &&
            rect.height > 0 &&
            style.display !== "none" &&
            style.visibility !== "hidden" &&
            Number(style.opacity) !== 0,
        };
      })
      .filter((item) => item.visible && (item.left < -2 || item.right > viewportWidth + 2))
      .slice(0, 8);

    return { documentWidth, offenders, viewportWidth };
  });

  expect(
    overflow.documentWidth,
    `Document width ${overflow.documentWidth}px exceeds viewport ${overflow.viewportWidth}px. Offenders: ${JSON.stringify(
      overflow.offenders,
    )}`,
  ).toBeLessThanOrEqual(overflow.viewportWidth + 2);
}

async function expectUsableMobileControls(page: Page) {
  const tinyTargets = await page.evaluate(() => {
    const selector = "header a, header button, form button, input, select, textarea";
    return Array.from(document.querySelectorAll<HTMLElement>(selector))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return {
          height: Math.round(rect.height),
          selector: element.tagName.toLowerCase(),
          text: (element.textContent ?? element.getAttribute("aria-label") ?? "").replace(/\s+/g, " ").trim().slice(0, 80),
          visible:
            rect.width > 0 &&
            rect.height > 0 &&
            style.display !== "none" &&
            style.visibility !== "hidden" &&
            Number(style.opacity) !== 0,
          width: Math.round(rect.width),
        };
      })
      .filter((target) => target.visible && (target.width < 32 || target.height < 32))
      .slice(0, 8);
  });

  expect(tinyTargets, `Visible mobile controls below 32px target size: ${JSON.stringify(tinyTargets)}`).toEqual([]);
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

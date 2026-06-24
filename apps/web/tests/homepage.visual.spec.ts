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
    const previousRootScrollBehavior = document.documentElement.style.scrollBehavior;
    const previousBodyScrollBehavior = document.body.style.scrollBehavior;

    document.documentElement.style.scrollBehavior = "auto";
    document.body.style.scrollBehavior = "auto";
    window.scrollTo(0, stageTop + scrollableDistance * nextProgress);
    document.documentElement.style.scrollBehavior = previousRootScrollBehavior;
    document.body.style.scrollBehavior = previousBodyScrollBehavior;
  }, progress);

  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );
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

async function archiveVisualState(page: Page) {
  return page.evaluate(() => {
    const cover = document.querySelector<HTMLElement>('[data-testid="archive-cover"]');
    const coverShadow = document.querySelector<HTMLElement>('[data-home-anim="archive-cover-shadow"]');
    const base = document.querySelector<HTMLElement>('[data-home-anim="archive-base"]');
    const closingCopy = document.querySelector<HTMLElement>('[data-testid="archive-copy-closing"]');
    const inside = document.querySelector<HTMLElement>('[data-home-anim="archive-inside"]');
    const panels = Array.from(
      document.querySelectorAll<HTMLElement>(
        '[data-home-anim="archive-left-panel"], [data-home-anim="archive-right-panel"]',
      ),
    );
    const pages = Array.from(document.querySelectorAll<HTMLElement>('[data-home-anim="archive-page"]'));
    const spineShadow = document.querySelector<HTMLElement>('[data-home-anim="archive-spine-shadow"]');

    if (!base || !cover || !coverShadow || !closingCopy || !inside || panels.length !== 2 || pages.length === 0 || !spineShadow) {
      throw new Error("Archive visual nodes were not found");
    }

    const coverRect = cover.getBoundingClientRect();
    const closingRect = closingCopy.getBoundingClientRect();
    const insideOpacity = Number(getComputedStyle(inside).opacity);
    const panelOpacities = panels.map((panel) => Number(getComputedStyle(panel).opacity));
    const pageOpacities = pages.map((archivePage) => {
      const panel = archivePage.closest<HTMLElement>(
        '[data-home-anim="archive-left-panel"], [data-home-anim="archive-right-panel"]',
      );

      return Number(getComputedStyle(archivePage).opacity) * insideOpacity * (panel ? Number(getComputedStyle(panel).opacity) : 1);
    });

    return {
      baseOpacity: Number(getComputedStyle(base).opacity),
      closingCopyOpacity: Number(getComputedStyle(closingCopy).opacity),
      closingCopyRight: closingRect.right,
      coverLeft: coverRect.left,
      coverOpacity: Number(getComputedStyle(cover).opacity),
      coverShadowOpacity: Number(getComputedStyle(coverShadow).opacity),
      insideOpacity,
      maxPageOpacity: Math.max(...pageOpacities),
      maxPanelOpacity: Math.max(...panelOpacities),
      minPageOpacity: Math.min(...pageOpacities),
      minPanelOpacity: Math.min(...panelOpacities),
      spineShadowOpacity: Number(getComputedStyle(spineShadow).opacity),
    };
  });
}

async function sectionBox(page: Page, labelledBy: string) {
  return page.locator(`section[aria-labelledby="${labelledBy}"]`).evaluate((element) => {
    const rect = element.getBoundingClientRect();

    return {
      bottom: rect.bottom,
      height: rect.height,
      top: rect.top,
      width: rect.width,
      windowHeight: window.innerHeight,
    };
  });
}

async function expectPostBookSectionMarkerInViewport(
  page: Page,
  labelledBy: string,
  marker: string,
  expectedCount: number,
  options: { animated?: boolean; minOpacity?: number } = {},
) {
  const { animated = true, minOpacity = 0.99 } = options;
  const section = page.locator(`section[aria-labelledby="${labelledBy}"]`);
  await expect(section).toHaveCount(1);
  await section.scrollIntoViewIfNeeded();
  await expect(section).toBeVisible();

  const visibleSectionBox = await sectionBox(page, labelledBy);
  expect(visibleSectionBox.width).toBeGreaterThan(320);
  expect(visibleSectionBox.height).toBeGreaterThan(80);
  expect(visibleSectionBox.top).toBeLessThan(visibleSectionBox.windowHeight);
  expect(visibleSectionBox.bottom).toBeGreaterThan(0);

  const locator = section.locator(`[data-home-anim="${marker}"]`);
  await expect(locator).toHaveCount(expectedCount);

  const target = locator.first();
  await target.scrollIntoViewIfNeeded();
  await expect(target).toBeVisible();

  const box = await target.evaluate((element) => {
    const rect = element.getBoundingClientRect();

    return {
      bottom: rect.bottom,
      height: rect.height,
      top: rect.top,
      width: rect.width,
      windowHeight: window.innerHeight,
    };
  });

  expect(box.width).toBeGreaterThan(20);
  expect(box.height).toBeGreaterThan(20);
  expect(box.top).toBeLessThan(box.windowHeight);
  expect(box.bottom).toBeGreaterThan(0);
  await expect(target).toHaveAttribute("data-home-anim", marker);

  if (animated) {
    await expect
      .poll(
        async () =>
          target.evaluate((element, expectedOpacity) => {
            const styles = getComputedStyle(element);
            const computedOpacity = Number(styles.opacity);
            const computedTranslate = styles.translate;
            const inlineOpacity = element.style.getPropertyValue("opacity");
            const inlineTranslate = element.style.getPropertyValue("translate");
            const inlineWillChange = element.style.getPropertyValue("will-change");

            return {
              inlineOpacity,
              inlineTranslate,
              inlineWillChange,
              opacityReady: computedOpacity > expectedOpacity,
              translateRestored: computedTranslate !== "0px 12px" && computedTranslate !== "0 12px",
            };
          }, minOpacity),
        { timeout: 7000 },
      )
      .toEqual({
        inlineOpacity: "",
        inlineTranslate: "",
        inlineWillChange: "",
        opacityReady: true,
        translateRestored: true,
      });
    return;
  }

  const nonAnimatedState = await target.evaluate((element) => {
    const styles = getComputedStyle(element);

    return {
      computedOpacity: Number(styles.opacity),
      computedTranslate: styles.translate,
      inlineOpacity: element.style.getPropertyValue("opacity"),
      inlineTranslate: element.style.getPropertyValue("translate"),
      inlineWillChange: element.style.getPropertyValue("will-change"),
    };
  });

  expect(nonAnimatedState.computedOpacity).toBeGreaterThan(minOpacity);
  expect(nonAnimatedState.computedTranslate).not.toBe("0px 12px");
  expect(nonAnimatedState.computedTranslate).not.toBe("0 12px");
  expect(nonAnimatedState.inlineOpacity).toBe("");
  expect(nonAnimatedState.inlineTranslate).toBe("");
  expect(nonAnimatedState.inlineWillChange).toBe("");
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
  await expect(page.locator('[data-home-anim="archive-open-copy"]')).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "打开你的期末复习资料册" })).toBeVisible();
  await expect(page.getByRole("link", { name: /进入工作区/ })).toHaveAttribute("href", "/workspace");
  await expect(page.getByRole("link", { name: /浏览课程资料/ })).toHaveAttribute("href", "/courses");
  await expect(page.getByText("软件学院资料库").first()).toBeVisible();

  await scrollArchiveTo(page, 0.66);
  await expect(page.getByTestId("archive-copy-intro")).toBeHidden();
  await expect(page.locator('[data-home-anim="archive-open-copy"]')).toHaveCount(0);
  await expect(page.locator('[data-home-anim="archive-page"]').first().getByText("资料目录")).toBeVisible();
  await expect(page.getByTestId("archive-cover")).toBeVisible();

  const openingCoverBox = await elementBox(page, "archive-cover");
  const liftingVisualState = await archiveVisualState(page);
  expect(liftingVisualState.coverShadowOpacity).toBeGreaterThan(0.12);
  expect(liftingVisualState.spineShadowOpacity).toBeGreaterThan(0.18);
  expect(liftingVisualState.baseOpacity).toBeLessThan(0.3);
  expect(liftingVisualState.insideOpacity).toBeGreaterThan(0.7);
  expect(liftingVisualState.minPanelOpacity).toBeGreaterThan(0.2);
  expect(liftingVisualState.maxPageOpacity).toBeGreaterThan(0.2);
  expect(openingCoverBox.centerX).toBeGreaterThan(1440 * 0.45);
  expect(openingCoverBox.centerX).toBeLessThan(1440 * 0.86);
  expect(openingCoverBox.width / openingCoverBox.height).toBeLessThan(0.86);

  await scrollArchiveTo(page, 0.8);
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  const lateTurningVisualState = await archiveVisualState(page);
  expect(lateTurningVisualState.insideOpacity).toBeGreaterThan(0.55);
  expect(lateTurningVisualState.minPageOpacity).toBeGreaterThan(0.55);
  expect(lateTurningVisualState.maxPageOpacity).toBeGreaterThan(0.55);
  const midTurnBookBox = await archiveBookBox(page);
  const midTurnCoverBox = await elementBox(page, "archive-cover");
  expect(midTurnCoverBox.width / midTurnBookBox.width).toBeLessThan(0.18);

  await scrollArchiveTo(page, 0.92);
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(page.getByText("数据结构").first()).toBeVisible();
  await expect(page.getByTestId("archive-copy-closing")).toBeHidden();

  const openVisualState = await archiveVisualState(page);
  expect(openVisualState.minPageOpacity).toBeGreaterThan(0.95);
  expect(openVisualState.coverOpacity).toBeLessThan(0.18);

  const openBox = await archiveBookBox(page);
  expect(openBox.width).toBeGreaterThan(1180);
  expect(openBox.width / openBox.height).toBeGreaterThan(1.25);
  expect(Math.abs(openBox.centerX - 720)).toBeLessThan(28);
  const seamBox = await elementBox(page, "archive-seam");
  expect(Math.abs(seamBox.centerX - openBox.centerX)).toBeLessThan(8);

  await scrollArchiveTo(page, 0.94);
  await expect(page.locator('[data-home-anim="archive-page"]').first()).toBeVisible();
  await expect(page.locator('[data-home-anim="archive-directory-line"]').first()).toHaveAttribute("tabindex", "-1");

  await scrollArchiveTo(page, 0.95);
  await expect(page.getByTestId("archive-copy-closing")).toBeHidden();
  const preClosingVisualState = await archiveVisualState(page);
  expect(preClosingVisualState.closingCopyOpacity).toBeLessThan(0.05);

  await scrollArchiveTo(page, 0.98);
  await expect(page.getByTestId("archive-copy-closing")).toBeVisible();
  await expect(page.getByTestId("archive-copy-closing").getByText("资料合上以后，还会继续生长")).toBeVisible();
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeHidden();

  const closingVisualState = await archiveVisualState(page);
  expect(closingVisualState.closingCopyOpacity).toBeGreaterThan(0.7);
  expect(closingVisualState.maxPageOpacity).toBeLessThan(0.12);
  expect(closingVisualState.coverOpacity).toBeGreaterThan(0.55);
  expect(closingVisualState.closingCopyRight).toBeLessThan(closingVisualState.coverLeft - 32);

  const closedAgainBox = await archiveBookBox(page);
  expect(Math.abs(closedAgainBox.centerX - openBox.centerX)).toBeLessThan(120);
});

test("homepage keeps archive narration clear of the open pages", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await scrollArchiveTo(page, 0.92);
  await expect(page.getByRole("heading", { name: "资料目录" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "课程入口" })).toBeVisible();
  await expect(page.getByTestId("archive-copy-intro")).toBeHidden();
  await expect(page.locator('[data-home-anim="archive-open-copy"]')).toHaveCount(0);
  await expect(page.getByTestId("archive-copy-closing")).toBeHidden();

  const openVisualState = await archiveVisualState(page);
  expect(openVisualState.minPageOpacity).toBeGreaterThan(0.95);
  expect(openVisualState.coverOpacity).toBeLessThan(0.18);
});

test("homepage binds archive pages to hinged book panels", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  const structure = await page.evaluate(() => {
    const leftPanel = document.querySelector<HTMLElement>('[data-home-anim="archive-left-panel"]');
    const rightPanel = document.querySelector<HTMLElement>('[data-home-anim="archive-right-panel"]');
    const pages = Array.from(document.querySelectorAll<HTMLElement>('[data-home-anim="archive-page"]'));

    const leftOriginX = leftPanel ? Number.parseFloat(getComputedStyle(leftPanel).transformOrigin) : Number.NaN;
    const rightOriginX = rightPanel ? Number.parseFloat(getComputedStyle(rightPanel).transformOrigin) : Number.NaN;

    return {
      hasLeftPanel: Boolean(leftPanel),
      hasRightPanel: Boolean(rightPanel),
      leftContainsDirectoryPage: Boolean(leftPanel && pages[0] && leftPanel.contains(pages[0])),
      leftOriginX,
      leftWidth: leftPanel?.offsetWidth ?? 0,
      pageCount: pages.length,
      rightContainsCoursePage: Boolean(rightPanel && pages[1] && rightPanel.contains(pages[1])),
      rightOriginX,
    };
  });

  expect(structure).toMatchObject({
    hasLeftPanel: true,
    hasRightPanel: true,
    leftContainsDirectoryPage: true,
    pageCount: 2,
    rightContainsCoursePage: true,
  });
  expect(structure.leftOriginX).toBeGreaterThan(structure.leftWidth - 4);
  expect(structure.rightOriginX).toBeLessThan(4);
});

test("homepage gives post-book cards tactile hover motion", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  for (const marker of ["community-note", "practice-card", "membership-ticket", "sales-note"]) {
    const target = page.locator(`[data-home-anim="${marker}"]`).first();
    await target.scrollIntoViewIfNeeded();
    await expect(target).toBeVisible();
    await page.waitForTimeout(700);

    const before = await target.evaluate((element) => getComputedStyle(element).transform);
    await target.hover();
    await page.waitForTimeout(280);
    const after = await target.evaluate((element) => getComputedStyle(element).transform);

    expect(after).not.toBe(before);
    expect(after).not.toBe("none");
  }
});

test("homepage reveals post-book product sections while preserving animation markers", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await scrollArchiveTo(page, 0.98);
  await expect(page.getByTestId("archive-copy-closing")).toBeVisible();

  await expectPostBookSectionMarkerInViewport(page, "community-title", "community-note", 4);
  await expectPostBookSectionMarkerInViewport(page, "practice-title", "practice-card", 4);
  await expectPostBookSectionMarkerInViewport(page, "membership-title", "membership-ticket", 1);
  await expectPostBookSectionMarkerInViewport(page, "membership-title", "membership-stamp", 1, {
    animated: false,
    minOpacity: 0.7,
  });
  await expectPostBookSectionMarkerInViewport(page, "sales-assistant-title", "sales-note", 1);
  await expectPostBookSectionMarkerInViewport(page, "guarantee-title", "guarantee-seal", 4);
});

test("homepage exposes precision animation markers", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto(homeUrl, { waitUntil: "networkidle" });

  await expect(page.locator('[data-home-anim="archive-book"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-cover"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-cover-shadow"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-spine-shadow"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-left-panel"]')).toHaveCount(1);
  await expect(page.locator('[data-home-anim="archive-right-panel"]')).toHaveCount(1);
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
        (propertyName === "translate" && stringValue === "0 12px") ||
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

test("homepage keeps JS-enabled SSR archive in animation initial layout", async ({ browser }) => {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1100 },
  });

  try {
    const page = await context.newPage();
    await page.route(/\.js(\?.*)?$/, (route) => route.abort());
    await page.goto(homeUrl, { waitUntil: "domcontentloaded" });

    const startBox = await archiveBookBox(page);
    expect(startBox.width).toBeGreaterThan(1180);

    const visualState = await page.evaluate(() => {
      const cover = document.querySelector<HTMLElement>('[data-home-anim="archive-cover"]');
      const inside = document.querySelector<HTMLElement>('[data-home-anim="archive-inside"]');
      const leftPanel = document.querySelector<HTMLElement>('[data-home-anim="archive-left-panel"]');
      const firstPage = document.querySelector<HTMLElement>('[data-home-anim="archive-page"]');

      if (!cover || !inside || !leftPanel || !firstPage) {
        throw new Error("Archive book elements were not found");
      }

      return {
        coverOpacity: Number(getComputedStyle(cover).opacity),
        insideOpacity: Number(getComputedStyle(inside).opacity),
        leftPanelOpacity: Number(getComputedStyle(leftPanel).opacity),
        pageOpacity: Number(getComputedStyle(firstPage).opacity),
      };
    });

    expect(visualState).toEqual({ coverOpacity: 1, insideOpacity: 0, leftPanelOpacity: 0, pageOpacity: 1 });
  } finally {
    await context.close();
  }
});

test("homepage keeps archive pages accessible without JavaScript and reduced motion", async ({ browser }) => {
  const context = await browser.newContext({
    javaScriptEnabled: false,
    reducedMotion: "reduce",
    viewport: { width: 1440, height: 1100 },
  });

  try {
    const page = await context.newPage();
    await page.goto(homeUrl, { waitUntil: "domcontentloaded" });

    const archivePages = page.locator('[data-home-anim="archive-page"]');
    await expect(archivePages).toHaveCount(2);

    const pageStates = await archivePages.evaluateAll((elements) =>
      elements.map((element) => {
        const styles = getComputedStyle(element);

        return {
          ariaHidden: element.getAttribute("aria-hidden"),
          computedVisibility: styles.visibility,
          inlineVisibility: (element as HTMLElement).style.visibility,
        };
      }),
    );

    expect(pageStates).toEqual([
      { ariaHidden: null, computedVisibility: "visible", inlineVisibility: "visible" },
      { ariaHidden: null, computedVisibility: "visible", inlineVisibility: "visible" },
    ]);

    const directoryLinkStates = await page.locator('[data-home-anim="archive-directory-line"]').evaluateAll((elements) =>
      elements.map((element) => ({
        tabIndex: (element as HTMLElement).tabIndex,
        tabIndexAttribute: element.getAttribute("tabindex"),
      })),
    );

    expect(directoryLinkStates).toHaveLength(6);
    expect(directoryLinkStates.every((state) => state.tabIndexAttribute !== "-1" && state.tabIndex !== -1)).toBe(true);
  } finally {
    await context.close();
  }
});

test("homepage keeps archive pages visible without JavaScript", async ({ browser }) => {
  const context = await browser.newContext({
    javaScriptEnabled: false,
    viewport: { width: 1440, height: 1100 },
  });

  try {
    const page = await context.newPage();
    await page.goto(homeUrl, { waitUntil: "domcontentloaded" });

    const archivePages = page.locator('[data-home-anim="archive-page"]');
    await expect(archivePages).toHaveCount(2);

    const pageStates = await archivePages.evaluateAll((elements) =>
      elements.map((element) => {
        const styles = getComputedStyle(element);

        return {
          ariaHidden: element.getAttribute("aria-hidden"),
          opacity: Number(styles.opacity),
          visibility: styles.visibility,
        };
      }),
    );

    expect(pageStates).toEqual([
      { ariaHidden: null, opacity: 1, visibility: "visible" },
      { ariaHidden: null, opacity: 1, visibility: "visible" },
    ]);

    const directoryLinkStates = await page.locator('[data-home-anim="archive-directory-line"]').evaluateAll((elements) =>
      elements.map((element) => ({
        tabIndex: (element as HTMLElement).tabIndex,
        tabIndexAttribute: element.getAttribute("tabindex"),
      })),
    );

    expect(directoryLinkStates).toHaveLength(6);
    expect(directoryLinkStates.every((state) => state.tabIndexAttribute !== "-1" && state.tabIndex !== -1)).toBe(true);

    await expect(page.getByTestId("archive-cover")).toHaveCSS("opacity", "0");
  } finally {
    await context.close();
  }
});

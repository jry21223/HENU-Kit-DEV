import { expect, test, type Page } from "@playwright/test";
import { contrastViolations, revealTextBackgrounds } from "./support/color-contrast";
import { expectTouchTargets } from "./support/touch-targets";
import { typographyViolations } from "./support/typography";

test("controlled QuizCraft catalog hands off a bank version before explicit session setup", async ({ page }) => {
  let catalogRequests = 0;
  let sessionRequests = 0;
  let legacyPracticeRequests = 0;
  const sessionIdempotencyKeys: string[] = [];
  const sessionBodies: unknown[] = [];
  page.on("request", (request) => {
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/v1/practice/schools" || pathname === "/api/v1/practice/banks") {
      legacyPracticeRequests += 1;
    }
  });
  await page.route("**/api/v1/practice/catalog", async (route) => {
    catalogRequests += 1;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [
          {
            bank_id: "11111111-1111-4111-8111-111111111111",
            bank_version_id: "22222222-2222-4222-8222-222222222222",
            name: "计算机基础",
            question_count: 42,
            available: true,
            chapters: [{ id: "chapter-1", name: "真实目录章节" }],
          },
        ],
        request_id: "req_catalog_browser",
      }),
    });
  });
  await page.route("**/api/v1/practice/sessions", async (route) => {
    sessionRequests += 1;
    expect(route.request().method()).toBe("POST");
    sessionIdempotencyKeys.push((await route.request().headerValue("idempotency-key")) ?? "");
    sessionBodies.push(route.request().postDataJSON());
    expect(route.request().postDataJSON()).toEqual({
      bank_id: "11111111-1111-4111-8111-111111111111",
      bank_version_id: "22222222-2222-4222-8222-222222222222",
      mode: "random",
      question_count: 7,
    });
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_catalog_handoff_session",
        data: {
          session_id: "33333333-3333-4333-8333-333333333333",
          bank_id: "11111111-1111-4111-8111-111111111111",
          bank_version_id: "22222222-2222-4222-8222-222222222222",
          mode: "random",
          excluded_unavailable_count: 0,
          questions: [
            {
              question_id: "44444444-4444-4444-8444-444444444444",
              question_version_id: "55555555-5555-4555-8555-555555555555",
              type: "single",
              chapter_id: "chapter-1",
              chapter: "真实目录章节",
              content: "由服务端会话选择的题目。",
              options: ["甲", "乙"],
            },
          ],
        },
      }),
    });
  });

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  await expect(page.getByTestId("quizcraft-catalog")).toBeVisible();
  await expect(page.getByText("计算机基础", { exact: true })).toBeVisible();
  await expect(page.getByTestId("quizcraft-catalog-start")).toHaveAttribute(
    "href",
    "/practice/quiz?bank_id=11111111-1111-4111-8111-111111111111&bank_version_id=22222222-2222-4222-8222-222222222222",
  );
  await expect(page.getByText("128,436", { exact: true })).toHaveCount(0);
  await expect(page.getByText(/卷王本王/)).toHaveCount(0);
  await expect(page.getByText("示例题库", { exact: true })).toHaveCount(0);
  await Promise.all([
    page.waitForURL(/\/practice\/quiz\?bank_id=11111111-1111-4111-8111-111111111111&bank_version_id=22222222-2222-4222-8222-222222222222$/),
    page.getByTestId("quizcraft-catalog-start").click(),
  ]);
  await expect(page.getByRole("heading", { name: "组卷设置" })).toBeVisible();
  expect(sessionRequests).toBe(0);
  await page.getByLabel("题数 / COUNT").fill("7");
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: "由服务端会话选择的题目。" })).toBeVisible();
  expect(catalogRequests).toBeGreaterThan(0);
  expect(legacyPracticeRequests).toBe(0);
  expect(sessionRequests).toBe(1);
  expect(sessionIdempotencyKeys[0]).toBeTruthy();
  expect(sessionBodies).toEqual([
    {
      bank_id: "11111111-1111-4111-8111-111111111111",
      bank_version_id: "22222222-2222-4222-8222-222222222222",
      mode: "random",
      question_count: 7,
    },
  ]);
});

test("controlled QuizCraft catalog keeps an upstream failure honest", async ({ page }) => {
  let catalogRequests = 0;
  await page.route("**/api/v1/practice/catalog", async (route) => {
    catalogRequests += 1;
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: "quizcraft_catalog_unavailable",
        request_id: "req_catalog_failure",
      }),
    });
  });

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  await expect(page.getByText("题库暂时加载不出来，请检查网络后重试。")).toBeVisible();
  // 失败只由提示条说一次，题库区不再叠一句空状态（#549）。
  await expect(page.getByText(/内容暂时加载不出来/)).toHaveCount(0);
  await expect(page.getByText("示例题库", { exact: true })).toHaveCount(0);
  await expect(page.getByTestId("quizcraft-catalog-start")).toHaveCount(0);
  expect(catalogRequests).toBeGreaterThan(0);
});

test("390px catalog cards keep every control at least 44×44 (#543)", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/v1/practice/catalog", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [
          {
            bank_id: "11111111-1111-4111-8111-111111111111",
            bank_version_id: "22222222-2222-4222-8222-222222222222",
            name: "计算机基础",
            question_count: 42,
            available: true,
            chapters: [],
          },
        ],
        request_id: "req_catalog_touch_targets",
      }),
    });
  });

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  await expect(page.getByTestId("quizcraft-catalog-start")).toBeVisible();
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
  await expectTouchTargets(page, "/practice (catalog on)");
});

// 生产配置（目录开关开启）下的手机首屏（#542）：搜索框和第一组题库——或它的加载占位——都在第一屏里。
test("390×844 first screen shows the search box and the first bank (#542)", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  let releaseCatalog: () => void = () => {};
  const catalogHeld = new Promise<void>((resolve) => {
    releaseCatalog = resolve;
  });
  await page.route("**/api/v1/practice/catalog", async (route) => {
    await catalogHeld;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [
          {
            bank_id: "11111111-1111-4111-8111-111111111111",
            bank_version_id: "22222222-2222-4222-8222-222222222222",
            name: "计算机基础",
            question_count: 42,
            available: true,
            chapters: [],
          },
        ],
        request_id: "req_catalog_first_screen",
      }),
    });
  });

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  const search = page.getByPlaceholder("如：数据结构 / 高等数学");
  await expect(search).toBeInViewport({ ratio: 1 });
  await expect(page.getByText("加载题库…", { exact: true })).toBeInViewport({ ratio: 1 });

  releaseCatalog();
  await expect(page.getByRole("heading", { name: "计算机基础", exact: true })).toBeInViewport({ ratio: 1 });
});

/** 未登录；题库目录以外的接口不可用，页面其余部分落在各自的失败或空状态。 */
async function mockSignedOutGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_catalog_contrast_unavailable" },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
}

/** 目录里一套可练习、一套暂不可用的题库。 */
async function mockCatalogBanks(page: Page) {
  await page.route("**/api/v1/practice/catalog", (route) =>
    route.fulfill({
      json: {
        banks: [
          {
            bank_id: "11111111-1111-4111-8111-111111111111",
            bank_version_id: "22222222-2222-4222-8222-222222222222",
            name: "计算机基础",
            question_count: 42,
            available: true,
            chapters: [],
          },
          {
            bank_id: "66666666-6666-4666-8666-666666666666",
            bank_version_id: "77777777-7777-4777-8777-777777777777",
            name: "数据结构",
            question_count: 18,
            available: false,
            chapters: [],
          },
        ],
        request_id: "req_catalog_contrast",
      },
    })
  );
}

// 搜不到题库时给「清除搜索」（#545）：清空搜索、题库回来，按钮随空状态消失后焦点交给搜索区。
test("a search with no matching bank offers to clear it and brings the catalog back", async ({ page }) => {
  await mockSignedOutGateway(page);
  await mockCatalogBanks(page);

  await page.goto("/practice", { waitUntil: "domcontentloaded" });
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
  const computing = page.getByRole("heading", { name: "计算机基础", exact: true });
  await expect(computing).toBeVisible();
  await expect(page.getByRole("button", { name: "清除搜索" })).toHaveCount(0);

  const search = page.getByPlaceholder("如：数据结构 / 高等数学");
  await search.fill("高等数学");
  await expect(page.getByText("无匹配题库", { exact: true })).toBeVisible();
  await expect(computing).toHaveCount(0);

  await page.getByRole("button", { name: "清除搜索", exact: true }).click();
  await expect(computing).toBeVisible();
  await expect(page.getByRole("heading", { name: "数据结构", exact: true })).toBeVisible();
  await expect(search).toHaveValue("");
  await expect(page.getByRole("search", { name: "题库搜索" })).toBeFocused();
  await expect(page.getByRole("button", { name: "清除搜索" })).toHaveCount(0);
});

/**
 * 生产构建开着题库目录（scripts/ops/henukit-release-images.sh），用户看到的 /practice 是题库卡片，
 * 或目录读不到时的失败提示。文字对比度要求同 color-contrast.spec.ts（#536）：axe 的 color-contrast
 * 规则为 0；字号与字距要求同 typography.spec.ts。桌面 1440 与手机 390 两种宽度都算。
 */
for (const viewport of [
  { label: "1440", width: 1440, height: 900 },
  { label: "390", width: 390, height: 844 },
]) {
  test.describe(`${viewport.label}px 题库目录的文字可读性（#536）`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height }, contextOptions: { reducedMotion: "reduce" } });

    test("可练习与暂不可用的题库卡片，文字对比度都达到 AA", async ({ page }) => {
      await mockSignedOutGateway(page);
      await mockCatalogBanks(page);

      await page.goto("/practice", { waitUntil: "domcontentloaded" });
      await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
      await expect(page.getByTestId("quizcraft-catalog-start")).toBeVisible();
      await expect(page.getByText("当前版本暂不可练习")).toBeVisible();

      await revealTextBackgrounds(page);
      expect(await contrastViolations(page), "/practice（题库卡片）：以下文字的对比度低于 WCAG AA").toEqual([]);
    });

    test("题库读不到时的失败提示，文字对比度达到 AA", async ({ page }) => {
      await mockSignedOutGateway(page);
      await page.route("**/api/v1/practice/catalog", (route) =>
        route.fulfill({
          status: 503,
          json: { error: "quizcraft_catalog_unavailable", request_id: "req_catalog_contrast_failure" },
        })
      );

      await page.goto("/practice", { waitUntil: "domcontentloaded" });
      await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
      await expect(page.getByText("题库暂时加载不出来，请检查网络后重试。")).toBeVisible();

      await revealTextBackgrounds(page);
      expect(await contrastViolations(page), "/practice（加载失败）：以下文字的对比度低于 WCAG AA").toEqual([]);
    });

    test("题库卡片的文字不小于 12px，中文不加宽字距", async ({ page }) => {
      await mockSignedOutGateway(page);
      await mockCatalogBanks(page);

      await page.goto("/practice", { waitUntil: "domcontentloaded" });
      await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
      await expect(page.getByTestId("quizcraft-catalog-start")).toBeVisible();
      await expect(page.getByText("当前版本暂不可练习")).toBeVisible();

      expect(await typographyViolations(page), "/practice（题库卡片）：以下文字的字号或字距不达标").toEqual([]);
    });
  });
}

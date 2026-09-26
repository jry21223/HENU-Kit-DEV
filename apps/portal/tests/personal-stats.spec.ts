import { expect, test } from "@playwright/test";
import { expectTouchTargets } from "./support/touch-targets";

const successPayload = {
  request_id: "req_browser_stats",
  data: {
    total_answers: 4,
    correct_answers: 3,
    accuracy: 75,
    streak_days: 2,
    mastery: [
      {
        bank_id: "10ca9b18-c303-4b7a-ab14-1241e41b665a",
        label: "计算机基础",
        value: 50,
        total_questions: 4,
        correct_questions: 2,
      },
    ],
  },
};

test.describe("QuizCraft personal Practice stats presentation", () => {
  test("Hero and dashboard render the same Gateway-shaped V2 facts across separate browser contexts", async ({ browser }) => {
    let statsCalls = 0;
    const desktop = await browser.newContext();
    const mobile = await browser.newContext({ viewport: { width: 390, height: 844 } });
    const desktopPage = await desktop.newPage();
    const mobilePage = await mobile.newPage();
    try {
      // This is deliberately a browser presentation contract: it verifies how
      // the Portal renders a Gateway response. The real answer -> immutable
      // facts -> two fresh Portal-session chain is covered by the Core and
      // Gateway integration tests, not replaced by this route interception.
      for (const page of [desktopPage, mobilePage]) {
        await page.route("**/api/v1/practice/stats*", async (route) => {
          statsCalls += 1;
          await route.fulfill({ contentType: "application/json", body: JSON.stringify(successPayload) });
        });
      }

      await Promise.all([
        desktopPage.goto("/practice", { waitUntil: "domcontentloaded" }),
        mobilePage.goto("/practice", { waitUntil: "domcontentloaded" }),
      ]);
      await expect(desktopPage.getByTestId("practice-hero-stats-state")).toHaveText("作答数和正确率根据你的答题记录计算。");
      await expect(desktopPage.getByTestId("practice-hero-stats-state").locator("xpath=.."))
        .toContainText("4");
      await expect(desktopPage.locator("main")).toContainText("图谱根据你的答题记录生成。", { useInnerText: true });
      // lg 以下知识点结构图隐藏（#542）：手机上看得到的说明不能再指向看不到的图谱。
      await expect(mobilePage.getByTestId("practice-hero-stats-state")).toHaveText("作答数和正确率根据你的答题记录计算。");
      await expect(mobilePage.locator("main")).not.toContainText("图谱", { useInnerText: true });

      await Promise.all([
        desktopPage.goto("/practice/stats", { waitUntil: "domcontentloaded" }),
        mobilePage.goto("/practice/stats", { waitUntil: "domcontentloaded" }),
      ]);
      for (const page of [desktopPage, mobilePage]) {
        await expect(page.getByTestId("practice-stats-success")).toBeVisible();
        await expect(page.getByTestId("practice-stats-success")).toContainText("计算机基础");
        await expect(page.getByTestId("practice-stats-success")).toContainText("50% · 2/4");
        await expect(page.locator("main")).not.toContainText("128,436");
        await expect(page.locator("main")).not.toContainText("击败用户");
      }
      expect(await mobilePage.locator("html").evaluate(
        (element) => element.scrollWidth <= element.clientWidth
      )).toBeTruthy();
      expect(statsCalls).toBeGreaterThanOrEqual(3);
    } finally {
      await desktop.close();
      await mobile.close();
    }
  });

  test("loading, zero, dependency failure, and retry remain explicit rather than falling back to mock success", async ({ page }) => {
    let phase: "loading" | "empty" | "failure" | "success" = "loading";
    let releaseLoading: (() => void) | undefined;
    const loadingGate = new Promise<void>((resolve) => {
      releaseLoading = resolve;
    });
    await page.route("**/api/v1/practice/stats*", async (route) => {
      if (phase === "loading") {
        await loadingGate;
      }
      if (phase === "failure") {
        await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "database unavailable" }) });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          request_id: phase === "success" ? "req_browser_retry" : "req_browser_empty",
          data: phase === "success" ? successPayload.data : {
            total_answers: 0,
            correct_answers: 0,
            accuracy: 0,
            streak_days: 0,
            mastery: [],
          },
        }),
      });
    });
    await page.goto("/practice/stats", { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("practice-stats-loading")).toBeVisible();
    phase = "empty";
    releaseLoading?.();
    await expect(page.getByTestId("practice-stats-empty")).toBeVisible();
    await expect(page.getByTestId("practice-stats-empty")).toContainText("还没有学习记录，从第一题开始建立你的学习图谱");
    await expect(page.getByTestId("practice-stats-empty").getByRole("link", { name: "去刷题", exact: true }))
      .toHaveAttribute("href", "/practice");
    await expect(page.locator("main")).not.toContainText("486");

    phase = "failure";
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("practice-stats-error")).toBeVisible();
    await expect(page.getByTestId("practice-stats-error")).toContainText("服务暂时不可用，请稍后再试。");
    await expect(page.locator("main")).not.toContainText("486");

    phase = "success";
    await page.getByRole("button", { name: "重试" }).click();
    await expect(page.getByTestId("practice-stats-success")).toBeVisible();
    await expect(page.getByTestId("practice-stats-success")).toContainText("50% · 2/4");
  });

  test("an unauthenticated visitor receives no personal progress fallback", async ({ page }) => {
    await page.route("**/api/v1/practice/stats*", async (route) => {
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ error: "login required" }),
      });
    });

    await page.goto("/practice/stats", { waitUntil: "domcontentloaded" });

    await expect(page.getByTestId("practice-stats-unauthenticated")).toBeVisible();
    await expect(page.getByTestId("practice-stats-unauthenticated")).toContainText("登录后查看跨设备同步的学习状态");
    await expect(page.locator("main")).not.toContainText("486");
  });

  // The release build turns V2 reads on, so anonymous visitors get these
  // buttons; with the flag off they never render, which is why this lives in
  // the stats group rather than touch-targets.spec.ts.
  test("390px sign-in and retry buttons for personal stats are at least 44×44 (#543)", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    let statsStatus = 401;
    await page.route("**/api/v1/**", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" } }),
      })
    );
    await page.route("**/api/v1/session", (route) =>
      route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
    );
    await page.route("**/api/v1/practice/stats*", (route) =>
      route.fulfill({
        status: statsStatus,
        contentType: "application/json",
        body: JSON.stringify({ error: statsStatus === 401 ? "login required" : "database unavailable" }),
      })
    );
    const hydrated = () =>
      expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });

    await page.goto("/");
    await hydrated();
    await expect(page.getByRole("button", { name: "登录查看" })).toBeVisible();
    await expectTouchTargets(page, "home (mastery signed out)");

    statsStatus = 503;
    await page.reload();
    await hydrated();
    // Module 01 (library) also fails here and shows its own 重试, and it does so
    // with the flag off too: only the one inside module 02's mastery alert is
    // the V2-only button.
    const mastery = page.getByRole("alert").filter({ hasText: "掌握度数据暂时不可用" });
    await expect(mastery.getByRole("button", { name: "重试" })).toBeVisible();
    await expectTouchTargets(page, "home (mastery unavailable)");

    statsStatus = 401;
    await page.goto("/practice/stats");
    await hydrated();
    await expect(page.getByTestId("practice-stats-unauthenticated")).toBeVisible();
    await expectTouchTargets(page, "/practice/stats (signed out)");
  });
});

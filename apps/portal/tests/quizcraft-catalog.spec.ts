import { expect, test } from "@playwright/test";

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
  await expect(page.getByText("示例题库", { exact: true })).toHaveCount(0);
  await expect(page.getByTestId("quizcraft-catalog-start")).toHaveCount(0);
  expect(catalogRequests).toBeGreaterThan(0);
});

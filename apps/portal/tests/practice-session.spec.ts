import { expect, test } from "@playwright/test";

const bankID = "33333333-3333-4333-8333-333333333333";
const bankVersionID = "44444444-4444-4444-8444-444444444444";
const sessionID = "22222222-2222-4222-8222-222222222222";
const questionID = "55555555-5555-4555-8555-555555555555";
const questionVersionID = "66666666-6666-4666-8666-666666666666";

function realSessionResponse() {
  return {
    request_id: "req_real_session",
    data: {
      session_id: sessionID,
      bank_id: bankID,
      bank_version_id: bankVersionID,
      mode: "random",
      excluded_unavailable_count: 0,
      questions: [
        {
          question_id: questionID,
          question_version_id: questionVersionID,
          type: "single",
          chapter_id: "chapter-1",
          chapter: "真实章节",
          content: "服务端选择的题目，不含标准答案。",
          options: ["甲", "乙"],
        },
      ],
    },
  };
}

test("Practice quiz submits to Gateway and retries the same server-owned answer command", async ({ page }) => {
  const answerKeys: string[] = [];
  const answerBodies: unknown[] = [];
  let answerCalls = 0;
  let sessionCalls = 0;

  await page.route("**/api/v1/practice/catalog", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [{
          bank_id: bankID,
          bank_version_id: bankVersionID,
          name: "计算机基础",
          question_count: 42,
          available: true,
          chapters: [{ id: "chapter-1", name: "真实章节" }],
        }],
        request_id: "req_direct_setup_catalog",
      }),
    });
  });
  await page.route("**/api/v1/practice/sessions", async (route) => {
    sessionCalls += 1;
    expect(route.request().method()).toBe("POST");
    expect(await route.request().headerValue("idempotency-key")).toBeTruthy();
    expect(route.request().postDataJSON()).toEqual({
      bank_id: bankID,
      bank_version_id: bankVersionID,
      mode: "random",
      question_count: 1,
    });
    await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(realSessionResponse()) });
  });
  await page.route(`**/api/v1/practice/sessions/${sessionID}/answers`, async (route) => {
    answerCalls += 1;
    answerKeys.push((await route.request().headerValue("idempotency-key")) ?? "");
    answerBodies.push(route.request().postDataJSON());
    if (answerCalls === 1) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "practice_commands_unavailable", request_id: "req_first_failure" }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_server_score",
        data: {
          question_id: questionID,
          question_version_id: questionVersionID,
          correct: false,
          replayed: false,
          expected_answer: 1,
          analysis: "这份解析来自服务端判题结果。",
        },
      }),
    });
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}&mode=random&question_count=1`);
  await expect(page.getByRole("heading", { name: "组卷设置" })).toBeVisible();
  expect(sessionCalls).toBe(0);
  await page.getByLabel("题数 / COUNT").fill("1");
  await page.getByRole("button", { name: "开始练习 →" }).click();
  await expect(page.getByRole("heading", { name: "服务端选择的题目，不含标准答案。" })).toBeVisible();
  await expect(page.getByText("下列排序算法中，最坏情况下时间复杂度为 O(n²) 的是？")).toHaveCount(0);

  await page.getByRole("button", { name: /甲/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.locator("p[role='alert']")).toContainText("服务暂时不可用，请稍后再试。");
  await expect(page.getByRole("button", { name: /甲/ })).toBeDisabled();
  await expect(page.getByRole("button", { name: /乙/ })).toBeDisabled();
  await page.getByRole("button", { name: "重试提交", exact: true }).click();

  await expect(page.getByText("参考答案：B. 乙")).toBeVisible();
  await expect(page.getByText("这份解析来自服务端判题结果。")).toBeVisible();
  expect(answerCalls).toBe(2);
  expect(answerKeys[0]).toBeTruthy();
  expect(answerKeys[1]).toBe(answerKeys[0]);
  expect(answerBodies).toEqual([
    { question_id: questionID, question_version_id: questionVersionID, answer: 0 },
    { question_id: questionID, question_version_id: questionVersionID, answer: 0 },
  ]);

  const widths = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(widths.scroll).toBeLessThanOrEqual(widths.client + 1);
});

test("Practice commits all setup modes, ignores stale URL settings, and retries one failed start", async ({ page }) => {
  const staleSetupURL = `/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}&mode=chapter&chapter_id=stale-chapter&question_count=500`;
  const sessionBodies: unknown[] = [];
  const sessionKeys: string[] = [];
  let difficultCalls = 0;

  await page.route("**/api/v1/practice/catalog", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [{
          bank_id: bankID,
          bank_version_id: bankVersionID,
          name: "计算机基础",
          question_count: 42,
          available: true,
          chapters: [
            { id: "chapter-1", name: "第一章" },
            { id: "chapter-2", name: "第二章" },
          ],
        }],
        request_id: "req_setup_modes_catalog",
      }),
    });
  });
  await page.route("**/api/v1/practice/sessions", async (route) => {
    const body = route.request().postDataJSON() as { mode: "random" | "difficult" | "chapter" };
    sessionBodies.push(body);
    sessionKeys.push((await route.request().headerValue("idempotency-key")) ?? "");
    if (body.mode === "difficult") {
      difficultCalls += 1;
      if (difficultCalls === 1) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "practice_commands_unavailable", request_id: "req_start_failure" }),
        });
        return;
      }
    }
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        ...realSessionResponse(),
        request_id: `req_${body.mode}_session`,
        data: {
          ...realSessionResponse().data,
          mode: body.mode,
          questions: [{
            ...realSessionResponse().data.questions[0],
            content: `${body.mode} 模式的服务端题目`,
          }],
        },
      }),
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });

  await page.goto(staleSetupURL);
  await expect(page.getByRole("heading", { name: "组卷设置" })).toBeVisible();
  expect(sessionBodies).toHaveLength(0);
  await page.getByLabel("题数 / COUNT").fill("7");
  await page.getByRole("button", { name: /随机/ }).click();
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: "random 模式的服务端题目" })).toBeVisible();
  expect(sessionBodies[0]).toEqual({
    bank_id: bankID,
    bank_version_id: bankVersionID,
    mode: "random",
    question_count: 7,
  });

  await page.goto(staleSetupURL);
  await page.getByRole("button", { name: /难题/ }).click();
  await page.getByLabel("题数 / COUNT").fill("9");
  await page.getByTestId("practice-session-start").click();
  await expect(page.locator("p[role='alert']")).toContainText("服务暂时不可用，请稍后再试。");
  await expect(page.getByTestId("practice-session-start")).toBeEnabled();
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: "difficult 模式的服务端题目" })).toBeVisible();
  expect(sessionBodies.slice(1, 3)).toEqual([
    { bank_id: bankID, bank_version_id: bankVersionID, mode: "difficult", question_count: 9 },
    { bank_id: bankID, bank_version_id: bankVersionID, mode: "difficult", question_count: 9 },
  ]);
  expect(sessionKeys[1]).toBeTruthy();
  expect(sessionKeys[2]).toBe(sessionKeys[1]);

  await page.goto(staleSetupURL);
  await page.getByRole("button", { name: /章节/ }).click();
  await page.getByLabel("题数 / COUNT").fill("0");
  await expect(page.locator("p[role='alert']")).toContainText("题数需为 1 到 500 之间的整数。");
  await expect(page.getByTestId("practice-session-start")).toBeDisabled();
  await page.getByLabel("题数 / COUNT").fill("3");
  await expect(page.getByTestId("practice-session-start")).toBeDisabled();
  await page.getByLabel("选择章节").selectOption("chapter-2");
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: "chapter 模式的服务端题目" })).toBeVisible();
  expect(sessionBodies[3]).toEqual({
    bank_id: bankID,
    bank_version_id: bankVersionID,
    mode: "chapter",
    chapter_id: "chapter-2",
    question_count: 3,
  });

  const widths = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(widths.scroll).toBeLessThanOrEqual(widths.client + 1);
});

test("Practice blocks a stale bank version until the refreshed catalog confirms it", async ({ page }) => {
  let catalogAvailable = false;
  let catalogRequests = 0;
  let sessionRequests = 0;

  await page.route("**/api/v1/practice/catalog", async (route) => {
    catalogRequests += 1;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: catalogAvailable ? [{
          bank_id: bankID,
          bank_version_id: bankVersionID,
          name: "计算机基础",
          question_count: 42,
          available: true,
          chapters: [{ id: "chapter-1", name: "第一章" }],
        }] : [],
        request_id: `req_stale_catalog_${catalogRequests}`,
      }),
    });
  });
  await page.route("**/api/v1/practice/sessions", async (route) => {
    sessionRequests += 1;
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify(realSessionResponse()),
    });
  });

  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}&mode=random`);
  await expect(page.getByRole("heading", { name: "当前题库版本不可用" })).toBeVisible();
  await expect(page.getByText("题库版本可能已更新或下架，请重新检查，或返回题库目录选择当前版本。")).toBeVisible();
  await expect(page.getByTestId("practice-session-start")).toHaveCount(0);
  await expect(page.getByRole("link", { name: /返回题库目录/ })).toHaveAttribute("href", "/practice");
  expect(sessionRequests).toBe(0);

  catalogAvailable = true;
  await page.getByRole("button", { name: "重新检查题库" }).click();
  await expect(page.getByRole("heading", { name: "组卷设置" })).toBeVisible();
  await expect(page.getByTestId("practice-session-start")).toBeEnabled();
  expect(catalogRequests).toBeGreaterThan(1);
  expect(sessionRequests).toBe(0);
});

test("Practice announces catalog confirmation and blocks start until the selected version is ready", async ({ page }) => {
  let releaseCatalog: (() => void) | undefined;
  const catalogGate = new Promise<void>((resolve) => {
    releaseCatalog = resolve;
  });

  await page.route("**/api/v1/practice/catalog", async (route) => {
    await catalogGate;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [{
          bank_id: bankID,
          bank_version_id: bankVersionID,
          name: "计算机基础",
          question_count: 42,
          available: true,
          chapters: [{ id: "chapter-1", name: "第一章" }],
        }],
        request_id: "req_delayed_catalog",
      }),
    });
  });

  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  const setup = page.getByTestId("practice-session-setup");
  await expect(setup).toHaveAttribute("aria-busy", "true");
  await expect(setup.getByRole("status")).toHaveText("正在确认题库版本…");
  await expect(setup.getByTestId("practice-session-start")).toBeDisabled();

  releaseCatalog?.();
  await expect(setup).toHaveAttribute("aria-busy", "false");
  await expect(setup.getByRole("status")).toHaveCount(0);
  await expect(setup.getByTestId("practice-session-start")).toBeEnabled();
});

test("Practice reports catalog confirmation failure without guessing the cause", async ({ page }) => {
  await page.route("**/api/v1/practice/catalog", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "quizcraft_catalog_unavailable", request_id: "req_catalog_error" }),
    })
  );

  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  await expect(page.getByRole("heading", { name: "暂时无法确认题库版本", exact: true })).toBeVisible();
  await expect(
    page.getByText("暂时无法确认题库版本，请重新检查；若仍无法加载，请返回题库目录重新选择。", { exact: true })
  ).toBeVisible();
  await expect(page.getByText(/检查网络/)).toHaveCount(0);
});

test("Practice quiz without a real bank selection remains an honest empty state", async ({ page }) => {
  let commandRequests = 0;
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/practice/sessions")) commandRequests += 1;
  });

  await page.goto("/practice/quiz");
  await expect(page.getByRole("heading", { name: "请先选择题库" })).toBeVisible();
  await expect(page.getByText("请先从题库目录选择一组练习后开始。")).toBeVisible();
  expect(commandRequests).toBe(0);
});

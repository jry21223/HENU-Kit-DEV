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

  await page.route("**/api/v1/practice/sessions", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(await route.request().headerValue("idempotency-key")).toBeTruthy();
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
  await expect(page.getByRole("heading", { name: "服务端选择的题目，不含标准答案。" })).toBeVisible();
  await expect(page.getByText("下列排序算法中，最坏情况下时间复杂度为 O(n²) 的是？")).toHaveCount(0);

  await page.getByRole("button", { name: /甲/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.locator("p[role='alert']")).toContainText("接口错误 (503)");
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

test("Practice quiz without a real bank selection remains an honest empty state", async ({ page }) => {
  let commandRequests = 0;
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/practice/sessions")) commandRequests += 1;
  });

  await page.goto("/practice/quiz");
  await expect(page.getByRole("heading", { name: "题库目录尚未切换" })).toBeVisible();
  await expect(page.getByText("请先从真实题库目录选择一组练习。当前页面不会加载演示题目。")).toBeVisible();
  expect(commandRequests).toBe(0);
});

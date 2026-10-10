import { expect, test, type Page } from "@playwright/test";

const bankID = "33333333-3333-4333-8333-333333333333";
const bankVersionID = "44444444-4444-4444-8444-444444444444";
const sessionID = "22222222-2222-4222-8222-222222222222";
const questionID = "55555555-5555-4555-8555-555555555555";
const questionVersionID = "66666666-6666-4666-8666-666666666666";

type FeedbackCase = {
  name: string;
  type: "single" | "multi" | "judge";
  options?: string[];
  /** Server-owned scoring result the answer endpoint returns. */
  correct: boolean;
  expected: unknown;
};

async function openScoredQuestion(page: Page, input: FeedbackCase) {
  await page.route("**/api/v1/practice/catalog", (route) =>
    route.fulfill({
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
        request_id: "req_feedback_catalog",
      }),
    })
  );
  await page.route("**/api/v1/practice/sessions", (route) =>
    route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_feedback_session",
        data: {
          session_id: sessionID,
          bank_id: bankID,
          bank_version_id: bankVersionID,
          mode: "random",
          excluded_unavailable_count: 0,
          questions: [{
            question_id: questionID,
            question_version_id: questionVersionID,
            type: input.type,
            chapter_id: "chapter-1",
            chapter: "第一章",
            content: "判题反馈测试题",
            ...(input.options ? { options: input.options } : {}),
          }],
        },
      }),
    })
  );
  await page.route(`**/api/v1/practice/sessions/${sessionID}/answers`, (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_feedback_answer",
        data: {
          question_id: questionID,
          question_version_id: questionVersionID,
          correct: input.correct,
          replayed: false,
          expected_answer: input.expected,
          analysis: "服务端解析。",
        },
      }),
    })
  );

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  await page.getByLabel("题数 / COUNT").fill("1");
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: "判题反馈测试题" })).toBeVisible();
}

test("multi choice: picking only one right option is wrong, shows the missed answer and colours the jump cell", async ({ page }) => {
  await openScoredQuestion(page, {
    name: "multi",
    type: "multi",
    options: ["甲", "乙", "丙", "丁"],
    correct: false,
    expected: [1, 3],
  });

  await page.getByRole("button", { name: /乙/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();

  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答错误/);
  const picked = page.getByRole("button", { name: /乙/ });
  const missed = page.getByRole("button", { name: /丁/ });
  await expect(picked).toContainText("✓");
  await expect(picked).not.toContainText("正确答案");
  await expect(missed).toContainText("正确答案");
  await expect(missed).not.toContainText("✓");
  await expect(page.getByRole("button", { name: /甲/ })).not.toContainText(/✓|✗|正确答案/);

  const cell = page.getByRole("button", { name: "第 1 题，答错" });
  await expect(cell).toHaveClass(/bg-accent/);
  await expect(cell).toHaveAttribute("aria-current", "step");
});

test("multi choice: picking every right option is right", async ({ page }) => {
  await openScoredQuestion(page, {
    name: "multi-right",
    type: "multi",
    options: ["甲", "乙", "丙", "丁"],
    correct: true,
    expected: [1, 3],
  });

  await page.getByRole("button", { name: /乙/ }).click();
  await page.getByRole("button", { name: /丁/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();

  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答正确/);
  await expect(page.getByRole("button", { name: /乙/ })).toContainText("✓");
  await expect(page.getByRole("button", { name: /丁/ })).toContainText("✓");
  await expect(page.getByRole("button", { name: "第 1 题，答对" })).toHaveClass(/bg-ink/);
});

test("single choice: a wrong pick shows ✗ and marks the right option as the answer", async ({ page }) => {
  await openScoredQuestion(page, {
    name: "single",
    type: "single",
    options: ["甲", "乙", "丙"],
    correct: false,
    expected: 1,
  });

  await page.getByRole("button", { name: /甲/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();

  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答错误/);
  await expect(page.getByRole("button", { name: /甲/ })).toContainText("✗");
  await expect(page.getByRole("button", { name: /乙/ })).toContainText("正确答案");
  await expect(page.getByRole("button", { name: /丙/ })).not.toContainText(/✓|✗|正确答案/);
});

test("judge: a wrong pick shows ✗ and marks the other side as the answer", async ({ page }) => {
  await openScoredQuestion(page, { name: "judge", type: "judge", correct: false, expected: false });

  await page.getByRole("button", { name: /^正确$/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();

  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答错误/);
  await expect(page.getByRole("button", { name: /^正确/ })).toContainText("✗");
  await expect(page.getByRole("button", { name: /^错误/ })).toContainText("正确答案");
});

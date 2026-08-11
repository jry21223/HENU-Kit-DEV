import { expect, test, type Browser, type Page } from "@playwright/test";

const bankID = "33333333-3333-4333-8333-333333333333";
const bankVersionID = "44444444-4444-4444-8444-444444444444";
const sessionID = "22222222-2222-4222-8222-222222222222";

const questions = [
  {
    question_id: "55555555-5555-4555-8555-555555555551",
    question_version_id: "66666666-6666-4666-8666-666666666661",
    type: "single",
    chapter_id: "chapter-1",
    chapter: "第一章",
    content: "第一题：选择甲。",
    options: ["甲", "乙"],
  },
  {
    question_id: "55555555-5555-4555-8555-555555555552",
    question_version_id: "66666666-6666-4666-8666-666666666662",
    type: "blank",
    chapter_id: "chapter-2",
    chapter: "第二章",
    content: "第二题：填写草稿。",
  },
  {
    question_id: "55555555-5555-4555-8555-555555555553",
    question_version_id: "66666666-6666-4666-8666-666666666663",
    type: "single",
    chapter_id: "chapter-3",
    chapter: "第三章",
    content: "第三题：末题边界。",
    options: ["丙", "丁"],
  },
] as const;

async function newMobilePage(browser: Browser, reducedMotion: "reduce" | "no-preference" = "no-preference") {
  const context = await browser.newContext({
    baseURL: "http://localhost:3001",
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
    reducedMotion,
  });
  return { context, page: await context.newPage() };
}

async function mockPractice(page: Page) {
  let answerCalls = 0;
  const answerKeys: string[] = [];
  await page.route("**/api/v1/practice/catalog", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        banks: [{
          bank_id: bankID,
          bank_version_id: bankVersionID,
          name: "移动练习题库",
          question_count: 3,
          available: true,
          chapters: questions.map((question) => ({ id: question.chapter_id, name: question.chapter })),
        }],
        request_id: "req_swipe_catalog",
      }),
    })
  );
  await page.route("**/api/v1/practice/sessions", (route) =>
    route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_swipe_session",
        data: {
          session_id: sessionID,
          bank_id: bankID,
          bank_version_id: bankVersionID,
          mode: "random",
          excluded_unavailable_count: 0,
          questions,
        },
      }),
    })
  );
  await page.route(`**/api/v1/practice/sessions/${sessionID}/answers`, async (route) => {
    answerCalls += 1;
    answerKeys.push((await route.request().headerValue("idempotency-key")) ?? "");
    if (answerCalls === 1) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "practice_commands_unavailable", request_id: "req_swipe_retry" }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_swipe_answer",
        data: {
          question_id: questions[0].question_id,
          question_version_id: questions[0].question_version_id,
          correct: true,
          replayed: false,
          expected_answer: 0,
          analysis: "服务端保留的第一题解析。",
        },
      }),
    });
  });
  return { answerKeys, answerCalls: () => answerCalls };
}

async function startPractice(page: Page) {
  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  await page.getByLabel("题数 / COUNT").fill("3");
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading", { name: questions[0].content })).toBeVisible();
}

async function touchGesture(page: Page, selector: string, deltaX: number, deltaY: number) {
  await touchPath(page, selector, [{ x: deltaX, y: deltaY }]);
}

async function touchPath(page: Page, selector: string, points: Array<{ x: number; y: number }>) {
  await page.locator(selector).evaluate((element, delta) => {
    const dispatch = (type: string, x: number, y: number, active: boolean) => {
      const touch = new Touch({ identifier: 1, target: element, clientX: x, clientY: y });
      element.dispatchEvent(new TouchEvent(type, {
        bubbles: true,
        cancelable: true,
        touches: active ? [touch] : [],
        changedTouches: [touch],
      }));
    };
    dispatch("touchstart", 250, 320, true);
    for (const point of delta) dispatch("touchmove", 250 + point.x, 320 + point.y, true);
    const end = delta.at(-1) ?? { x: 0, y: 0 };
    dispatch("touchend", 250 + end.x, 320 + end.y, false);
  }, points);
}

test("390px touch swipe navigates without losing drafts, retry identity, or server results", async ({ browser }) => {
  const { context, page } = await newMobilePage(browser);
  const commands = await mockPractice(page);
  await startPractice(page);
  const card = "[data-testid='practice-question-card']";

  await page.getByRole("button", { name: /甲/ }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("button", { name: "重试提交" })).toBeVisible();

  await touchGesture(page, card, -110, 8);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await page.getByLabel("填写答案").fill("保留这份草稿");
  await touchGesture(page, card, 20, 2);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await touchGesture(page, card, 12, 140);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await touchPath(page, card, [{ x: 13, y: 0 }, { x: 60, y: 120 }]);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await touchGesture(page, "input[aria-label='填写答案']", -120, 4);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();

  await touchGesture(page, card, 110, 5);
  await expect(page.getByRole("heading", { name: questions[0].content })).toBeVisible();
  await expect(page.getByRole("button", { name: "重试提交" })).toBeVisible();
  await page.getByRole("button", { name: "重试提交" }).click();
  await expect(page.getByText("服务端保留的第一题解析。")).toBeVisible();
  expect(commands.answerCalls()).toBe(2);
  expect(commands.answerKeys[0]).toBeTruthy();
  expect(commands.answerKeys[1]).toBe(commands.answerKeys[0]);

  await page.getByRole("button", { name: "下一道题" }).click();
  await expect(page.getByLabel("填写答案")).toHaveValue("保留这份草稿");
  await page.getByLabel("填写答案").press("ArrowRight");
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await page.getByRole("heading", { name: questions[1].content }).click();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("heading", { name: questions[2].content })).toBeVisible();
  await touchGesture(page, card, -120, 2);
  await expect(page.getByRole("heading", { name: questions[2].content })).toBeVisible();
  await page.keyboard.press("ArrowLeft");
  await page.getByRole("button", { name: "上一道题" }).click();
  await expect(page.getByText("服务端保留的第一题解析。")).toBeVisible();
  await touchGesture(page, card, 120, 3);
  await expect(page.getByRole("heading", { name: questions[0].content })).toBeVisible();
  expect(commands.answerCalls()).toBe(2);

  await touchGesture(page, "[data-feedback] > button", -120, 3);
  await page.getByRole("button", { name: "这道题有问题？提交纠错 +" }).click();
  const correction = page.getByPlaceholder("简单描述问题，例如：第 2 题解析里的公式有笔误。");
  await correction.fill("移动端纠错输入保持可用");
  await touchGesture(page, "[data-feedback] textarea", -120, 3);
  await expect(correction).toHaveValue("移动端纠错输入保持可用");

  const favorite = page.getByRole("button", { name: "登录后收藏" });
  await expect(favorite).toBeEnabled();
  await touchGesture(page, "button:has-text('登录后收藏')", -120, 3);
  await expect(page.getByRole("heading", { name: questions[0].content })).toBeVisible();

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 1);
  await favorite.click();
  await expect(page).toHaveURL(/\/api\/v1\/auth\/login\?return_to=/);
  await context.close();
});

test("reduced motion reaches the same swipe destination while mouse drag does nothing", async ({ browser }) => {
  const { context, page } = await newMobilePage(browser, "reduce");
  await mockPractice(page);
  await startPractice(page);
  const card = page.getByTestId("practice-question-card");

  const box = await card.boundingBox();
  if (!box) throw new Error("question card is not visible");
  await page.mouse.move(box.x + box.width - 30, box.y + 100);
  await page.mouse.down();
  await page.mouse.move(box.x + 30, box.y + 104);
  await page.mouse.up();
  await expect(page.getByRole("heading", { name: questions[0].content })).toBeVisible();

  await touchGesture(page, "[data-testid='practice-question-card']", -120, 4);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await context.close();
});

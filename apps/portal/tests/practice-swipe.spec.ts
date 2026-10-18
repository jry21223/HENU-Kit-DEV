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

async function mockPractice(page: Page, firstContent?: string) {
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
          questions: firstContent ? questions.map((item, index) => index === 0 ? { ...item, content: firstContent } : item) : questions,
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
  const target = selector.includes("practice-question-card") ? page.locator(selector) : page.getByTestId("practice-question-card").locator(selector);
  await target.evaluate((element, delta) => {
    const dispatch = (type: string, x: number, y: number) => {
      element.dispatchEvent(new PointerEvent(type, {
        bubbles: true, cancelable: true, pointerId: 1, pointerType: "touch",
        isPrimary: true, clientX: x, clientY: y,
      }));
    };
    dispatch("pointerdown", 250, 320);
    // Cross the intent threshold before following the rest of the drag.
    const first = delta[0] ?? { x: 0, y: 0 };
    dispatch("pointermove", 250 + first.x * 0.2, 320 + first.y * 0.2);
    for (const point of delta) dispatch("pointermove", 250 + point.x, 320 + point.y);
    const end = delta.at(-1) ?? { x: 0, y: 0 };
    dispatch("pointerup", 250 + end.x, 320 + end.y);
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
  await touchGesture(page, card, 6, 2);
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
  await expect(page.getByRole("button", { name: "下一道题" })).toBeFocused();
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

  // 上一题 / 下一题是一组有名字的按钮，读屏读得出「题目导航」。
  await expect(page.getByRole("group", { name: "题目导航" })).toBeVisible();
  // 纠错入口是文字按钮，点击区同样撑到 44px 高（DESIGN_SYSTEM §13）。
  const feedbackToggleBox = await page.getByRole("button", { name: "这道题有问题？提交纠错 +" }).boundingBox();
  expect(feedbackToggleBox?.height ?? 0).toBeGreaterThanOrEqual(44);
  await touchGesture(page, "[data-feedback] > button", -120, 3);
  await page.getByRole("button", { name: "这道题有问题？提交纠错 +" }).click();
  const correction = page.getByTestId("practice-question-card").getByPlaceholder("简单描述问题，例如：第 2 题解析里的公式有笔误。");
  await correction.fill("移动端纠错输入保持可用");
  await touchGesture(page, "[data-feedback] textarea", -120, 3);
  await expect(correction).toHaveValue("移动端纠错输入保持可用");

  const favorite = page.getByRole("button", { name: "登录后收藏" });
  await expect(favorite).toBeEnabled();
  // 收藏和纠错里有边框的按钮本身做到 44px 高（DESIGN_SYSTEM §13）。
  for (const name of ["登录后收藏", "答案有误", "表述歧义", "错别字", "内容过时", "其他", "提交纠错"]) {
    await expect(page.getByRole("button", { name, exact: true }), name).toHaveCSS("min-height", "44px");
  }
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

test("real mobile touch follows the finger and scrolls code without changing questions", async ({ browser, browserName }, testInfo) => {
  test.skip(browserName !== "chromium", "CDP supplies real touchscreen input in Chromium");
  const { context, page } = await newMobilePage(browser);
  await mockPractice(page, '阅读下面代码：\n\n```java\nList<String> list = new ArrayList<>();\n  list.add("a very long value that extends beyond the mobile code viewport");\n```');
  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  await page.getByLabel("题数 / COUNT").fill("3");
  await page.getByTestId("practice-session-start").click();
  const card = page.getByTestId("practice-question-card");
  const code = card.locator("pre");
  await expect(code).toContainText("List<String>");
  await expect(code).toHaveCSS("font-size", "14px");
  await expect(card.locator(".hljs-keyword")).not.toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("markdown-mobile.png"), fullPage: true });
  await code.focus();
  await code.press("ArrowRight");
  await expect(page.getByRole("heading", { name: questions[1].content })).toHaveCount(0);
  await code.evaluate((element) => { element.scrollLeft = 0; element.blur(); });
  const session = await context.newCDPSession(page);
  const touch = async (type: "touchStart" | "touchMove" | "touchEnd", x: number, y: number) => {
    await session.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y }] });
  };
  await code.scrollIntoViewIfNeeded();
  const box = (await code.boundingBox())!;
  const x = box.x + box.width - 24;
  const y = box.y + 30;
  await touch("touchStart", x, y);
  for (let offset = 15; offset <= 135; offset += 15) await touch("touchMove", x - offset, y);
  await touch("touchEnd", x - 135, y);
  await expect.poll(() => code.evaluate((el) => el.scrollLeft)).toBeGreaterThan(40);
  await expect(page.getByRole("button", { name: "下一道题" })).toBeVisible();
  await expect(page.getByRole("heading", { name: questions[1].content })).toHaveCount(0);

  // Drag on the question text and inspect geometry before releasing: a fade
  // that starts only on touchend cannot satisfy this assertion.
  const stem = card.getByRole("heading");
  await stem.scrollIntoViewIfNeeded();
  const stemBox = (await stem.boundingBox())!;
  const before = (await card.boundingBox())!;
  const dragX = before.x + before.width - 30;
  const dragY = stemBox.y + 12;
  await touch("touchStart", dragX, dragY);
  await touch("touchMove", dragX - 20, dragY);
  await touch("touchMove", dragX - 70, dragY);
  await touch("touchMove", dragX - 120, dragY);
  await expect.poll(async () => (await card.boundingBox())!.x).toBeLessThan(before.x - 60);
  await touch("touchEnd", dragX - 120, dragY);
  await expect(page.getByRole("heading", { name: questions[1].content })).toBeVisible();
  await page.getByRole("button", { name: "上一道题" }).click();
  await expect(page.getByTestId("practice-question-card").locator("pre")).toBeVisible();
  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 1);
  await context.close();
});

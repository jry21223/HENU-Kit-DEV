import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { createServer } from "vite";
import { chromium } from "playwright";

const longJavaOption =
  '普通说明文字\npublic static java.util.Map<String, java.util.List<com.example.quizcraft.ExtremelyLongGenericTypeName>> buildQuestionIndexWithVeryLongMethodName(String bankKey, java.util.Collection<QuestionRecord> records) { return new java.util.LinkedHashMap<>(); }\n结束';
const longHtmlOption =
  '行内普通文字 <a data-super-long-attribute-name="quizcraft-long-html-option-layout-regression-value-without-natural-breakpoints">链接文本</a> 后续说明文字';
const longUrlOption =
  "普通文本 https://superhuazai.me/practice/software_engineering_process_tests/chapter/ch01/question/this-is-a-very-long-url-segment-without-natural-breakpoints-abcdefghijklmnopqrstuvwxyz0123456789 结束";

const practiceState = {
  state: {
    currentBank: "web",
    user: {
      id: "layout-test-user",
      name: "Layout Test",
      correct: 0,
      total: 0,
      rate: 0,
      practice_history: [],
    },
    history: [],
    wrongQuestions: [],
    starredQuestions: [],
    practice: {
      bankKey: "web",
      questions: [
        {
          id: "layout_q1",
          number: "1",
          type: "single",
          chapter: "布局回归测试",
          chapter_id: "ch01",
          content: "行内普通文字 `code` 和长代码选项混排时不应产生横向滚动。",
          options: [
            longJavaOption,
            longHtmlOption,
            longUrlOption,
            "普通短选项",
          ],
          answer: 0,
          analysis: "",
          stats: { total: 0, correct: 0, rate: 0 },
        },
      ],
      currentIndex: 0,
      answers: {},
      results: {},
      correctAnswers: {},
      analyses: {},
      createdAt: Date.now(),
      startTime: Date.now(),
      isFinished: false,
    },
  },
  version: 0,
};

const vite = await createServer({
  server: {
    host: "127.0.0.1",
    port: 0,
  },
  logLevel: "silent",
});

let browser;

const findSystemChromium = () =>
  [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
  ].find((path) => existsSync(path));

try {
  await vite.listen();
  const baseUrl = vite.resolvedUrls?.local?.[0];
  assert.ok(baseUrl, "Vite did not expose a local URL");

  const executablePath = findSystemChromium();
  browser = await chromium.launch(executablePath ? { executablePath } : {});
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  await page.addInitScript((value) => {
    window.localStorage.setItem("quiz-storage", JSON.stringify(value));
  }, practiceState);

  await page.goto(new URL("quiz", baseUrl).href, { waitUntil: "networkidle" });
  await page.getByText("buildQuestionIndexWithVeryLongMethodName").waitFor();

  const metrics = await page.evaluate(() => {
    const root = document.documentElement;
    const body = document.body;
    const optionButtons = Array.from(document.querySelectorAll("button")).filter(
      (button) =>
        button.textContent?.includes("buildQuestionIndexWithVeryLongMethodName") ||
        button.textContent?.includes("data-super-long-attribute-name") ||
        button.textContent?.includes("very-long-url-segment"),
    );

    return {
      viewportWidth: root.clientWidth,
      documentScrollWidth: Math.max(root.scrollWidth, body.scrollWidth),
      options: optionButtons.map((button) => {
        const rect = button.getBoundingClientRect();
        const overflowingDescendants = Array.from(button.querySelectorAll("*"))
          .map((element) => {
            const elementRect = element.getBoundingClientRect();
            return {
              tag: element.tagName.toLowerCase(),
              text: element.textContent?.slice(0, 80) || "",
              left: elementRect.left,
              right: elementRect.right,
            };
          })
          .filter(
            (element) =>
              element.left < rect.left - 1 || element.right > rect.right + 1,
          );
        return {
          text: button.textContent || "",
          clientWidth: button.clientWidth,
          scrollWidth: button.scrollWidth,
          height: rect.height,
          left: rect.left,
          right: rect.right,
          overflowingDescendants,
        };
      }),
    };
  });

  assert.equal(metrics.options.length, 3, "expected to find all long option buttons");
  assert.ok(
    metrics.documentScrollWidth <= metrics.viewportWidth + 1,
    `page introduced horizontal scroll: scrollWidth=${metrics.documentScrollWidth}, viewport=${metrics.viewportWidth}`,
  );

  for (const option of metrics.options) {
    assert.ok(
      option.scrollWidth <= option.clientWidth + 1,
      `option overflows its card: scrollWidth=${option.scrollWidth}, clientWidth=${option.clientWidth}`,
    );
    assert.ok(option.height > 72, "long option card should grow vertically instead of clipping text");
    assert.ok(option.left >= 0 && option.right <= metrics.viewportWidth + 1, "option card should stay inside the viewport");
    assert.deepEqual(
      option.overflowingDescendants,
      [],
      `option descendants should stay inside the option card: ${JSON.stringify(option)}`,
    );
  }
} finally {
  if (browser) {
    await browser.close();
  }
  await vite.close();
}

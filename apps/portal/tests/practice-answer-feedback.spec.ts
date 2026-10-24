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
  content?: string;
  analysis?: string;
  nextContent?: string;
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
            content: input.content ?? "判题反馈测试题",
            ...(input.options ? { options: input.options } : {}),
          }, ...(input.nextContent ? [{
            question_id: "77777777-7777-4777-8777-777777777777",
            question_version_id: "88888888-8888-4888-8888-888888888888",
            type: input.type,
            chapter_id: "chapter-1",
            chapter: "第一章",
            content: input.nextContent,
            ...(input.options ? { options: input.options } : {}),
          }] : [])],
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
          analysis: input.analysis ?? "服务端解析。",
        },
      }),
    })
  );

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}`);
  await page.getByLabel("题数 / COUNT").fill(input.nextContent ? "2" : "1");
  await page.getByTestId("practice-session-start").click();
  await expect(page.getByRole("heading").filter({ hasText: input.content?.split("\n")[0] ?? "判题反馈测试题" })).toBeVisible();
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

for (const viewport of [{ width: 1440, height: 900 }, { width: 1366, height: 768 }, { width: 360, height: 800 }]) {
test(`Markdown figure rows work in options and explanations at ${viewport.width}×${viewport.height}`, async ({ page }) => {
  const sources = [
    "/practice-assets/h3c/dbcf2c743d6de5fdfc8f39eaaf8bfa061975eb3957968464ece706afc5d39c4d.png",
    "/practice-assets/h3c/55b82f46ab0d1734faad4f8df5d543ea5af361680e40dd4c6a554ddd5c8ab52e.png",
  ];
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openScoredQuestion(page, {
    name: "markdown-figure-rows",
    type: "single", correct: true, expected: 0,
    content: "选择配置。",
    options: [`配置甲\n\n![选项左图](${sources[0]})  \n![选项右图](${sources[1]})`, "配置乙"],
    analysis: `配置说明。\n\n![解析左图](${sources[0]})\n\n![解析右图](${sources[1]})\n\n\`\`\`text\n[MSR] ospf 1\n area 0\n  network 10.1.1.0 0.0.0.255\n\`\`\`\n\n配置结束。`,
  });
  await page.setViewportSize(viewport);
  const option = page.getByRole("button", { name: /配置甲/ });
  const left = option.getByRole("img", { name: "选项左图" });
  const right = option.getByRole("img", { name: "选项右图" });
  await expect(left).toHaveJSProperty("naturalWidth", 646);
  await expect(right).toHaveJSProperty("naturalWidth", 649);
  const a = (await left.boundingBox())!;
  const b = (await right.boundingBox())!;
  expect(Math.abs(a.y - b.y)).toBeLessThanOrEqual(1);
  expect(b.x).toBeGreaterThanOrEqual(a.x + a.width);
  expect(b.x + b.width).toBeLessThanOrEqual(viewport.width);
  await expect(option.getByRole("link")).toHaveCount(0);
  await option.click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答正确/);
  const analysisLeft = page.getByRole("img", { name: "解析左图" });
  const analysisRight = page.getByRole("img", { name: "解析右图" });
  await expect(analysisLeft).toHaveJSProperty("naturalWidth", 646);
  await expect(analysisRight).toHaveJSProperty("naturalWidth", 649);
  const c = (await analysisLeft.boundingBox())!;
  const d = (await analysisRight.boundingBox())!;
  expect(Math.abs(c.y - d.y)).toBeLessThanOrEqual(1);
  expect(d.x).toBeGreaterThanOrEqual(c.x + c.width);
  expect(d.x + d.width).toBeLessThanOrEqual(viewport.width);
  await expect(page.getByRole("link", { name: "打开解析左图原图" })).toHaveAttribute("href", sources[0]);
  await expect(page.getByRole("link", { name: "打开解析右图原图" })).toHaveAttribute("href", sources[1]);
  await expect(page.getByLabel("代码，可横向滚动")).toContainText("[MSR] ospf 1\n area 0\n  network 10.1.1.0 0.0.0.255");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(viewport.width);
});

test(`consecutive source figures share a compact row at ${viewport.width}×${viewport.height}`, async ({ page }) => {
  // Frozen GB0-192 source question gb0192_main_q0028, including its image separators.
  const sources = [
    "/practice-assets/h3c/dbcf2c743d6de5fdfc8f39eaaf8bfa061975eb3957968464ece706afc5d39c4d.png",
    "/practice-assets/h3c/55b82f46ab0d1734faad4f8df5d543ea5af361680e40dd4c6a554ddd5c8ab52e.png",
  ];
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openScoredQuestion(page, {
    name: "consecutive-source-figures",
    type: "multi",
    options: ["电路交换延迟小，传输实时性强", "电路交换网络资源利用率高", "分组交换延迟大，传输实时性差", "分组交换网络资源利用率低"],
    correct: true,
    expected: [0, 2],
    content: `以下关于电路交换和分组交换的描述正确的是____。（选择一项或多项）\n\n![题图](${sources[0]})\n![题图](${sources[1]})`,
  });
  await page.setViewportSize(viewport);
  await page.evaluate(() => document.fonts.ready);
  const figures = page.getByRole("img", { name: "题图", exact: true });
  await expect(figures).toHaveCount(2);
  const geometry = [];
  for (let index = 0; index < 2; index++) {
    const image = figures.nth(index);
    await expect(image).toHaveJSProperty("complete", true);
    geometry.push(await image.evaluate((element) => {
      const img = element as HTMLImageElement;
      const rect = img.getBoundingClientRect();
      const style = getComputedStyle(img);
      return {
        x: rect.x, y: rect.y, right: rect.right, bottom: rect.bottom,
        width: rect.width - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight) - parseFloat(style.borderLeftWidth) - parseFloat(style.borderRightWidth),
        height: rect.height - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom) - parseFloat(style.borderTopWidth) - parseFloat(style.borderBottomWidth),
        naturalWidth: img.naturalWidth, naturalHeight: img.naturalHeight, objectFit: style.objectFit,
      };
    }));
    await expect(page.getByRole("link", { name: "打开题图原图", exact: true }).nth(index)).toHaveAttribute("href", sources[index]);
  }
  expect(geometry.map(({ naturalWidth, naturalHeight }) => [naturalWidth, naturalHeight])).toEqual([[646, 434], [649, 539]]);
  expect(Math.abs(geometry[0].y - geometry[1].y)).toBeLessThanOrEqual(1);
  expect(geometry[1].x).toBeGreaterThanOrEqual(geometry[0].right);
  const textBottom = await page.getByRole("heading").filter({ hasText: "以下关于电路交换和分组交换的描述正确的是" }).evaluate((heading) => {
    const walker = document.createTreeWalker(heading, NodeFilter.SHOW_TEXT);
    let lastText: Node | null = null;
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (node.textContent?.trim()) lastText = node;
    }
    const range = document.createRange();
    range.selectNodeContents(lastText!);
    return range.getBoundingClientRect().bottom;
  });
  expect(geometry[0].y - textBottom).toBeLessThanOrEqual(24);
  for (const image of geometry) {
    expect(image.x).toBeGreaterThanOrEqual(0);
    expect(image.right).toBeLessThanOrEqual(viewport.width);
    expect(image.objectFit).toBe("contain");
    expect(image.width).toBeLessThanOrEqual(image.naturalWidth + 1);
    expect(image.height).toBeLessThanOrEqual(image.naturalHeight + 1);
    expect(Math.abs(image.width / image.height - image.naturalWidth / image.naturalHeight)).toBeLessThan(0.03);
  }
  const firstOption = await page.getByRole("button", { name: /电路交换延迟小，传输实时性强/ }).boundingBox();
  expect(firstOption).not.toBeNull();
  expect(firstOption!.y - Math.max(...geometry.map((image) => image.bottom))).toBeLessThanOrEqual(64);
  if (viewport.width >= 1024) expect(firstOption!.y + firstOption!.height).toBeLessThanOrEqual(viewport.height);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(viewport.width);
});

test(`source figures stay compact without cropping or upscaling at ${viewport.width}×${viewport.height}`, async ({ page }) => {
  const src = "/practice-assets/h3c/c8cb43031248931995bb1e9eef62b59c10dc96f33acbdad50cda1eb47377b413.png";
  const small = "/practice-assets/h3c/f55e6873ac899a02f2bed67dffcb9da6ce367dff5db112d8d40f2e0a8976c507.png";
  const portrait = "/practice-assets/h3c/63f4c34b6350bb357aa26ef6bd03234fc5808af9461364540c8e4fcfd09b4470.png";
  const square = "/practice-assets/h3c/2b88300d50f28bf5f842c6d4a35cfe9aa008ec86198de0e4519ea2fd2935ec7e.png";
  const landscape = "/practice-assets/h3c/4547ee6bdcb442f379db407d50749aa871481088d934429d9c09564ddb809670.png";
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openScoredQuestion(page, {
    name: "source-figure",
    type: "single",
    options: ["网络中四台路由器都是 ABR(Area Border Router)", `小图选项\n![选项示意](${small})`],
    correct: true,
    expected: 0,
    content: `在四台 MSR 路由器之间运行 OSPF 并划分区域，其连接以及划分方式如图。已知网络中所有链路的 OSPF Cost 值都为 10，那么关于此图描述正确的是______。（选择一项或多项）\n![网络拓扑](${src})`,
    analysis: `根据题图检查各条链路。\n![纵向配置](${portrait})\n![方形拓扑](${square})\n![横向拓扑](${landscape})`,
  });
  await page.setViewportSize(viewport);
  await page.evaluate(() => document.fonts.ready);
  const figure = page.getByRole("img", { name: "网络拓扑", exact: true });
  await expect(figure).toBeVisible();
  await expect(figure).toHaveJSProperty("naturalWidth", 459);
  const bounds = await figure.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
  expect(bounds!.height).toBeLessThanOrEqual(Math.min(280, viewport.height * 0.28) + 1);
  const original = page.getByRole("link", { name: "打开网络拓扑原图" });
  await expect(original).toHaveAttribute("href", src);
  await expect(original).toHaveAttribute("target", "_blank");
  await expect(original).toHaveAttribute("rel", "noopener noreferrer");

  const firstOption = page.getByRole("button", { name: /网络中四台路由器都是 ABR/ });
  const optionBounds = await firstOption.boundingBox();
  expect(optionBounds).not.toBeNull();
  expect(optionBounds!.y - (bounds!.y + bounds!.height)).toBeLessThanOrEqual(64);
  // On a laptop the diagram must leave room for the first choice in the same viewport.
  if (viewport.width >= 1024) expect(optionBounds!.y + optionBounds!.height).toBeLessThanOrEqual(viewport.height);
  await expect(page.getByRole("button", { name: /小图选项/ }).getByRole("link")).toHaveCount(0);

  await firstOption.click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByTestId("practice-verdict")).toHaveText(/回答正确/);
  for (const label of ["网络拓扑", "选项示意", "纵向配置", "方形拓扑", "横向拓扑"]) {
    const image = page.getByRole("img", { name: label, exact: true });
    await expect(image).toHaveJSProperty("complete", true);
    const geometry = await image.evaluate((element) => {
      const img = element as HTMLImageElement;
      const style = getComputedStyle(img);
      const rect = img.getBoundingClientRect();
      const horizontalInset = parseFloat(style.paddingLeft) + parseFloat(style.paddingRight) + parseFloat(style.borderLeftWidth) + parseFloat(style.borderRightWidth);
      const verticalInset = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom) + parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
      return { x: rect.x, right: rect.right, width: rect.width - horizontalInset, height: rect.height - verticalInset, outerHeight: rect.height, naturalWidth: img.naturalWidth, naturalHeight: img.naturalHeight, objectFit: style.objectFit };
    });
    expect(geometry.naturalWidth).toBeGreaterThan(0);
    expect(geometry.objectFit).toBe("contain");
    expect(geometry.outerHeight).toBeLessThanOrEqual(Math.min(280, viewport.height * 0.28) + 1);
    expect(geometry.x).toBeGreaterThanOrEqual(0);
    expect(geometry.right).toBeLessThanOrEqual(viewport.width);
    expect(geometry.width).toBeLessThanOrEqual(geometry.naturalWidth + 1);
    expect(geometry.height).toBeLessThanOrEqual(geometry.naturalHeight + 1);
    expect(Math.abs(geometry.width / geometry.height - geometry.naturalWidth / geometry.naturalHeight)).toBeLessThan(0.03);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(viewport.width);
});
}

test("a failed figure does not hide the next question's valid figure", async ({ page }) => {
  const nestingErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error" && /cannot be a descendant|cannot contain a nested/.test(message.text())) {
      nestingErrors.push(message.text());
    }
  });
  const missing = `/practice-assets/h3c/${"a".repeat(64)}.png`;
  const present = `/practice-assets/h3c/${"b".repeat(64)}.png`;
  await page.route(`**${missing}`, (route) => route.fulfill({ status: 404, body: "missing" }));
  await page.route(`**${present}`, (route) => route.fulfill({
    contentType: "image/png",
    body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jXioAAAAASUVORK5CYII=", "base64"),
  }));
  await openScoredQuestion(page, {
    name: "figure-recovery", type: "single", options: ["甲", "乙"], correct: true, expected: 0,
    content: `第一题\n![网络拓扑](${missing})`,
    nextContent: `第二题\n![网络拓扑](${present})`,
  });
  await expect(page.getByRole("img", { name: "网络拓扑", exact: true })).toContainText("暂无图片");
  await page.getByRole("button", { name: "第 2 题，未答", exact: true }).click();
  await expect(page.getByRole("heading").filter({ hasText: "第二题" })).toBeVisible();
  const recovered = page.getByRole("img", { name: "网络拓扑", exact: true });
  await expect(recovered).toHaveAttribute("src", present);
  await expect(recovered).toHaveJSProperty("naturalWidth", 1);
  expect(nestingErrors).toEqual([]);
});

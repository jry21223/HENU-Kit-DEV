import { expect, test } from "@playwright/test";
import { expectTouchTargets } from "./support/touch-targets";
import { contrastViolations } from "./support/color-contrast";

test("real cloze stack keeps full context, recovers hints and partial answers, and restores authored review", async ({ page, context }, testInfo) => {
  test.setTimeout(90_000);
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/practice");
  await page.getByRole("link", { name: "英语听力与完形训练" }).click();
  await page.getByRole("button", { name: "开始完形训练", exact: true }).click();
  await expect(page).toHaveURL(/\/practice\/training\/[a-f0-9-]{36}$/);
  await expect(page.getByRole("heading", { name: "A Repair Afternoon on Campus", exact: true })).toBeVisible();
  const sessionID = page.url().split("/").at(-1)!;
  const sessionPath = `/api/v1/practice/training/sessions/${sessionID}`;
  const passage = page.getByRole("region", { name: "完形全文", exact: true });
  await expect(passage).toContainText("When a campus club announced a repair afternoon");
  await expect(passage).toContainText("asking for help was not a sign of weakness");
  await expect(passage.getByRole("button")).toHaveCount(10);
  await expect(page.getByText("参考答案", { exact: false })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "英文原文", exact: true })).toHaveCount(0);
  await expectTouchTargets(page, "完形训练 360px", { within: "[aria-label='空格导航'], [aria-label='完形全文'], main form" });
  expect(await contrastViolations(page, "main")).toEqual([]);
  // Stable blank IDs and native radio keys preserve passage context at 360px.
  await passage.getByRole("button", { name: "第 2 空，未选择", exact: true }).focus();
  await page.keyboard.press("Enter");
  const second = page.getByRole("group", { name: /选择第 2 空/ });
  await expect(second).toBeFocused();
  await second.getByRole("radio").first().focus();
  await page.keyboard.press("ArrowRight");
  await expect(second.getByRole("radio").nth(1)).toBeChecked();
  await page.getByRole("region", { name: "分层提示", exact: true }).getByText("提示列表", { exact: false }).click();
  let hintCommands = 0;
  await page.route("**/api/v1/practice/training/sessions/*/hints", async route => {
    hintCommands++;
    if (hintCommands === 1) { expect((await route.fetch()).status()).toBe(200); await route.abort("failed"); }
    else await route.continue();
  });
  await page.getByRole("button", { name: "查看提示 1 第 1 层", exact: true }).click();
  await expect(page.getByText("观察空格后的 throwing", { exact: false })).toBeVisible();
  expect(hintCommands).toBe(1);
  await page.getByRole("button", { name: "查看提示 1 第 2 层", exact: true }).click();
  await expect(page.getByText("前后是两种处理旧灯的方式", { exact: false })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("cloze-hints-360.png") });
  await page.reload();
  await page.getByRole("region", { name: "分层提示", exact: true }).getByText("提示列表", { exact: false }).click();
  await expect(page.getByText("前后是两种处理旧灯的方式", { exact: false })).toBeVisible();
  expect(hintCommands).toBe(2);
  const before = await page.evaluate(async path => (await fetch(path)).json(), sessionPath);
  expect(before.data.hint_disclosures).toHaveLength(2);
  expect(before.data.hint_disclosures[0].affected_question_ids).toHaveLength(10);
  const blocked = await page.evaluate(async path => (await fetch(`${path}/review`)).status, sessionPath);
  expect(blocked).toBe(409);
  for (const [index, answer] of [1, 2, 0, 3, 1, 0, 2, 3, 0, 0].entries()) {
    await page.getByRole("button", { name: new RegExp(`^第 ${index + 1} 题，`) }).click();
    await page.getByRole("group").getByRole("radio").nth(answer).check();
  }
  await page.screenshot({ path: testInfo.outputPath("cloze-options-360.png") });
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("cloze-context-360.png") });
  const answers: { question: string; key: string; body: string }[] = [];
  let lostAnswer = false;
  await page.route("**/api/v1/practice/training/sessions/*/answers", async route => {
    answers.push({ question: route.request().postDataJSON().question_id, key: route.request().headers()["idempotency-key"], body: route.request().postData()! });
    if (!lostAnswer) { lostAnswer = true; expect((await route.fetch()).status()).toBe(200); await route.abort("failed"); }
    else await route.continue();
  });
  await page.getByRole("button", { name: "提交整组答案", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/10 题" })).toBeVisible();
  await expect(page.getByRole("button", { name: "继续保存答案", exact: true })).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath("cloze-partial-save-360.png") });
  await expect(page.getByText("参考答案", { exact: false })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/10 题" })).toBeVisible();
  await page.getByRole("button", { name: "继续保存答案", exact: true }).click();
  await expect(page.getByRole("region", { name: "训练结果", exact: true })).toBeFocused();
  await expect(page.getByText("独立答对 0 题 · 提示后答对 9 题 · 答错 1 题", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("cloze-result-360.png") });
  await expect(page.getByText("需要独立复习 10 题（含提示后答对的题目）。", { exact: true })).toBeVisible();
  expect(answers).toHaveLength(10);
  expect(new Set(answers.map(answer => answer.question)).size).toBe(10);
  await expect(page.getByRole("region", { name: "本题语境依据", exact: true })).toBeVisible();
  await expect(page.getByText("分类是作者对本题考点的说明，供复习参考。", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "查看英文原文", exact: true }).click();
  await expect(page.getByRole("region", { name: "英文原文", exact: true })).toContainText("instead of throwing it away");
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "查看中文译文", exact: true }).click();
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toContainText("独立解决问题");
  const completed = await page.evaluate(async path => (await fetch(path)).json(), sessionPath);
  expect(completed.data.saved_count).toBe(10);
  expect(completed.data.receipts.every((receipt: { assisted: boolean }) => receipt.assisted)).toBe(true);
  expect(completed.data.revealed_layers).toEqual(["english", "chinese"]);
  const foreign = await context.browser()!.newContext();
  expect((await foreign.request.get(new URL(sessionPath, page.url()).href)).status()).toBe(404);
  await foreign.close();
  await page.reload();
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.getByRole("link", { name: "查看待复习训练", exact: true }).click();
  await expect(page.getByText("完形 · 10 题需要独立复习", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "打开完整复盘", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/practice/training/${sessionID}$`));
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("cloze-context-wide.png"), fullPage: true });
  await page.getByRole("link", { name: "返回训练目录", exact: true }).click();
  await page.getByRole("button", { name: "开始完形训练", exact: true }).click();
  expect(page.url().split("/").at(-1)).not.toBe(sessionID);
  await expect(page.getByRole("status").filter({ hasText: "已保存 0/10 题" })).toBeVisible();
});

// Product data is owned by a real Core/PostgreSQL behind the real signed Gateway.
// Only transport loss is injected; no catalog/session/answer/review is fabricated.
test("real listening stack recovers a lost response, saves four first answers and keeps review private", async ({ page, context }, testInfo) => {
  test.setTimeout(90_000);
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  const trainingResponses: Array<{ path: string; status: number; body: string }> = [];
  page.on("response", async response => {
    if (response.url().includes("/api/v1/practice/training/")) {
      try { trainingResponses.push({ path: new URL(response.url()).pathname, status: response.status(), body: await response.text() }); } catch { /* Intentional transport failure has no browser response. */ }
    }
  });
  await page.goto("/practice");
  await page.getByRole("link", { name: "英语听力与完形训练" }).click();
  await page.getByRole("button", { name: "开始听力训练" }).click();
  await expect(page).toHaveURL(/\/practice\/training\/[a-f0-9-]{36}$/);
  await expect(page.getByRole("heading", { name: "The North Wind and the Sun", exact: true })).toBeVisible();
  const sessionID = page.url().split("/").at(-1)!;
  const audio = page.locator("audio");
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).readyState)).toBeGreaterThanOrEqual(2);
  expect(await audio.evaluate(node => (node as HTMLAudioElement).duration)).toBeCloseTo(36.426848, 2);
  await audio.evaluate(node => (node as HTMLAudioElement).play());
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).currentTime)).toBeGreaterThan(0.2);
  await audio.evaluate(node => (node as HTMLAudioElement).pause());
  const mediaURL = await audio.getAttribute("src");
  const actualDigest = await page.evaluate(async url => { const bytes = await (await fetch(url)).arrayBuffer(); const digest = await crypto.subtle.digest("SHA-256", bytes); return Array.from(new Uint8Array(digest), value => value.toString(16).padStart(2, "0")).join(""); }, mediaURL!);
  expect(actualDigest).toBe("92aa98d9bf83c7f1532dcd2ae2169190b649b5214c13481405ce68d14a630336");
  const range = await page.request.get(mediaURL!, { headers: { Range: "bytes=100-199" } });
  expect(range.status()).toBe(206);expect(range.headers()["content-range"]).toBe("bytes 100-199/584180");
  await expect(page.getByText("参考答案", { exact: false })).toHaveCount(0);
  const groups = page.getByRole("group");
  for (const [index, answer] of [1, 2, 0, 0].entries()) await groups.nth(index).getByRole("radio").nth(answer).check();
  let lost = false;
  await page.route("**/api/v1/practice/training/sessions/*/answers", async route => {
    if (!lost) {
      lost = true;
      const response = await route.fetch();
      expect(response.status()).toBe(200);
      await route.abort("failed");
    } else await route.continue();
  });
  await page.getByRole("button", { name: "提交整组答案" }).click();
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/4 题" })).toBeVisible();
  await expect(page.getByText("部分答案可能尚未保存", { exact: false })).toBeVisible();
  expect(trainingResponses.some(response => response.path.endsWith("/review"))).toBe(false);
  const partial = await page.evaluate(async url => { const response = await fetch(url); return { status: response.status, body: await response.json() }; }, `/api/v1/practice/training/sessions/${sessionID}`);
  expect(partial.status).toBe(200);
  const partialBody = partial.body;
  expect(partialBody.data.saved_count).toBe(1);
  expect(partialBody.data.receipts[0].replayed).toBe(true);
  for (const field of ["expected_answer", "analysis", "evidence", "transcript", "The North Wind and the Sun were disputing"]) expect(JSON.stringify(partialBody)).not.toContain(field);
  const blocked = await page.evaluate(async url => (await fetch(url)).status, `/api/v1/practice/training/sessions/${sessionID}/review`);
  expect(blocked).toBe(409);
  await page.reload();
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/4 题" })).toBeVisible();
  await page.getByRole("button", { name: "继续保存答案" }).click();
  await expect(page.getByRole("heading", { name: "本组训练已完成" })).toBeVisible();
  await expect(page.getByRole("region", { name: "训练结果" })).toBeFocused();
  await expect(page.getByRole("region", { name: "训练结果" })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: testInfo.outputPath("training-stack-result-focus-360.png") });
  await expect(page.getByText("参考答案", { exact: false })).toHaveCount(4);
  await expect(page.getByText("独立答对 3 题 · 提示后答对 0 题 · 答错 1 题", { exact: true })).toBeVisible();
  const completedBody = await page.evaluate(async url => (await fetch(url)).json(), `/api/v1/practice/training/sessions/${sessionID}`);expect(completedBody.data.saved_count).toBe(4);expect(completedBody.data.receipts).toHaveLength(4);
  const review = await page.evaluate(async url => { const response = await fetch(url); return { status: response.status, body: await response.json() }; }, `/api/v1/practice/training/sessions/${sessionID}/review`);
  expect(review.status).toBe(200);expect(review.body.data.layers).toEqual([]);
  // These are real authored sentence ranges from the pinned Core review. A
  // high-frequency boundary timer must wrap before the next sentence can sound.
  await page.getByLabel("播放速度", { exact: true }).selectOption("2");
  expect(await audio.evaluate(node => (node as HTMLAudioElement).playbackRate)).toBe(2);
  await page.getByRole("button", { name: "播放第 4 句", exact: true }).click();
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).currentTime)).toBeGreaterThanOrEqual(26.35);
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).paused)).toBe(false);
  await page.getByRole("button", { name: "循环第 4 句", exact: true }).click();
  await audio.evaluate(node => { (node as HTMLAudioElement).currentTime = 31.40; });
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).currentTime)).toBeLessThan(27.2);
  const boundary = await audio.evaluate(async node => {
    const media = node as HTMLAudioElement;
    media.currentTime = 31.40;
    const samples: number[] = [];
    await new Promise<void>(resolve => {
      const timer = window.setInterval(() => {
        samples.push(media.currentTime);
        if (samples.length >= 24) { window.clearInterval(timer); resolve(); }
      }, 10);
    });
    return Math.max(...samples);
  });
  expect(boundary).toBeLessThan(31.68); // Next sentence starts at 31.690s.
  await page.getByRole("button", { name: "循环第 1 句", exact: true }).click();
  await audio.evaluate(node => { (node as HTMLAudioElement).currentTime = 6.70; });
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).currentTime)).toBeLessThan(0.8);
  await page.getByRole("button", { name: "停止单句循环", exact: true }).click();
  await audio.evaluate(node => (node as HTMLAudioElement).pause());
  // Lose the first committed reveal response, then recover only Core facts.
  let lostReveal = false;
  await page.route("**/api/v1/practice/training/sessions/*/reveals", async route => {
    if (!lostReveal) {
      lostReveal = true;
      const response = await route.fetch();
      expect(response.status()).toBe(200);
      await route.abort("failed");
    } else await route.continue();
  });
  await page.getByRole("button", { name: "查看英文原文", exact: true }).click();
  await expect(page.getByRole("region", { name: "英文原文", exact: true })).toContainText("The North Wind and the Sun were disputing");
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "查看中文译文", exact: true }).click();
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  const revealedBody = await page.evaluate(async url => (await fetch(url)).json(), `/api/v1/practice/training/sessions/${sessionID}`);
  expect(revealedBody.data.revealed_layers).toEqual(["english", "chinese"]);
  expect(revealedBody.data.receipts).toEqual(completedBody.data.receipts);
  const fresh = await context.browser()!.newContext();
  const foreign = await fresh.request.get(new URL(`/api/v1/practice/training/sessions/${sessionID}`, page.url()).href);
  expect(foreign.status()).toBe(404);
  const foreignReveal = await fresh.request.post(new URL(`/api/v1/practice/training/sessions/${sessionID}/reveals`, page.url()).href, { headers: { "Idempotency-Key": "foreign-reveal-isolation-0001" }, data: { unit_version_id: completedBody.data.unit.unit_version_id, layer: "english" } });
  expect(foreignReveal.status()).toBe(404);await fresh.close();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
  await page.screenshot({ path: testInfo.outputPath("training-stack-360.png") });
  await page.reload();
  await expect(page.getByRole("heading", { name: "本组训练已完成" })).toBeVisible();
  await expect(page.getByRole("region", { name: "英文原文", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  // A missing public MP3 is an independent media failure. Saved answers and
  // authorized text still restore through Core, and retry reloads actual media.
  await page.route("**/practice-media/*.mp3", route => route.fulfill({ status: 404, body: "" }));
  await page.reload();
  await expect(page.getByRole("alert").filter({ hasText: "音频暂时无法播放" })).toBeVisible();
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  await page.unroute("**/practice-media/*.mp3");
  await page.getByRole("region", { name: "听力音频", exact: true }).getByRole("button", { name: "重试", exact: true }).click();
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).readyState)).toBeGreaterThanOrEqual(2);
  const mediaRecovered = await page.evaluate(async url => (await fetch(url)).json(), `/api/v1/practice/training/sessions/${sessionID}`);
  expect(mediaRecovered.data.receipts).toEqual(completedBody.data.receipts);
  await page.getByRole("link", { name: "查看待复习训练", exact: true }).click();
  await expect(page.getByRole("heading", { name: "待复习训练", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "打开完整复盘", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/practice/training/${sessionID}$`));
  await expect(page.getByRole("region", { name: "中文译文", exact: true })).toBeVisible();
  await expect(page.getByText("独立答对 3 题 · 提示后答对 0 题 · 答错 1 题", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: testInfo.outputPath("training-stack-wide.png") });
  await page.getByRole("button", { name: "循环第 4 句", exact: true }).click();
  await audio.evaluate(node => { (window as unknown as { previousTrainingAudio: HTMLAudioElement }).previousTrainingAudio = node as HTMLAudioElement; });
  await page.getByRole("link", { name: "返回听力目录", exact: true }).click();
  await page.getByRole("button", { name: "开始听力训练", exact: true }).click();
  await expect(page).toHaveURL(/\/practice\/training\/[a-f0-9-]{36}$/);
  expect(page.url().split("/").at(-1)).not.toBe(sessionID);
  await expect.poll(() => page.evaluate(() => {
    const prior = (window as unknown as { previousTrainingAudio?: HTMLAudioElement }).previousTrainingAudio;
    return !prior || prior.paused && !prior.getAttribute("src");
  })).toBe(true);
  await expect(page.getByRole("status").filter({ hasText: "已保存 0/4 题" })).toBeVisible();
  await testInfo.attach("training-network", { body: JSON.stringify(trainingResponses, null, 2), contentType: "application/json" });
});

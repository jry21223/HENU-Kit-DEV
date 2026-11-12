import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import type { TrainingSession } from "../src/lib/api/generated/training";

const sessionID = "22222222-2222-4222-8222-222222222222";
const unitID = "33333333-3333-4333-8333-333333333333";
const versionID = "44444444-4444-4444-8444-444444444444";
const digest = "92aa98d9bf83c7f1532dcd2ae2169190b649b5214c13481405ce68d14a630336";
const mediaURL = `/practice-media/${digest}.mp3`;
const questions = Array.from({ length: 4 }, (_, index) => ({ question_id: `55555555-5555-4555-8555-55555555555${index}`, question_version_id: `66666666-6666-4666-8666-66666666666${index}`, content: `Listening question ${index + 1}`, options: ["First choice", "Second choice"] }));
const unit = { unit_id: unitID, unit_version_id: versionID, bank_id: unitID, bank_version_id: versionID, kind: "listening" as const, content_sha256: digest, title: "The North Wind and the Sun", attributions: [{ component: "audio" as const, original: false, source_url: "https://commons.wikimedia.org/w/index.php?title=File:Recording_of_speaker_of_British_English_(Received_Pronunciation).ogg&oldid=1067153391", license_id: "CC-BY-SA-3.0", license_url: "https://creativecommons.org/licenses/by-sa/3.0/", attribution: "P. Roach / International Phonetic Association (2003)", modifications: "Converted to MP3" }], questions, media: { asset_id: unitID, url: mediaURL, sha256: digest, content_type: "audio/mpeg" as const, duration_ms: 36427 } };
const session = (): TrainingSession => ({ session_id: sessionID, unit, phase: "pending", saved_count: 0, total_count: 4, receipts: [], hint_disclosures: [], revealed_layers: [], recovery_scope: "browser" });

async function ownerFixture(page: Page) {
  let current = session();
  await page.route("**/api/v1/session", route => route.fulfill({ status: 401, json: { error: "unauthenticated" } }));
  await page.route("**/api/v1/practice/catalog", route => route.fulfill({ json: { request_id: "catalog", banks: [] } }));
  await page.route("**/api/v1/practice/training/catalog", route => route.fulfill({ json: { request_id: "catalog", data: { units: [{ unit_id: unit.unit_id, unit_version_id: unit.unit_version_id, bank_id: unit.bank_id, bank_version_id: unit.bank_version_id, kind: unit.kind, content_sha256: unit.content_sha256, title: unit.title, attributions: unit.attributions, question_count: 4 }] } } }));
  await page.route("**/api/v1/practice/training/sessions", route => route.fulfill({ status: 201, json: { request_id: "create", data: current } }));
  await page.route(`**/api/v1/practice/training/sessions/${sessionID}`, route => route.fulfill({ json: { request_id: "recover", data: current } }));
  const audio = await readFile(path.join(__dirname, "fixtures/training/north-wind.mp3"));
  await page.route(`**${mediaURL}`, async route => {
    const range = route.request().headers().range;
    const start = Number(range?.match(/bytes=(\d+)-/)?.[1] || 0);
    const headers = { "Content-Type": "audio/mpeg", "Accept-Ranges": "bytes", "Content-Length": String(audio.length - start), ...(range ? { "Content-Range": `bytes ${start}-${audio.length - 1}/${audio.length}` } : {}) };
    await route.fulfill({ status: range ? 206 : 200, headers, body: audio.subarray(start) });
  });
  return { get: () => current, set: (value: TrainingSession) => { current = value; } };
}

test("Practice discovers listening, starts a whole unit, and plays actual MP3 audio", async ({ page }) => {
  await ownerFixture(page);
  const mediaStatuses: number[] = [];
  page.on("response", response => { if (response.url().endsWith(mediaURL)) mediaStatuses.push(response.status()); });
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("/practice");
  await page.getByRole("link", { name: "英语听力训练" }).click();
  await page.getByRole("button", { name: "开始听力训练" }).click();
  await expect(page).toHaveURL(new RegExp(`/practice/training/${sessionID}$`));
  await expect(page.getByRole("heading", { name: unit.title })).toBeVisible();
  await expect(page.getByRole("group", { name: /Listening question/ })).toHaveCount(4);
  const audio = page.locator("audio");
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).readyState)).toBeGreaterThanOrEqual(2);
  expect(await audio.evaluate(node => (node as HTMLAudioElement).duration)).toBeCloseTo(36.426848, 1);
  await audio.evaluate(node => (node as HTMLAudioElement).play());
  await expect.poll(() => audio.evaluate(node => (node as HTMLAudioElement).currentTime)).toBeGreaterThan(0.1);
  await audio.evaluate(node => (node as HTMLAudioElement).pause());
  expect(mediaStatuses).toContain(206);
  await expect(page.getByText("参考答案", { exact: false })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
});

test("an uncertain start survives refresh with the original create command key", async ({ page }) => {
  await ownerFixture(page);
  const keys: string[] = [];
  await page.route("**/api/v1/practice/training/sessions", async route => {
    keys.push(route.request().headers()["idempotency-key"]);
    await route.fulfill(keys.length === 1 ? { status: 503, json: { request_id: "lost-start", error: { code: "TRAINING_DISABLED", message: "unavailable" } } } : { status: 201, json: { request_id: "replay", data: session() } });
  });
  await page.goto("/practice/training");
  await page.getByRole("button", { name: "开始听力训练" }).click();
  await expect(page.getByText("训练暂时无法开始，请重试。重试会继续这次操作。", { exact: true })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "开始听力训练" }).click();
  await expect(page.getByRole("heading", { name: unit.title })).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[1]).toBe(keys[0]);
});

test("partial save recovers after refresh, retries only missing answers, and unlocks review after all four", async ({ page }, testInfo) => {
  const fixture = await ownerFixture(page);
  const bodies: Array<{ body: unknown; key: string }> = [];
  let failed = false;
  let reviewCalls = 0;
  await page.route(`**/api/v1/practice/training/sessions/${sessionID}/answers`, async route => {
    const body = route.request().postDataJSON();
    bodies.push({ body, key: route.request().headers()["idempotency-key"] });
    if (body.question_id === questions[1].question_id && !failed) {
      failed = true;
      await route.fulfill({ status: 503, json: { request_id: "answer-failed", error: { code: "TRAINING_DISABLED", message: "unavailable" } } });
      return;
    }
    const current = fixture.get();
    const count = current.saved_count + 1;
    const receipt = { session_id: sessionID, unit_version_id: versionID, question_id: body.question_id, question_version_id: body.question_version_id, answer: body.answer, correct: body.question_id !== questions[3].question_id, assisted: false, outcome: body.question_id === questions[3].question_id ? "review_required" as const : "independent_correct" as const, review_required: body.question_id === questions[3].question_id, submitted_at: "2026-11-12T12:00:00Z", replayed: false, saved_count: count, total_count: 4, phase: count === 4 ? "completed" as const : "partial" as const };
    fixture.set({ ...current, saved_count: count, phase: receipt.phase, receipts: [...current.receipts, receipt].map(item => ({ ...item, saved_count: count, phase: receipt.phase, replayed: true })) });
    await route.fulfill({ json: { request_id: "answer", data: receipt } });
  });
  await page.route(`**/api/v1/practice/training/sessions/${sessionID}/review`, async route => {
    reviewCalls++;
    expect(fixture.get().saved_count).toBe(4);
    await route.fulfill({ json: { request_id: "review", data: { session_id: sessionID, unit_version_id: versionID, kind: "listening", questions: questions.map(question => ({ question_id: question.question_id, question_version_id: question.question_version_id, expected_answer: 0, analysis: "来自已完成训练的解析。", error_category: "detail", evidence: { kind: "listening", segment_ids: ["sentence-1"] } })), segments: [{ segment_id: "sentence-1", start_ms: 0, end_ms: 6898 }], layers: [] } } });
  });
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(`/practice/training/${sessionID}`);
  for (const group of await page.getByRole("group", { name: /Listening question/ }).all()) {
    const radio = group.getByRole("radio").first();
    await radio.focus();
    await page.keyboard.press("Space");
  }
  await page.getByRole("button", { name: "提交整组答案" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/4 题" })).toBeVisible();
  await expect(page.getByText(/部分答案可能尚未保存/)).toBeVisible();
  await expect(page.getByRole("alert").filter({ hasText: "部分答案可能尚未保存" })).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "继续保存答案" })).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath("listening-retry-feedback-360.png") });
  expect(reviewCalls).toBe(0);
  await expect(page.getByText(/参考答案/)).toHaveCount(0);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("listening-partial-360.png"), fullPage: true });
  await page.reload();
  await expect(page.getByRole("status").filter({ hasText: "已保存 1/4 题" })).toBeVisible();
  await expect(page.getByRole("radio").nth(2)).toBeChecked();
  await page.getByRole("button", { name: "继续保存答案" }).click();
  await expect(page.getByRole("heading", { name: "本组训练已完成" })).toBeVisible();
  await expect(page.getByRole("region", { name: "训练结果" })).toBeFocused();
  await expect(page.getByRole("region", { name: "训练结果" })).toBeInViewport({ ratio: 1 });
  await expect(page.getByText("来自已完成训练的解析。", { exact: true })).toHaveCount(4);
  await expect(page.getByText("独立答对 3 题 · 提示后答对 0 题 · 需要复习 1 题", { exact: true })).toBeVisible();
  expect(bodies).toHaveLength(5);
  expect(bodies[2]).toEqual(bodies[1]);
  expect(bodies.filter(item => (item.body as { question_id: string }).question_id === questions[0].question_id)).toHaveLength(1);
  expect(reviewCalls).toBe(1);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("listening-complete-360.png"), fullPage: true });
  await page.reload();
  await expect(page.getByRole("heading", { name: "本组训练已完成" })).toBeVisible();
  expect(bodies).toHaveLength(5);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("listening-complete-wide.png"), fullPage: true });
});

test("audio failure has an honest retry and preserves the selected answer", async ({ page }) => {
  await ownerFixture(page);
  const failAudio = async (route: import("@playwright/test").Route) => route.abort();
  await page.route(`**${mediaURL}`, failAudio);
  await page.goto(`/practice/training/${sessionID}`);
  await expect(page.getByText("音频暂时无法播放，请检查网络后重试。", { exact: true })).toBeVisible();
  await page.getByRole("group", { name: /Listening question 1/ }).getByRole("radio").first().check();
  await page.unroute(`**${mediaURL}`, failAudio);
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect.poll(() => page.locator("audio").evaluate(node => (node as HTMLAudioElement).readyState)).toBeGreaterThanOrEqual(2);
  await expect(page.getByText("音频暂时无法播放，请检查网络后重试。", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("group", { name: /Listening question 1/ }).getByRole("radio").first()).toBeChecked();
  await expect(page.getByRole("button", { name: "提交整组答案" })).toBeDisabled();
});

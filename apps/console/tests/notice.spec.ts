import { expect, test } from "@playwright/test";

const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["notice.read", "notice.manage", "notice.review", "notice.distribute"], scopes: [{ kind: "product", product_code: "notice" }], verified_at: "2026-07-19T00:00:00Z" }, expires_at: "2026-07-19T01:00:00Z" };
let approved = false;
let distributed = false;

function snapshot() { return { items: [{ id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", source: { id: "571f1c6f-7b10-4c92-91a2-b39bf5af5302", code: "henu-office", name: "学校办公室" }, version: 1, title: "暑期安排", body: "不可变正文", source_url: "https://example.edu/notices/1", content_hash: "a".repeat(64), state: distributed ? "distributed" : approved ? "approved" : "pending_review", revision: distributed ? 3 : approved ? 2 : 1, created_at: "2026-07-19T00:00:00Z", distribution_count: distributed ? 1 : 0, ...(distributed ? { distribution_status: "queued" } : {}) }], generated_at: "2026-07-19T00:00:00Z" }; }

test.beforeEach(async ({ page }) => {
  approved = false;
  distributed = false;
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_notice_session" }) }));
  await page.route("**/api/v1/notices", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: snapshot(), request_id: "req_notice_snapshot" }) }));
  await page.route("**/api/v1/notices/sources", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_notice_source_create_/);
    expect(await route.request().postDataJSON()).toEqual({ code: "henu-office-new", name: "新来源", canonical_url: "https://example.edu/new" });
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "671f1c6f-7b10-4c92-91a2-b39bf5af5302" }, request_id: "req_notice_source" }) });
  });
  await page.route("**/api/v1/notices/sources/*/versions", async (route) => {
    expect(route.request().url()).toContain("671f1c6f-7b10-4c92-91a2-b39bf5af5302/versions");
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_notice_version_create_/);
    expect(await route.request().postDataJSON()).toMatchObject({ title: "新通知", body: "新正文", source_url: "https://example.edu/new/1" });
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "771f1c6f-7b10-4c92-91a2-b39bf5af5302", state: "pending_review", revision: 1 }, request_id: "req_notice_version" }) });
  });
  await page.route("**/api/v1/notices/versions/*/reviews", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_notice_review_/);
    expect((await route.request().postDataJSON()).expected_revision).toBe(1);
    await route.abort("connectionreset");
  });
  await page.route("**/api/v1/notices/operations/review", (route) => { approved = true; return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { state: "approved", revision: 2 }, request_id: "req_notice_resolved" }) }); });
  await page.route("**/api/v1/notices/versions/*/distributions", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_notice_distribution_/);
    expect(await route.request().postDataJSON()).toEqual({ channel: "email", audience: { kind: "college", value: "software-college" }, expected_revision: 2 });
    distributed = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { status: "queued", revision: 3 }, request_id: "req_notice_distribution" }) });
  });
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Notice review resolves an unknown write without resubmission`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/notices");
    await expect(page.getByRole("heading", { name: "校园通知审核与分发" })).toBeVisible();
    await expect(page.getByText("不可变正文", { exact: true })).toBeVisible();
    await page.getByLabel("来源代码").fill("henu-office-new");
    await page.getByLabel("来源名称").fill("新来源");
    await page.getByLabel("来源主页").fill("https://example.edu/new");
    await page.getByRole("button", { name: "创建来源" }).click();
    await expect(page.getByRole("status")).toContainText("通知来源已创建");
    await page.getByLabel("标题").fill("新通知");
    await page.getByLabel("正文").fill("新正文");
    await page.getByLabel("原文链接").fill("https://example.edu/new/1");
    await page.getByRole("button", { name: "创建版本" }).click();
    await expect(page.getByRole("status")).toContainText("不可变通知版本已创建");
    await page.getByRole("button", { name: "批准" }).click();
    await expect(page.getByRole("status")).toContainText("审核已批准");
    await expect(page.getByText("已通过 · 版本 v2", { exact: true })).toBeVisible();
    await page.getByLabel("渠道").selectOption("email");
    await page.getByLabel("受众").selectOption("college");
    await page.getByLabel("受众值").fill("software-college");
    await page.getByRole("button", { name: "创建分发任务" }).click();
    await expect(page.getByRole("status")).toContainText("分发任务已创建");
    await expect(page.getByText("已分发 · 版本 v3", { exact: true })).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

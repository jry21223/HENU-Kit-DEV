import { expect, test } from "@playwright/test";

const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["library.read", "library.manage", "library.review"], scopes: [{ kind: "product", product_code: "library" }], verified_at: "2026-07-19T00:00:00Z" }, expires_at: "2026-07-19T01:00:00Z" };
let approved = false;
let submissionCommandCount = 0;

function workspace() {
  const material = { id: "22222222-2222-4222-8222-222222222222", course_id: "11111111-1111-4111-8111-111111111111", title: "期末复习提纲", type: "quick_review", file_name: "review.pdf", file_size: 2048, access_level: "authenticated", status: approved ? "published" : "pending", updated_at: "2026-07-19T00:00:00Z" };
  return { status: "partial", status_message: "资料纠错来源暂不可用，其他数据可用。", degraded: true, courses: [{ id: "11111111-1111-4111-8111-111111111111", name: "高等数学", slug: "math", grade: "2025", status: "published", updated_at: "2026-07-19T00:00:00Z" }], materials: [material], downloads: [{ id: "33333333-3333-4333-8333-333333333333", material_id: material.id, material_title: material.title, access_level: "authenticated", downloaded_at: "2026-07-19T01:00:00Z" }], submissions: approved ? [] : [material], corrections: [], generated_at: "2026-07-19T00:00:00Z" };
}

test.beforeEach(async ({ page }) => {
  approved = false;
  submissionCommandCount = 0;
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_library_session" }) }));
  await page.route("**/api/v1/library", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: workspace(), request_id: "req_library_workspace" }) }));
  await page.route("**/api/v1/library/commands", async (route) => {
    const input = await route.request().postDataJSON();
    if (input.kind === "course_create") {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_library_course_create_/);
      expect(input).toEqual({ kind: "course_create", payload: { schoolId: "55555555-5555-4555-8555-555555555555", collegeId: "66666666-6666-4666-8666-666666666666", majorId: "77777777-7777-4777-8777-777777777777", name: "线性代数", slug: "linear-algebra", grade: "2025", status: "draft" } });
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "course_create", resource_id: "44444444-4444-4444-8444-444444444444", state: "succeeded" }, request_id: "req_library_course_create" }) });
    }
    if (input.kind === "course_update") {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_library_course_update_/);
      expect(input).toEqual({ kind: "course_update", resource_id: "11111111-1111-4111-8111-111111111111", expected_version: "2026-07-19T00:00:00Z", payload: { name: "高等数学 A" } });
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "course_update", resource_id: input.resource_id, state: "succeeded" }, request_id: "req_library_course_update" }) });
    }
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_library_submission_approve_/);
    expect(input).toEqual({ kind: "submission_approve", resource_id: "22222222-2222-4222-8222-222222222222", expected_version: "2026-07-19T00:00:00Z", payload: { reviewReason: "人工核验通过" } });
    submissionCommandCount += 1;
    await new Promise((resolve) => setTimeout(resolve, 150));
    await route.abort("connectionreset");
  });
  await page.route("**/api/v1/library/operations/submission_approve", (route) => {
    approved = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", resource_id: "22222222-2222-4222-8222-222222222222", state: "succeeded" }, request_id: "req_library_resolved" }) });
  });
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Library management and review stay bounded`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/library");
    await expect(page.getByRole("heading", { name: "资料库运营" })).toBeVisible();
    for (const heading of ["课程", "资料", "下载", "投稿审核", "资料纠错"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByRole("status")).toContainText("资料纠错来源暂不可用");
    for (const excluded of ["社区", "支付", "刷题", "积分", "会员"]) await expect(page.getByText(excluded, { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "创建课程" })).toBeVisible();
    await expect(page.getByRole("button", { name: "创建资料" })).toBeVisible();
    await page.getByLabel("学校标识").fill("55555555-5555-4555-8555-555555555555");
    await page.getByLabel("学院标识").fill("66666666-6666-4666-8666-666666666666");
    await page.getByLabel("专业标识").fill("77777777-7777-4777-8777-777777777777");
    await page.getByLabel("课程名称", { exact: true }).fill("线性代数");
    await page.getByLabel("课程标识").fill("linear-algebra");
    await page.getByLabel("年级").fill("2025");
    await page.getByRole("button", { name: "创建课程" }).click();
    await expect(page.getByText("课程已创建。", { exact: true })).toBeVisible();
    await page.getByLabel("编辑课程名称").fill("高等数学 A");
    await page.getByRole("button", { name: "保存课程" }).click();
    await expect(page.getByText("课程已更新。", { exact: true })).toBeVisible();

    // 审核：理由与确认在同一单步面板内提交
    await expect(page.getByText("快速复习")).toHaveCount(2);
    await page.getByRole("button", { name: "批准投稿" }).click();
    await page.getByLabel("审核理由").fill("人工核验通过");
    const confirmApprove = page.getByRole("button", { name: "确认批准" });
    await confirmApprove.click();

    // 请求飞行期间按钮禁用，重复触发不会发出第二条命令
    await expect(confirmApprove).toBeDisabled();
    await expect(page.getByRole("button", { name: "归档课程" })).toBeDisabled();
    await confirmApprove.click({ force: true });
    await expect(page.getByText("投稿已批准。", { exact: true })).toBeVisible();
    expect(submissionCommandCount).toBe(1);
    await expect(page.getByRole("button", { name: "批准投稿" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "归档课程" })).toBeEnabled();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Library review retry reuses the original idempotency key", async ({ page }) => {
  const keys: string[] = [];
  await page.route("**/api/v1/library/commands", async (route) => {
    const input = await route.request().postDataJSON();
    expect(input.kind).toBe("submission_approve");
    keys.push(route.request().headers()["idempotency-key"] ?? "");
    if (keys.length === 1) return route.abort("connectionreset");
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", resource_id: "22222222-2222-4222-8222-222222222222", state: "succeeded" }, request_id: "req_library_retry" }) });
  });
  await page.route("**/api/v1/library/operations/submission_approve", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", state: "unknown" }, request_id: "req_library_still_unknown" }) }));
  await page.goto("/library");
  await page.getByRole("button", { name: "批准投稿" }).click();
  await page.getByLabel("审核理由").fill("人工核验通过");
  await page.getByRole("button", { name: "确认批准" }).click();
  await expect(page.getByRole("button", { name: "按原请求重试" })).toBeVisible();
  await page.getByRole("button", { name: "按原请求重试" }).click();
  await expect(page.getByText("投稿已批准。", { exact: true })).toBeVisible();
  expect(keys.length).toBe(2);
  expect(keys[0]).toBeTruthy();
  expect(keys[1]).toBe(keys[0]);
});

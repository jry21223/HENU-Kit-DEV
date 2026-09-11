import { expect, test } from "@playwright/test";

const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["food.read", "food.review", "food.anomaly", "food.tier_adjust"], scopes: [{ kind: "product", product_code: "food" }], verified_at: "2026-07-20T00:00:00Z" }, expires_at: "2026-07-20T01:00:00Z" };
let approved = false;
let anomalyResolved = false;
let tierConfirmed = false;
let submissionEdited = false;
let postEdited = false;
let lookupCalls = 0;
let submissionCommandCount = 0;

function workspace() {
  return { status: "stale", status_message: "Food 数据可能已过期", stale: true, as_of: "2026-07-19T22:00:00Z",
    submissions: approved ? [] : [{ id: "11111111-1111-4111-8111-111111111111", venue_name: submissionEdited ? "北苑餐厅（一餐）" : "北苑餐厅", item_name: "胡辣汤", description: "早餐窗口", campus: submissionEdited ? "minglun" : null, status: "pending", version: submissionEdited ? 2 : 1, submitted_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }],
    anomaly_tickets: anomalyResolved ? [] : [{ id: "22222222-2222-4222-8222-222222222222", venue_name: "北苑餐厅", kind: "duplicate", details: "重复地点", severity: "medium", status: "open", version: 1, created_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }],
    tier_adjustments: tierConfirmed ? [] : [{ id: "33333333-3333-4333-8333-333333333333", venue_name: "北苑餐厅", current_tier: "standard", proposed_tier: "recommended", reason: "近期评分稳定", status: "pending", version: 1, created_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }],
    // Frozen legacy Food rows use deterministic md5-derived PostgreSQL UUIDs.
    // Their canonical 8-4-4-4-12 shape is valid for the owner and Go clients,
    // even though the hash does not carry RFC version/variant marker bits.
    posts: [{ id: "01234567-89ab-cdef-0123-456789abcdef", venue_name: postEdited ? "南苑餐厅（一餐）" : "南苑餐厅", campus: postEdited ? "minglun" : "jinming", tier: postEdited ? "top" : "hang", review_text: "学生推荐", price_reference: "¥12", hours_reference: "10:00-20:00", author_display_name: "张三", hidden: postEdited, version: postEdited ? 2 : 1, created_at: "2026-07-19T10:00:00Z", updated_at: "2026-07-19T10:00:00Z" }] };
}

test.beforeEach(async ({ page }) => {
  approved = anomalyResolved = tierConfirmed = submissionEdited = postEdited = false;
  lookupCalls = 0;
  submissionCommandCount = 0;
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_food_session" }) }));
  await page.route(new RegExp("^.*/api/v1/food(\\?.*)?$"), (route) => {
    const campus = new URL(route.request().url()).searchParams.get("campus") ?? "";
    let data = workspace();
    if (campus) {
      data = { ...data, submissions: data.submissions.filter((item) => item.campus === campus), posts: data.posts.filter((item) => item.campus === campus) };
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data, request_id: "req_food_workspace" }) });
  });
  await page.route("**/api/v1/food/commands", async (route) => {
    const input = await route.request().postDataJSON();
    expect(route.request().headers()["idempotency-key"]).toMatch(new RegExp(`^idem_food_${input.kind}_`));
    expect(input.payload.note.length).toBeGreaterThan(1);
    if (input.kind === "submission_approve") {
      expect(input.payload.note).toBe("人工核验通过");
      submissionCommandCount += 1;
      await new Promise((resolve) => setTimeout(resolve, 150));
      return route.abort("connectionreset");
    }
    if (input.kind === "submission_edit") {
      expect(input.payload.venue_name).toBe("北苑餐厅（一餐）");
      expect(input.payload.campus).toBe("minglun");
      expect(input.payload.note).toBe("修正店名并分配校区");
      submissionEdited = true;
    }
    if (input.kind === "post_edit") {
      expect(input.payload.campus).toBe("minglun");
      expect(input.payload.tier).toBe("top");
      expect(input.payload.hidden).toBe(true);
      expect(input.payload.note).toBe("修正校区档位并隐藏");
      postEdited = true;
    }
    if (input.kind === "anomaly_resolve") anomalyResolved = true;
    if (input.kind === "tier_adjustment_confirm") tierConfirmed = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: input.kind, resource_id: input.resource_id, state: "succeeded", version: 2 }, request_id: "req_food_command" }) });
  });
  await page.route("**/api/v1/food/operations/submission_approve", (route) => {
    lookupCalls++;
    if (lookupCalls === 1) return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", state: "unknown" }, request_id: "req_food_still_unknown" }) });
    approved = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", resource_id: "11111111-1111-4111-8111-111111111111", state: "succeeded", version: 2 }, request_id: "req_food_resolved" }) });
  });
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Food operations resolve unknown writes and stay responsive`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/food");
    await expect(page.getByRole("heading", { name: "美食运营" })).toBeVisible();
    for (const heading of ["投稿审核", "异常票处理", "调档确认"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByRole("status")).toContainText("Food 数据可能已过期");
    await expect(page.getByText("标准 → 推荐")).toHaveCount(1);

    // 投稿：理由与确认在同一单步面板内提交；请求飞行期间按钮禁用，重复触发不会发出第二条命令
    await page.getByRole("button", { name: "批准投稿" }).click();
    await page.getByLabel("操作理由").fill("人工核验通过");
    const confirmApprove = page.getByRole("button", { name: "确认批准投稿" });
    await confirmApprove.click();
    await expect(confirmApprove).toBeDisabled();
    await expect(page.getByRole("button", { name: "标记已处理" })).toBeDisabled();
    await confirmApprove.click({ force: true });
    await expect(page.getByText("投稿已批准。", { exact: true })).toBeVisible();
    expect(submissionCommandCount).toBe(1);

    await page.getByRole("button", { name: "标记已处理" }).click();
    await page.getByLabel("操作理由").fill("已到现场复核");
    await page.getByRole("button", { name: "确认标记已处理" }).click();
    await expect(page.getByText("异常票已处理。", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: "确认调档" }).click();
    await page.getByLabel("操作理由").fill("档位调整依据充分");
    await page.getByRole("button", { name: "确认调档" }).click();
    await expect(page.getByText("调档已确认。", { exact: true })).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Food review retry reuses the original idempotency key", async ({ page }) => {
  const keys: string[] = [];
  await page.route("**/api/v1/food/commands", async (route) => {
    const input = await route.request().postDataJSON();
    expect(input.kind).toBe("submission_approve");
    keys.push(route.request().headers()["idempotency-key"] ?? "");
    if (keys.length === 1) return route.abort("connectionreset");
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", resource_id: "11111111-1111-4111-8111-111111111111", state: "succeeded", version: 2 }, request_id: "req_food_retry" }) });
  });
  await page.route("**/api/v1/food/operations/submission_approve", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "submission_approve", state: "unknown" }, request_id: "req_food_still_unknown" }) }));
  await page.goto("/food");
  await page.getByRole("button", { name: "批准投稿" }).click();
  await page.getByLabel("操作理由").fill("人工核验通过");
  await page.getByRole("button", { name: "确认批准投稿" }).click();
  await expect(page.getByRole("button", { name: "按原请求重试" })).toBeVisible();
  await page.getByRole("button", { name: "按原请求重试" }).click();
  await expect(page.getByText("投稿已批准。", { exact: true })).toBeVisible();
  expect(keys.length).toBe(2);
  expect(keys[0]).toBeTruthy();
  expect(keys[1]).toBe(keys[0]);
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Food ops edit a pending submission with campus`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/food");
    await expect(page.getByText("早餐窗口 · 未分配 · 版本 1")).toBeVisible();
    await page.getByRole("button", { name: "编辑投稿" }).click();
    await page.getByLabel("店名").fill("北苑餐厅（一餐）");
    await page.getByLabel("投稿校区").selectOption("minglun");
    await page.getByLabel("操作理由").fill("修正店名并分配校区");
    await page.getByRole("button", { name: "保存修改" }).click();
    await expect(page.getByText("投稿信息已更新。", { exact: true })).toBeVisible();
    await expect(page.getByText("北苑餐厅（一餐） · 胡辣汤")).toBeVisible();
    await expect(page.getByText("早餐窗口 · 明伦 · 版本 2")).toBeVisible();
    await expect(page.locator("body")).not.toContainText(/\bv2\b/i);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Food ops edit a published post with campus tier and hidden`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/food");
    await expect(page.getByText("南苑餐厅 · 金明 · 夯")).toBeVisible();
    await page.getByRole("button", { name: "编辑已发布投稿" }).click();
    await page.getByLabel("投稿校区").selectOption("minglun");
    await page.getByLabel("档位").selectOption("top");
    await page.getByLabel("隐藏此投稿（公开榜单不再展示）").check();
    await page.getByLabel("操作理由").fill("修正校区档位并隐藏");
    await page.getByRole("button", { name: "保存修改" }).click();
    await expect(page.getByText("投稿信息已更新。", { exact: true })).toBeVisible();
    await expect(page.getByText("南苑餐厅（一餐） · 明伦 · 顶级")).toBeVisible();
    await expect(page.getByText("学生推荐 · 张三 · 已隐藏 · 版本 2")).toBeVisible();
    await expect(page.locator("body")).not.toContainText(/\bv2\b/i);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Food ops filter workspace by campus`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/food");
    await expect(page.getByText("早餐窗口 · 未分配 · 版本 1")).toBeVisible();
    await expect(page.getByText("南苑餐厅 · 金明 · 夯")).toBeVisible();
    await page.getByLabel("校区筛选").selectOption("minglun");
    await expect(page.getByText("暂无待审核投稿")).toBeVisible();
    await expect(page.getByText("暂无已发布投稿")).toBeVisible();
    await page.getByLabel("校区筛选").selectOption("jinming");
    await expect(page.getByText("南苑餐厅 · 金明 · 夯")).toBeVisible();
    await expect(page.getByText("暂无待审核投稿")).toBeVisible();
    await page.getByLabel("校区筛选").selectOption("");
    await expect(page.getByText("早餐窗口 · 未分配 · 版本 1")).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

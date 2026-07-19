import { expect, test } from "@playwright/test";

const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["food.read", "food.review", "food.anomaly", "food.tier_adjust"], scopes: [{ kind: "product", product_code: "food" }], verified_at: "2026-07-20T00:00:00Z" }, expires_at: "2026-07-20T01:00:00Z" };
let approved = false;
let anomalyResolved = false;
let tierConfirmed = false;
let lookupCalls = 0;

function workspace() {
  return { status: "stale", status_message: "Food 数据可能已过期", stale: true, as_of: "2026-07-19T22:00:00Z",
    submissions: approved ? [] : [{ id: "11111111-1111-4111-8111-111111111111", venue_name: "北苑餐厅", item_name: "胡辣汤", description: "早餐窗口", status: "pending", version: 1, submitted_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }],
    anomaly_tickets: anomalyResolved ? [] : [{ id: "22222222-2222-4222-8222-222222222222", venue_name: "北苑餐厅", kind: "duplicate", details: "重复地点", severity: "medium", status: "open", version: 1, created_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }],
    tier_adjustments: tierConfirmed ? [] : [{ id: "33333333-3333-4333-8333-333333333333", venue_name: "北苑餐厅", current_tier: "standard", proposed_tier: "recommended", reason: "近期评分稳定", status: "pending", version: 1, created_at: "2026-07-20T00:00:00Z", updated_at: "2026-07-20T00:00:00Z" }] };
}

test.beforeEach(async ({ page }) => {
  approved = anomalyResolved = tierConfirmed = false;
  lookupCalls = 0;
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_food_session" }) }));
  await page.route("**/api/v1/food", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: workspace(), request_id: "req_food_workspace" }) }));
  await page.route("**/api/v1/food/commands", async (route) => {
    const input = await route.request().postDataJSON();
    expect(route.request().headers()["idempotency-key"]).toMatch(new RegExp(`^idem_food_${input.kind}_`));
    expect(input.payload.note.length).toBeGreaterThan(1);
    if (input.kind === "submission_approve") return route.abort("connectionreset");
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
    await expect(page.getByRole("heading", { name: "Food 运营" })).toBeVisible();
    for (const heading of ["投稿审核", "异常票处理", "调档确认"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByRole("status")).toContainText("Food 数据可能已过期");
    await page.getByRole("button", { name: "批准投稿" }).click();
    await expect(page.getByText("投稿已批准。", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "标记已处理" }).click();
    await expect(page.getByText("异常票已处理。", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "确认调档" }).click();
    await expect(page.getByText("调档已确认。", { exact: true })).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

import { expect, test } from "@playwright/test";

const operations = {
  access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
  accounts: [{ id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-07-19T00:00:00Z", grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }] }],
  sessions: [{ id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", kind: "core", last_seen_at: "2026-07-19T00:00:00Z", expires_at: "2026-07-19T01:00:00Z" }],
  mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 0, dead_letters: 0 },
  inbox_items: [{ id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302", source_product_code: "quizcraft", source_resource_type: "submission", source_resource_id: "submission-7", priority: "normal", status: "open", version: 1, created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" }],
  audit: [{ request_id: "req_operations_browser", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "permission_granted", created_at: "2026-07-19T00:00:00Z" }],
  dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-07-19T00:00:00Z",
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 403, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/operations", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: operations, request_id: "req_operations_envelope" }) }));
  await page.route("**/api/v1/operations/sessions/*/revocations", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_console_revoke_/);
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "session_revoke", status: "succeeded", resource_id: operations.sessions[0].id }, request_id: "req_revoke" }) });
  });
  await page.route("**/api/v1/operations/users/*/access-updates", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_console_access_/);
    expect((await route.request().postDataJSON()).grants[0].role_code).toBe("operations-reviewer");
    await route.abort("connectionreset");
  });
  await page.route("**/api/v1/operations/results/access_update", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "succeeded", resource_id: operations.accounts[0].id, resource_version: 2 }, request_id: "req_access_resolved" }) }));
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Platform Operations supports bounded read and Session revocation`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/operations");
    for (const heading of ["平台运营工作台", "账户、角色与 Scope", "Session", "邮件基础设施", "Operations Inbox", "授权审计"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByLabel("角色代码")).toHaveValue("operations-operator");
    await page.getByRole("button", { name: "撤销 Session" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    await page.getByLabel("角色代码").fill("operations-reviewer");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("status")).toContainText("结果未知");
    await page.getByRole("button", { name: "查询结果" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

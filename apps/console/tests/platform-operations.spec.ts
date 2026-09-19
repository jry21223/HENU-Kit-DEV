import { expect, test } from "@playwright/test";

const operations = {
  access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
  accounts: [{ id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-07-19T00:00:00Z", grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }] }],
  sessions: [{ id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", kind: "core", last_seen_at: "2026-07-19T00:00:00Z", expires_at: "2026-07-19T01:00:00Z" }],
  mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 0, dead_letters: 0 },
  inbox_items: [{ id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302", source_product_code: "quizcraft", source_resource_type: "submission", source_resource_id: "submission-7", priority: "normal", status: "open", version: 1, created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" }],
  audit: [{ request_id: "req_operations_browser", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "permission_granted", created_at: "2026-07-19T00:00:00Z" }],
  dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-07-19T00:00:00Z",
};
const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" }, expires_at: "2026-07-19T01:00:00Z" };

test.beforeEach(async ({ page }) => {
  await page.route("https://fonts.googleapis.com/**", (route) => route.abort());
  await page.route("https://fonts.gstatic.com/**", (route) => route.abort());
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_operations_session" }) }));
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
    for (const heading of ["平台运营工作台", "账户、角色与权限", "登录会话", "邮件基础设施", "运营收件箱", "授权审计"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByLabel("角色代码")).toHaveValue("operations-operator");
    await expect(page.getByText("张老师").first()).toBeVisible();
    await expect(page.getByText("very.long.operator.identity@henu.edu.cn").first()).toBeVisible();
    await expect(page.getByText(/171f1c6f-7b10/)).toHaveCount(0);
    await page.getByRole("button", { name: "撤销登录" }).click();
    const revokeDialog = page.getByRole("dialog");
    await expect(revokeDialog).toContainText("张老师");
    await expect(revokeDialog).toContainText("撤销核心登录");
    await revokeDialog.getByRole("button", { name: "确认撤销会话" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    await page.getByLabel("角色代码").fill("operations-reviewer");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    const accessDialog = page.getByRole("dialog");
    await expect(accessDialog).toContainText("授权集合");
    await accessDialog.getByRole("button", { name: "确认更新访问设置" }).click();
    await expect(page.getByRole("status")).toContainText("结果尚未确认");
    await page.getByRole("button", { name: "查询结果" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} marking an account deleted requires confirmation and cancel performs no write`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const requests: Array<{ idempotencyKey?: string; status?: string }> = [];
    await page.route("**/api/v1/operations/users/*/access-updates", async (route) => {
      const body = await route.request().postDataJSON();
      requests.push({ idempotencyKey: route.request().headers()["idempotency-key"], status: body.status });
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "succeeded", resource_id: operations.accounts[0].id, resource_version: 2 }, request_id: "req_access_deleted" }) });
    });
    await page.goto("/operations");
    await expect(page.getByRole("heading", { name: "平台运营工作台", exact: true })).toBeVisible();

    await page.getByLabel("账户状态").selectOption("deleted");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("button", { name: "确认更新访问设置" })).toBeVisible();
    await expect(page.getByText(/不会被物理删除/)).toBeVisible();
    // 提交前出现确认步骤：确认面板展示期间与取消之前，不发生任何写入。
    expect(requests).toHaveLength(0);

    await page.getByRole("button", { name: "取消", exact: true }).click();
    await expect(page.getByRole("button", { name: "确认更新访问设置" })).toHaveCount(0);
    expect(requests).toHaveLength(0);

    // 再次提交仍先确认；确认后才发起写入，且请求内容为「已删除」。
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("button", { name: "确认更新访问设置" })).toBeVisible();
    await page.getByRole("button", { name: "确认更新访问设置" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    expect(requests).toHaveLength(1);
    expect(requests[0].status).toBe("deleted");
    expect(requests[0].idempotencyKey).toMatch(/^idem_console_access_/);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} Session revocation confirmation can be cancelled without a write`, async ({ page }) => {
    await page.setViewportSize(viewport);
    let requests = 0;
    await page.route("**/api/v1/operations/sessions/*/revocations", (route) => { requests += 1; return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "session_revoke", status: "succeeded", resource_id: operations.sessions[0].id }, request_id: "req_revoke_cancel" }) }); });
    await page.goto("/operations");
    await page.getByRole("button", { name: "撤销登录" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toContainText("very.long.operator.identity@henu.edu.cn");
    await expect(dialog).toContainText("撤销核心登录");
    await expect(dialog.getByRole("button", { name: "取消" })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("button", { name: "撤销登录" })).toBeFocused();
    expect(requests).toBe(0);
  });

  test(`${viewport.name} account suspension shows a consequential diff before writing`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const requests: Array<{ status?: string }> = [];
    await page.route("**/api/v1/operations/users/*/access-updates", async (route) => {
      const body = await route.request().postDataJSON();
      requests.push({ status: body.status });
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "succeeded", resource_id: operations.accounts[0].id, resource_version: 2 }, request_id: "req_access_suspended" }) });
    });
    await page.goto("/operations");
    await expect(page.getByRole("heading", { name: "平台运营工作台", exact: true })).toBeVisible();

    await page.getByLabel("账户状态").selectOption("suspended");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("dialog")).toContainText("账户状态：正常 → 已停用");
    expect(requests).toHaveLength(0);
    await page.getByRole("button", { name: "确认更新访问设置" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    expect(requests).toHaveLength(1);
    expect(requests[0].status).toBe("suspended");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} audit panel renders operable fields with readable reason mapping`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const auditOperations = {
      ...operations,
      accounts: [{ ...operations.accounts[0], status: "deleted" }],
      audit: [
        { request_id: "req_allowed", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "operator@henu.edu.cn", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "GRANTED", created_at: "2026-07-19T00:00:00Z" },
        { request_id: "req_denied", actor_user_id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "deleted.operator@henu.edu.cn", permission_code: "quizcraft.attempt.write", target_kind: "resource", target_product_code: "quizcraft", target_resource_type: "attempt", target_resource_id: "attempt-9", decision: "denied", reason_code: "SESSION_EXPIRED", created_at: "2026-07-19T01:00:00Z" },
        { request_id: "req_unknown_reason", actor_user_id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "deleted.operator@henu.edu.cn", permission_code: "platform.operations.write", target_kind: "product", target_product_code: "notice", decision: "allowed", reason_code: "something_else", created_at: "2026-07-19T02:00:00Z" },
      ],
    };
    await page.route("**/api/v1/operations", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: auditOperations, request_id: "req_audit_envelope" }) }));
    await page.goto("/operations");

    // 发起人（无 display_name 时显示中性名称 + 邮箱）、时间（本地时区）、权限码、结果。
    await expect(page.getByText(/未设置姓名 · operator@henu.edu.cn/).first()).toBeVisible();
    await expect(page.getByText(/2026/).first()).toBeVisible();
    await expect(page.getByText("允许 · platform.operations.read")).toBeVisible();
    await expect(page.getByText("拒绝 · quizcraft.attempt.write")).toBeVisible();
    // 目标（产品/资源维度）与原因映射；未知原因码兜底为「其他原因」并保留原码小字。
    await expect(page.getByText(/目标 平台/)).toBeVisible();
    await expect(page.getByText(/目标 资源 \/ quizcraft \/ attempt \/ attempt-9/)).toBeVisible();
    await expect(page.getByText(/目标 产品 \/ notice/)).toBeVisible();
    await expect(page.getByText("原因：权限授予")).toBeVisible();
    await expect(page.getByText("原因：会话已过期")).toBeVisible();
    await expect(page.getByText(/原因：其他原因（something_else）/)).toBeVisible();
    // 已删除账户的审计行不消失：同一行仍渲染完整字段。
    await expect(page.getByText(/未设置姓名 · deleted.operator@henu.edu.cn.*目标 资源/)).toBeVisible();
    await expect(page.getByText(/用户 #5302/)).toHaveCount(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} dependency health renders Chinese labels with distinct visuals`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/operations", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ...operations, dependencies: { postgres: "ready", redis: "unavailable" } }, request_id: "req_health_envelope" }) }));
    await page.goto("/operations");
    const database = page.locator(".operation-summary-grid article").filter({ hasText: "数据库" });
    const cache = page.locator(".operation-summary-grid article").filter({ hasText: "缓存" });
    await expect(database.getByText("正常")).toBeVisible();
    await expect(cache.getByText("不可用")).toBeVisible();
    // 正常与故障在视觉上可区分：绿色 success 徽标 vs 红色 destructive 徽标，而非仅文字差异。
    await expect(database.locator("span.text-success")).toBeVisible();
    await expect(cache.locator("span.text-destructive")).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} accounts, sessions, and audit actors render neutral name plus email when display name is missing`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const named = {
      ...operations,
      accounts: [{ ...operations.accounts[0], display_name: undefined }],
      sessions: [{ ...operations.sessions[0], display_name: undefined }],
      audit: [{ ...operations.audit[0], display_name: undefined, created_at: "2026-07-19T03:00:00Z" }],
    };
    await page.route("**/api/v1/operations", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: named, request_id: "req_named_envelope" }) }));
    await page.goto("/operations");
    await expect(page.getByText("未设置姓名").first()).toBeVisible();
    await expect(page.getByText("very.long.operator.identity@henu.edu.cn").first()).toBeVisible();
    await expect(page.getByText(/用户 #5302/)).toHaveCount(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Platform Operations reload reconciles the original pending command and key", async ({ page }) => {
  let writeCalls = 0;
  let originalKey = "";
  let resolveSucceeded = false;
  const resolvedKeys: string[] = [];
  await page.route("**/api/v1/operations/users/*/access-updates", async (route) => {
    writeCalls += 1;
    originalKey = route.request().headers()["idempotency-key"] ?? "";
    await route.abort("connectionreset");
  });
  await page.route("**/api/v1/operations/results/access_update", (route) => {
    resolvedKeys.push(route.request().headers()["idempotency-key"] ?? "");
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: resolveSucceeded ? "succeeded" : "unknown", resource_id: operations.accounts[0].id, ...(resolveSucceeded ? { resource_version: 2 } : {}) }, request_id: "req_access_reload_resolve" }) });
  });
  await page.goto("/operations");
  await page.getByLabel("角色代码").fill("operations-reviewer");
  await page.getByRole("button", { name: "保存访问设置" }).click();
  await page.getByRole("button", { name: "确认更新访问设置" }).click();
  await expect(page.getByRole("status")).toContainText("结果尚未确认");
  await expect(page.getByRole("button", { name: "等待结果" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "已有操作待确认" })).toBeDisabled();
  const stored = await page.evaluate(() => sessionStorage.getItem("henukit.console.pending-platform-operation.v1"));
  expect(stored).toContain(originalKey);
  expect(stored).not.toContain("very.long.operator.identity@henu.edu.cn");
  resolveSucceeded = true;
  await page.reload();
  await expect(page.getByRole("status")).toContainText("操作已完成");
  expect(writeCalls).toBe(1);
  expect(resolvedKeys).toEqual([originalKey]);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.console.pending-platform-operation.v1"))).toBeNull();
});

test("Platform Operations discards pending state owned by another operator", async ({ page }) => {
  let resolveCalls = 0;
  await page.addInitScript(() => sessionStorage.setItem("henukit.console.pending-platform-operation.v1", JSON.stringify({ version: 1, operator_id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", operation: "access_update", idempotency_key: "idem_console_access_11111111-1111-4111-8111-111111111111", resource_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", target_label: "其他运营员", change_summary: "已停用" })));
  await page.route("**/api/v1/operations/results/access_update", (route) => { resolveCalls += 1; return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "unknown" }, request_id: "req_foreign_pending" }) }); });
  await page.goto("/operations");
  await expect(page.getByRole("button", { name: "保存访问设置" })).toBeEnabled();
  expect(resolveCalls).toBe(0);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.console.pending-platform-operation.v1"))).toBeNull();
});

test("Platform Operations does not write when pending-command storage is unavailable", async ({ page }) => {
  let writes = 0;
  await page.addInitScript(() => {
    Object.defineProperty(Storage.prototype, "setItem", { configurable: true, value: () => { throw new DOMException("blocked", "SecurityError"); } });
  });
  await page.route("**/api/v1/operations/sessions/*/revocations", (route) => { writes += 1; return route.abort(); });
  await page.goto("/operations");
  await page.getByRole("button", { name: "撤销登录" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认撤销会话" }).click();
  await expect(page.getByRole("status")).toContainText("操作未提交");
  expect(writes).toBe(0);
  await expect(page.getByRole("button", { name: "撤销登录" })).toBeEnabled();
});

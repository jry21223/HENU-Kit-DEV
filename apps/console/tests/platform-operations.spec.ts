import { expect, test } from "@playwright/test";

const operations = {
  access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
  accounts: [{ id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-07-19T00:00:00Z", grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }] }],
  sessions: [{ id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", kind: "core", last_seen_at: "2026-07-19T00:00:00Z", expires_at: "2026-07-19T01:00:00Z" }],
  mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 0, dead_letters: 0 },
  inbox_items: [{ id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302", source_product_code: "quizcraft", source_resource_type: "submission", source_resource_id: "submission-7", priority: "normal", status: "open", version: 1, created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" }],
  audit: [{ request_id: "req_operations_browser", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "张老师", email: "very.long.operator.identity@henu.edu.cn", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "permission_granted", created_at: "2026-07-19T00:00:00Z" }],
  pagination: {
    accounts: { page: 1, next_page: 2, next_cursor: "accounts-next" },
    sessions: { page: 1, next_page: 2, next_cursor: "sessions-next" },
    inbox_items: { page: 1, next_page: 2, next_cursor: "inbox-next" },
    audit: { page: 1, next_page: 2, next_cursor: "audit-next" },
  },
  dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-07-19T00:00:00Z",
};

async function gotoOperations(page: import("@playwright/test").Page) {
  await page.goto("/operations", { waitUntil: "commit" });
  await page.locator("#operations-heading").waitFor({ state: "attached", timeout: 20_000 });
}

test.beforeEach(async ({ page }) => {
  await page.route("https://fonts.googleapis.com/**", (route) => route.abort());
  await page.route("https://fonts.gstatic.com/**", (route) => route.abort());
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 403, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/operations*", (route) => {
    const requestURL = new URL(route.request().url());
    if (route.request().method() !== "GET" || requestURL.pathname !== "/api/v1/operations") return route.fallback();
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: operations, request_id: "req_operations_envelope" }) });
  });
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
    await gotoOperations(page);
    for (const heading of ["平台运营工作台", "账户、角色与权限", "登录会话", "邮件基础设施", "运营收件箱", "授权审计"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByLabel("角色代码")).toHaveValue("operations-operator");
    await expect(page.getByText("张老师").first()).toBeVisible();
    await expect(page.getByText("very.long.operator.identity@henu.edu.cn").first()).toBeVisible();
    await expect(page.getByText(/171f1c6f-7b10/)).toHaveCount(0);
    await page.getByRole("button", { name: "撤销登录" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    await page.getByLabel("角色代码").fill("operations-reviewer");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("status")).toContainText("结果还没确认");
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
    await gotoOperations(page);
    await expect(page.getByRole("heading", { name: "平台运营工作台", exact: true })).toBeVisible();

    await page.getByLabel("账户状态").selectOption("deleted");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("button", { name: "确认标记已删除" })).toBeVisible();
    await expect(page.getByText(/不会被物理删除/)).toBeVisible();
    // 提交前出现确认步骤：确认面板展示期间与取消之前，不发生任何写入。
    expect(requests).toHaveLength(0);

    await page.getByRole("button", { name: "取消", exact: true }).click();
    await expect(page.getByRole("button", { name: "确认标记已删除" })).toHaveCount(0);
    expect(requests).toHaveLength(0);

    // 再次提交仍先确认；确认后才发起写入，且请求内容为「已删除」。
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("button", { name: "确认标记已删除" })).toBeVisible();
    await page.getByRole("button", { name: "确认标记已删除" }).click();
    await expect(page.getByRole("status")).toContainText("操作已完成");
    expect(requests).toHaveLength(1);
    expect(requests[0].status).toBe("deleted");
    expect(requests[0].idempotencyKey).toMatch(/^idem_console_access_/);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} status changes other than deleted write without confirmation`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const requests: Array<{ status?: string }> = [];
    await page.route("**/api/v1/operations/users/*/access-updates", async (route) => {
      const body = await route.request().postDataJSON();
      requests.push({ status: body.status });
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "succeeded", resource_id: operations.accounts[0].id, resource_version: 2 }, request_id: "req_access_suspended" }) });
    });
    await gotoOperations(page);
    await expect(page.getByRole("heading", { name: "平台运营工作台", exact: true })).toBeVisible();

    // 只有改为「已删除」才强制确认：正常 → 已停用 点保存直接写入，不出现确认面板。
    await page.getByLabel("账户状态").selectOption("suspended");
    await page.getByRole("button", { name: "保存访问设置" }).click();
    await expect(page.getByRole("button", { name: "确认标记已删除" })).toHaveCount(0);
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
    await page.route("**/api/v1/operations?*", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: auditOperations, request_id: "req_audit_envelope" }) }));
    await gotoOperations(page);

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
    await page.route("**/api/v1/operations?*", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ...operations, dependencies: { postgres: "ready", redis: "unavailable" } }, request_id: "req_health_envelope" }) }));
    await gotoOperations(page);
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
    await page.route("**/api/v1/operations?*", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: named, request_id: "req_named_envelope" }) }));
    await gotoOperations(page);
    await expect(page.getByText("未设置姓名").first()).toBeVisible();
    await expect(page.getByText("very.long.operator.identity@henu.edu.cn").first()).toBeVisible();
    await expect(page.getByText(/用户 #5302/)).toHaveCount(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} read-only Platform Operations hides every mutation control`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/operations?*", (route) => route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { ...operations, access_context: { ...operations.access_context, permissions: ["platform.operations.read"] } }, request_id: "req_operations_read_only" }),
    }));
    await gotoOperations(page);
    await expect(page.getByText("只读").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "保存访问设置" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "新增角色 / 权限" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "删除授权" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "撤销登录" })).toHaveCount(0);
    await expect(page.getByLabel("角色代码")).toBeDisabled();
    await expect(page.getByLabel("账户状态")).toBeDisabled();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Platform Operations pages each bounded collection independently", async ({ page }) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/operations*", async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== "GET" || url.pathname !== "/api/v1/operations") return route.fallback();
    requests.push(url);
    const accountPage = Number(url.searchParams.get("accounts_page") ?? "1");
    const sessionPage = Number(url.searchParams.get("sessions_page") ?? "1");
    const paged = {
      ...operations,
      accounts: accountPage === 2
        ? [{ ...operations.accounts[0], id: "671f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "较早账户", email: "older.operator@henu.edu.cn" }]
        : operations.accounts,
      sessions: sessionPage === 2
        ? [{ ...operations.sessions[0], id: "871f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "较早会话" }]
        : operations.sessions,
      pagination: {
        ...operations.pagination,
        accounts: { page: accountPage, next_page: accountPage === 1 ? 2 : null, next_cursor: accountPage === 1 ? "accounts-next" : null },
        sessions: { page: sessionPage, next_page: sessionPage === 1 ? 2 : null, next_cursor: sessionPage === 1 ? "sessions-next" : null },
      },
    };
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: paged, request_id: "req_operations_page" }) });
  });

  await gotoOperations(page);
  await expect(page.getByText("张老师").first()).toBeVisible();
  await page.getByLabel("角色代码").fill("unsaved-operator-draft");
  await page.getByRole("button", { name: "会话下一页" }).click();
  await expect(page.getByText("较早会话").first()).toBeVisible();
  await expect(page.getByLabel("角色代码")).toHaveValue("unsaved-operator-draft");
  await page.getByRole("button", { name: "账户下一页" }).click();
  await expect(page.getByText("较早账户").first()).toBeVisible();

  const latest = requests[requests.length - 1];
  expect(latest?.searchParams.get("accounts_page")).toBe("2");
  expect(latest?.searchParams.get("accounts_cursor")).toBe("accounts-next");
  expect(latest?.searchParams.get("sessions_page")).toBe("2");
  expect(latest?.searchParams.get("sessions_cursor")).toBe("sessions-next");
  expect(latest?.searchParams.get("inbox_page")).toBe("1");
  expect(latest?.searchParams.get("audit_page")).toBe("1");
  expect(latest?.searchParams.get("snapshot_at")).toBe(operations.generated_at);
});

test("Platform Operations searches accounts without putting identity text in the URL", async ({ page }) => {
  const requests: Array<{ url: string; body: unknown }> = [];
  await page.route("**/api/v1/operations/accounts/search", async (route) => {
    requests.push({ url: route.request().url(), body: await route.request().postDataJSON() });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          accounts: [{ ...operations.accounts[0], id: "771f1c6f-7b10-4c92-91a2-b39bf5af5302", display_name: "目标账户", email: "target.operator@henu.edu.cn" }],
          next_page: null,
          next_cursor: null,
        },
        request_id: "req_operations_account_search",
      }),
    });
  });

  await gotoOperations(page);
  await page.getByLabel("账户搜索").fill("target.operator@henu.edu.cn");
  await page.getByRole("button", { name: "搜索账户" }).click();
  await expect(page.getByText("目标账户").first()).toBeVisible();
  expect(requests).toHaveLength(1);
  expect(requests[0].url).not.toContain("target.operator");
  expect(requests[0].body).toEqual({ query: "target.operator@henu.edu.cn", page: 1, snapshot_at: operations.generated_at });
});

test("Platform Operations explains rate limits and blocks paging while account search is pending", async ({ page }) => {
  let releaseSearch: (() => void) | undefined;
  await page.route("**/api/v1/operations/accounts/search", async (route) => {
    await new Promise<void>((resolve) => { releaseSearch = resolve; });
    await route.fulfill({ status: 429, contentType: "application/json", body: JSON.stringify({ error: { code: "RATE_LIMITED", message: "rate limited" } }) });
  });
  await gotoOperations(page);
  await page.getByLabel("账户搜索").fill("目标运营");
  await page.getByRole("button", { name: "搜索账户" }).click();
  await expect(page.getByRole("button", { name: "会话下一页" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "撤销登录" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "保存访问设置" })).toBeDisabled();
  await expect(page.getByLabel("角色代码")).toBeDisabled();
  await expect(page.getByLabel("账户状态")).toBeDisabled();
  releaseSearch?.();
  await expect(page.getByRole("status")).toContainText("请求过于频繁，请稍后重试");
});

test("Platform Operations explains a snapshot read rate limit", async ({ page }) => {
  await page.route("**/api/v1/operations?*", (route) => route.fulfill({ status: 429, contentType: "application/json", body: JSON.stringify({ error: { code: "RATE_LIMITED", message: "rate limited" } }) }));
  await gotoOperations(page);
  await expect(page.getByText("请求过于频繁，请稍后重试。")).toBeVisible();
});

test("Platform Operations refresh starts a new first-page snapshot", async ({ page }) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/operations*", async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== "GET" || url.pathname !== "/api/v1/operations") return route.fallback();
    requests.push(url);
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: operations, request_id: "req_operations_refresh" }) });
  });
  await gotoOperations(page);
  await expect(page.getByText(/数据快照/)).toBeVisible();
  await page.getByRole("button", { name: "账户下一页" }).click();
  await page.getByRole("button", { name: "刷新数据" }).click();
  await expect.poll(() => requests.length).toBeGreaterThanOrEqual(3);
  const latest = requests[requests.length - 1];
  expect(latest.searchParams.get("accounts_page")).toBe("1");
  expect(latest.searchParams.has("snapshot_at")).toBe(false);
  expect(latest.searchParams.has("accounts_cursor")).toBe(false);
});

test("Platform Operations write from a continuation refreshes a fresh first page", async ({ page }) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/operations*", async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== "GET" || url.pathname !== "/api/v1/operations") return route.fallback();
    requests.push(url);
    const accountPage = Number(url.searchParams.get("accounts_page") ?? "1");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { ...operations, accounts: [{ ...operations.accounts[0], display_name: accountPage === 2 ? "第二页账户" : "张老师" }], pagination: { ...operations.pagination, accounts: { page: accountPage, next_page: accountPage === 1 ? 2 : null, next_cursor: accountPage === 1 ? "accounts-next" : null } } }, request_id: "req_operations_write_refresh" }),
    });
  });
  await page.route("**/api/v1/operations/users/*/access-updates", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { operation: "access_update", status: "succeeded", resource_id: operations.accounts[0].id, resource_version: 2 }, request_id: "req_access_page_two" }) }));
  await gotoOperations(page);
  await page.getByRole("button", { name: "账户下一页" }).click();
  await expect(page.getByText("第二页账户")).toBeVisible();
  await page.getByLabel("账户状态").selectOption("suspended");
  await page.getByRole("button", { name: "保存访问设置" }).click();
  await expect.poll(() => requests.length).toBeGreaterThanOrEqual(3);
  const latest = requests[requests.length - 1];
  expect(latest.searchParams.get("accounts_page")).toBe("1");
  expect(latest.searchParams.has("snapshot_at")).toBe(false);
  expect(latest.searchParams.has("accounts_cursor")).toBe(false);
  await expect(page.getByText("张老师").first()).toBeVisible();
});

test("390px Platform Operations keeps account paging and body-only search usable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/v1/operations*", async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== "GET" || url.pathname !== "/api/v1/operations") return route.fallback();
    const accountPage = Number(url.searchParams.get("accounts_page") ?? "1");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          ...operations,
          accounts: accountPage === 2 ? [{ ...operations.accounts[0], display_name: "移动端较早账户" }] : operations.accounts,
          pagination: { ...operations.pagination, accounts: { page: accountPage, next_page: accountPage === 1 ? 2 : null, next_cursor: accountPage === 1 ? "accounts-next" : null } },
        },
        request_id: "req_operations_mobile_page",
      }),
    });
  });
  await page.route("**/api/v1/operations/accounts/search", async (route) => {
    expect(route.request().url()).not.toContain("target.operator");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { accounts: [{ ...operations.accounts[0], display_name: "移动端目标账户" }], next_page: null, next_cursor: null }, request_id: "req_operations_mobile_search" }),
    });
  });

  await gotoOperations(page);
  await page.getByRole("button", { name: "账户下一页" }).click();
  await expect(page.getByText("移动端较早账户").first()).toBeVisible();
  await page.getByLabel("账户搜索").fill("target.operator@henu.edu.cn");
  await page.getByRole("button", { name: "搜索账户" }).click();
  await expect(page.getByText("移动端目标账户").first()).toBeVisible();
  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

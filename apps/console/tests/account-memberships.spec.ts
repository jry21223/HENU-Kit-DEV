import { expect, test, type Page } from "@playwright/test";

const targetUserID = "88888888-8888-4888-8888-888888888888";
const secondUserID = "99999999-9999-4999-8999-999999999999";
const targetEmail = "member@stu.henu.edu.cn";
const secondEmail = "second@stu.henu.edu.cn";
const targetName = "张同学";
const secondName = "李同学";
const session = {
  user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
  access_context: {
    permissions: ["account.membership.write"],
    scopes: [{ kind: "product", product_code: "account-portfolio" }],
    verified_at: "2026-07-28T00:00:00Z",
  },
  expires_at: "2026-07-28T01:00:00Z",
};

type Membership = { plan: "free" | "lifetime"; lifetime: boolean; version: number };
const freeMembership: Membership = { plan: "free", lifetime: false, version: 1 };

function account(id: string, displayName: string, email: string, membership: Membership | null = freeMembership) {
  return { id, display_name: displayName, email, status: "active", membership };
}

async function installSession(page: Page, value = session) {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: value, request_id: "req_membership_session" }) }),
  );
}

async function selectTarget(page: Page) {
  await page.getByLabel("显示名称或邮箱").fill(targetName);
  await page.getByRole("button", { name: "搜索" }).click();
  await page.getByRole("button", { name: new RegExp(`${targetName}.*${targetEmail}`) }).click();
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} operator searches a bounded user list, selects one user, then grants and revokes VIP`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await installSession(page);
    let membership: Membership = { ...freeMembership };
    const searches: Array<{ query: string; page: number }> = [];

    await page.route("**/api/v1/account/memberships/search", async (route) => {
      const input = await route.request().postDataJSON() as { query: string; page: number };
      searches.push(input);
      const accounts = input.query === ""
        ? [account(targetUserID, targetName, targetEmail, membership), account(secondUserID, secondName, secondEmail)]
        : [account(targetUserID, targetName, targetEmail, membership)];
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts, next_page: null }, request_id: "req_membership_search" }) });
    });
    await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: "req_membership_read" }) }),
    );
    await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, async (route) => {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_membership_grant_/);
      expect(route.request().headers()["x-actor-user-id"]).toBeUndefined();
      expect(await route.request().postDataJSON()).toEqual({ reason: "核验后发放终身权益", expected_version: 1 });
      membership = { plan: "lifetime", lifetime: true, version: 2 };
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: "req_membership_grant" }) });
    });
    await page.route(`**/api/v1/account/memberships/${targetUserID}/revocations`, async (route) => {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_membership_revoke_/);
      expect(await route.request().postDataJSON()).toEqual({ reason: "复核后撤销终身权益", expected_version: 2 });
      membership = { plan: "free", lifetime: false, version: 3 };
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: "req_membership_revoke" }) });
    });

    await page.goto("/account/memberships");
    await expect(page.locator('[data-membership-list-state="ready"]')).toContainText(secondEmail);
    await selectTarget(page);
    const detail = page.locator('[data-account-membership-detail-state="ready"]');
    await expect(detail).toContainText(targetName);
    await expect(detail).toContainText(targetEmail);
    await expect(detail).not.toContainText(targetUserID);
    expect(searches).toEqual([{ query: "", page: 1 }, { query: targetName, page: 1 }]);

    await page.getByLabel("操作原因").fill("核验后发放终身权益");
    await page.getByRole("button", { name: "发放终身会员" }).click();
    await expect(page.locator("[data-membership-confirm-step]")).toContainText(`${targetName} · ${targetEmail}`);
    await expect(page.getByText("提交后立即生效，之后可通过相反操作调整。")).toBeVisible();
    await page.getByRole("button", { name: "确认执行" }).click();
    await expect(page.getByRole("status")).toContainText("终身会员权益已发放");

    await page.getByLabel("操作原因").fill("复核后撤销终身权益");
    await page.getByRole("button", { name: "撤销终身会员" }).click();
    await page.getByRole("button", { name: "确认执行" }).click();
    await expect(page.getByRole("status")).toContainText("终身会员权益已撤销");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });

  test(`${viewport.name} retries an unknown VIP result with the original idempotency key`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await installSession(page);
    const keys: string[] = [];
    await page.route("**/api/v1/account/memberships/search", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts: [account(targetUserID, targetName, targetEmail)], next_page: null }, request_id: "req_membership_search" }) }),
    );
    await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, async (route) => {
      keys.push(route.request().headers()["idempotency-key"] ?? "");
      const succeeded = keys.length > 1;
      await route.fulfill({
        status: succeeded ? 200 : 503,
        contentType: "application/json",
        body: JSON.stringify(succeeded
          ? { data: { membership: { plan: "lifetime", lifetime: true, version: 2 } }, request_id: "req_membership_replayed" }
          : { error: { code: "unavailable", message: "retry later" }, request_id: "req_membership_unknown" }),
      });
    });

    await page.goto("/account/memberships");
    await page.getByRole("button", { name: new RegExp(`${targetName}.*${targetEmail}`) }).click();
    await page.getByLabel("操作原因").fill("网关结果未确认时重试");
    await page.getByRole("button", { name: "发放终身会员" }).click();
    await page.getByRole("button", { name: "确认执行" }).click();
    await expect(page.getByRole("status")).toContainText("结果还没确认");
    await expect(page.getByRole("status")).not.toContainText(targetUserID);
    const stored = await page.evaluate(() => sessionStorage.getItem("henukit.account-membership.pending-command"));
    expect(stored).toContain(targetUserID);
    expect(stored).toContain(targetName);
    expect(stored).toContain(targetEmail);
    await expect(page.getByRole("status")).toContainText(`${targetName} · ${targetEmail}`);
    await page.getByRole("button", { name: "按原请求重试" }).click();
    await expect(page.getByRole("status")).toContainText("终身会员权益已发放");
    expect(keys).toHaveLength(2);
    expect(keys[1]).toBe(keys[0]);
  });
}

test("Console keeps pagination on the submitted search while the input is being edited", async ({ page }) => {
  await installSession(page);
  const searches: Array<{ query: string; page: number }> = [];
  await page.route("**/api/v1/account/memberships/search", async (route) => {
    const input = await route.request().postDataJSON() as { query: string; page: number };
    searches.push(input);
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts: [account(targetUserID, targetName, targetEmail)], next_page: input.page === 1 ? 2 : null }, request_id: "req_membership_search" }) });
  });
  await page.goto("/account/memberships");
  await page.getByLabel("显示名称或邮箱").fill("A");
  await page.getByRole("button", { name: "搜索" }).click();
  await page.getByLabel("显示名称或邮箱").fill("B");
  await page.getByRole("button", { name: "下一页" }).click();
  expect(searches.slice(-2)).toEqual([{ query: "A", page: 1 }, { query: "A", page: 2 }]);
});

test("Console refreshes membership after a stale-version conflict", async ({ page }) => {
  await installSession(page);
  let reads = 0;
  await page.route("**/api/v1/account/memberships/search", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts: [account(targetUserID, targetName, targetEmail)], next_page: null }, request_id: "req_membership_search" }) }),
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) => {
    reads += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "lifetime", lifetime: true, version: 2 } }, request_id: "req_membership_read" }) });
  });
  await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, (route) =>
    route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ error: { code: "conflict", message: "stale version" }, request_id: "req_membership_conflict" }) }),
  );

  await page.goto("/account/memberships");
  await page.getByRole("button", { name: new RegExp(`${targetName}.*${targetEmail}`) }).click();
  await page.getByLabel("操作原因").fill("并发更新后重新核验");
  await page.getByRole("button", { name: "发放终身会员" }).click();
  await page.getByRole("button", { name: "确认执行" }).click();
  await expect(page.getByRole("status")).toContainText("会员版本已变化");
  await expect(page.getByRole("heading", { name: "终身会员", exact: true })).toBeVisible();
  expect(reads).toBe(1);
});

test("Console shows bounded-list empty, outage, permission, and signed-out states honestly", async ({ page }) => {
  await installSession(page);
  let searches = 0;
  await page.route("**/api/v1/account/memberships/search", async (route) => {
    searches += 1;
    if (searches === 1) {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts: [], next_page: null }, request_id: "req_membership_empty" }) });
      return;
    }
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "unavailable", message: "retry later" }, request_id: "req_membership_down" }) });
  });
  await page.goto("/account/memberships");
  await expect(page.getByText("没有匹配的用户。可以清空搜索条件后重试。")).toBeVisible();
  await page.getByRole("button", { name: "搜索" }).click();
  await expect(page.getByText("用户与会员状态暂时不可用，请稍后再试。")).toBeVisible();

  const noPermission = { ...session, access_context: { ...session.access_context, permissions: [] } };
  await page.unrouteAll({ behavior: "wait" });
  await installSession(page, noPermission);
  await page.reload();
  await expect(page.getByText("当前账户没有会员权益运营权限，请联系管理员开通。")).toBeVisible();

  await page.unrouteAll({ behavior: "wait" });
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: { code: "unauthorized", message: "expired" }, request_id: "req_membership_signed_out" }) }),
  );
  await page.reload();
  await expect(page.getByText("登录状态已过期，请重新登录后再操作。")).toBeVisible();
  await expect(page.getByRole("button", { name: "搜索" })).toHaveCount(0);
});

test("Console discards a persisted retry that belongs to another operator", async ({ page }) => {
  await page.addInitScript(({ storageKey, userID }) => {
    sessionStorage.setItem(storageKey, JSON.stringify({
      kind: "grant",
      operatorID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      userID,
      input: { reason: "旧操作员请求", expected_version: 1 },
      key: "idem_account_membership_grant_previous",
    }));
  }, { storageKey: "henukit.account-membership.pending-command", userID: targetUserID });
  await installSession(page);
  await page.route("**/api/v1/account/memberships/search", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { accounts: [], next_page: null }, request_id: "req_membership_search" }) }),
  );
  await page.goto("/account/memberships");
  await expect(page.getByRole("button", { name: "按原请求重试" })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.account-membership.pending-command"))).toBeNull();
});

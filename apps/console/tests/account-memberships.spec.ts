import { expect, test } from "@playwright/test";

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

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} Console operator finds an account by email, verifies the name, and grants and revokes a durable membership`, async ({ page }) => {
    await page.setViewportSize(viewport);
    let membership = { plan: "free", lifetime: false, version: 1 };
    const lookedUpEmails: string[] = [];

    await page.route("**/api/v1/session", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
    );
    await page.route("**/api/v1/operations/account-lookups", async (route) => {
      lookedUpEmails.push((await route.request().postDataJSON()).email);
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) });
    });
    await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: "req_membership_lookup" }) })
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
      expect(route.request().headers()["x-actor-user-id"]).toBeUndefined();
      expect(await route.request().postDataJSON()).toEqual({ reason: "复核后撤销终身权益", expected_version: 2 });
      membership = { plan: "free", lifetime: false, version: 3 };
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: "req_membership_revoke" }) });
    });

    await page.goto("/account/memberships");
    await expect(page.getByRole("heading", { name: "会员权益运营" })).toBeVisible();
    await page.getByLabel("完整邮箱").fill(targetEmail);
    await page.getByRole("button", { name: "查找账户" }).click();
    await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText(targetName);
    await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText("免费会员");
    expect(lookedUpEmails).toEqual([targetEmail]);

    await page.getByLabel("操作原因").fill("核验后发放终身权益");
    await page.getByRole("button", { name: "发放终身会员" }).click();
    await expect(page.locator("[data-membership-confirm-step]")).toContainText(targetName);
    await page.getByRole("button", { name: "确认执行" }).click();
    await expect(page.getByRole("status")).toContainText("终身会员权益已发放");
    await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText("终身会员");

    await page.getByLabel("操作原因").fill("复核后撤销终身权益");
    await page.getByRole("button", { name: "撤销终身会员" }).click();
    await expect(page.locator("[data-membership-confirm-step]")).toContainText(targetName);
    await page.getByRole("button", { name: "确认执行" }).click();
    await expect(page.getByRole("status")).toContainText("终身会员权益已撤销");
    await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText("免费会员");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Console refreshes the durable membership after a stale-version conflict", async ({ page }) => {
  let membership = { plan: "free", lifetime: false, version: 1 };
  let lookupCount = 0;

  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) => {
    lookupCount += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership }, request_id: `req_membership_lookup_${lookupCount}` }) });
  });
  await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, async (route) => {
    expect(await route.request().postDataJSON()).toEqual({ reason: "并发更新后重新核验", expected_version: 1 });
    membership = { plan: "lifetime", lifetime: true, version: 2 };
    await route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ error: { code: "conflict", message: "stale version" }, request_id: "req_membership_conflict" }) });
  });

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("操作原因").fill("并发更新后重新核验");
  await page.getByRole("button", { name: "发放终身会员" }).click();
  await page.getByRole("button", { name: "确认执行" }).click();

  await expect(page.getByRole("status")).toContainText("会员版本已变化");
  await expect(page.locator('[data-account-membership-detail-state="ready"]').getByRole("heading", { name: "终身会员", exact: true })).toBeVisible();
  expect(lookupCount).toBe(2);
});

test("Console ignores a late account lookup after the operator changes the email", async ({ page }) => {
  let releaseFirstLookup: (() => void) | undefined;
  let firstLookupStarted: (() => void) | undefined;
  const firstLookupPending = new Promise<void>((resolve) => {
    firstLookupStarted = resolve;
  });
  const firstLookupRelease = new Promise<void>((resolve) => {
    releaseFirstLookup = resolve;
  });
  let firstUserMembershipRead = false;

  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", async (route) => {
    const { email } = await route.request().postDataJSON();
    if (email === targetEmail) {
      firstLookupStarted?.();
      await firstLookupRelease;
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_first", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_second", account: { id: secondUserID, display_name: secondName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) });
  });
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) => {
    firstUserMembershipRead = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "free", lifetime: false, version: 1 } }, request_id: "req_membership_first" }) });
  });
  await page.route(`**/api/v1/account/memberships/${secondUserID}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "lifetime", lifetime: true, version: 2 } }, request_id: "req_membership_second" }) })
  );

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await firstLookupPending;

  await page.getByLabel("完整邮箱").fill(secondEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.locator('[data-account-membership-detail-state="ready"]').getByRole("heading", { name: "终身会员", exact: true })).toBeVisible();
  await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText(secondName);

  releaseFirstLookup?.();
  await expect(page.locator('[data-account-membership-detail-state="ready"]')).toContainText(secondName);
  await expect(page.getByRole("button", { name: "撤销终身会员" })).toBeVisible();
  expect(firstUserMembershipRead).toBe(false);
});

test("Console writes nothing when the operator cancels the membership confirmation step", async ({ page }) => {
  let mutationAttempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "free", lifetime: false, version: 1 } }, request_id: "req_membership_lookup" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, (route) => {
    mutationAttempts += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "lifetime", lifetime: true, version: 2 } }, request_id: "req_membership_grant" }) });
  });

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("操作原因").fill("取消后不得写入");
  await page.getByRole("button", { name: "发放终身会员" }).click();
  await expect(page.locator("[data-membership-confirm-step]")).toContainText(targetName);
  await page.getByRole("button", { name: "取消" }).click();
  await expect(page.getByRole("status")).toContainText("已取消，未执行任何操作。");
  expect(mutationAttempts).toBe(0);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.account-membership.pending-command"))).toBeNull();
});

test("Console distinguishes an unknown email from a lookup outage", async ({ page }) => {
  let lookupAttempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", async (route) => {
    lookupAttempts += 1;
    const { email } = await route.request().postDataJSON();
    if (email === "absent@stu.henu.edu.cn") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_miss", account: null }, request_id: "req_membership_lookup_gateway" }) });
      return;
    }
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "unavailable", message: "retry later" }, request_id: "req_membership_lookup_down" }) });
  });

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill("absent@stu.henu.edu.cn");
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("没有找到该邮箱对应的账户。请核对邮箱后重试；这不是服务不可用。")).toBeVisible();

  await page.getByLabel("完整邮箱").fill("service-down@stu.henu.edu.cn");
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("账户查找服务暂时不可用，请稍后再试。")).toBeVisible();
  expect(lookupAttempts).toBe(2);
});

test("Console retries an unknown membership result with the original idempotency key and never persists the email", async ({ page }) => {
  const idempotencyKeys: string[] = [];
  const bodies: unknown[] = [];
  let mutationAttempts = 0;

  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "free", lifetime: false, version: 1 } }, request_id: "req_membership_lookup" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}/grants`, async (route) => {
    mutationAttempts += 1;
    idempotencyKeys.push(route.request().headers()["idempotency-key"] ?? "");
    bodies.push(await route.request().postDataJSON());
    if (mutationAttempts === 1) {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "unavailable", message: "retry later" }, request_id: "req_membership_unknown" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { membership: { plan: "lifetime", lifetime: true, version: 2 } }, request_id: "req_membership_replayed" }) });
  });

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("操作原因").fill("网关结果未确认时重试");
  await page.getByRole("button", { name: "发放终身会员" }).click();
  await page.getByRole("button", { name: "确认执行" }).click();

  await expect(page.getByRole("status")).toContainText("结果还没确认");
  await expect(page.getByRole("status")).toContainText("待确认操作：向用户");
  await expect(page.getByRole("status").getByText(targetUserID, { exact: true })).toBeVisible();
  const stored = await page.evaluate(() => sessionStorage.getItem("henukit.account-membership.pending-command"));
  expect(stored).toContain(targetUserID);
  expect(stored).not.toContain(targetEmail);
  await page.getByRole("button", { name: "确认并按原请求重试" }).click();
  await expect(page.getByRole("status")).toContainText("终身会员权益已发放");
  expect(idempotencyKeys).toHaveLength(2);
  expect(idempotencyKeys[1]).toBe(idempotencyKeys[0]);
  expect(bodies).toEqual([
    { reason: "网关结果未确认时重试", expected_version: 1 },
    { reason: "网关结果未确认时重试", expected_version: 1 },
  ]);
});

test("Console fails closed for missing membership permission and a server denial", async ({ page }) => {
  const noMembershipPermission = {
    ...session,
    access_context: { ...session.access_context, permissions: [] },
  };
  let membershipRead = false;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: noMembershipPermission, request_id: "req_membership_session" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) => {
    membershipRead = true;
    return route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "forbidden", message: "forbidden" }, request_id: "req_membership_denied" }) });
  });

  await page.goto("/account/memberships");
  await expect(page.getByText("当前账户没有会员权益运营权限，请联系管理员开通。")).toBeVisible();
  expect(membershipRead).toBe(false);
});

test("Console surfaces a membership endpoint authorization denial", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
    route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "forbidden", message: "forbidden" }, request_id: "req_membership_denied" }) })
  );

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("当前账户没有会员权益运营权限，请联系管理员开通。")).toBeVisible();
});

test("Console presents an expired Console session without a fake membership state", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: { code: "unauthorized", message: "expired" }, request_id: "req_membership_signed_out" }) })
  );

  await page.goto("/account/memberships");
  await expect(page.getByText("登录状态已过期，请重新登录后再操作。")).toBeVisible();
  await expect(page.getByRole("button", { name: "查找账户" })).toHaveCount(0);
});

test("Console fails closed when a membership read reports an expired session", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { request_id: "req_membership_lookup_account", account: { id: targetUserID, display_name: targetName, status: "active" } }, request_id: "req_membership_lookup_gateway" }) })
  );
  await page.route(`**/api/v1/account/memberships/${targetUserID}`, (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: { code: "unauthorized", message: "expired" }, request_id: "req_membership_read_signed_out" }) })
  );

  await page.goto("/account/memberships");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("登录状态已过期，请重新登录后再操作。")).toBeVisible();
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
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_membership_session" }) })
  );

  await page.goto("/account/memberships");
  await expect(page.getByRole("button", { name: "确认并按原请求重试" })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.account-membership.pending-command"))).toBeNull();
});

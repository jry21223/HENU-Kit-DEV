import { expect, test } from "@playwright/test";

const targetUserID = "88888888-8888-4888-8888-888888888888";
const targetEmail = "points@stu.henu.edu.cn";
const targetName = "张同学";
const session = {
  user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
  access_context: {
    permissions: ["account.points.adjust"],
    scopes: [{ kind: "product", product_code: "account-portfolio" }],
    verified_at: "2026-07-28T00:00:00Z",
  },
  expires_at: "2026-07-28T01:00:00Z",
};

async function routeAccountLookup(page: import("@playwright/test").Page, account: { id: string; display_name: string; email: string; status: string } | null) {
  await page.route("**/api/v1/operations/account-lookups", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { account }, request_id: "req_points_lookup_gateway" }),
    })
  );
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} Console operator finds an account by email, confirms the target, and adjusts durable points`, async ({ page }) => {
    await page.setViewportSize(viewport);
    let attempts = 0;

    await page.route("**/api/v1/session", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_points_session" }) }),
    );
    await routeAccountLookup(page, { id: targetUserID, display_name: targetName, email: targetEmail, status: "active" });
    await page.route("**/api/v1/account/points/adjustments", async (route) => {
      attempts += 1;
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_points_/);
      expect(route.request().headers()["x-actor-user-id"]).toBeUndefined();
      const input = await route.request().postDataJSON();
      expect(input).toEqual(attempts === 1
        ? { user_id: targetUserID, amount: 120, reason: "人工核验后的补偿积分" }
        : { user_id: targetUserID, amount: -20, reason: "重复记账复核扣减" });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            balance: attempts === 1 ? 120 : 100,
            entry: {
              id: attempts === 1 ? "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaaa" : "aaaaaaa2-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
              amount: attempts === 1 ? 120 : -20,
              reason: attempts === 1 ? "人工核验后的补偿积分" : "重复记账复核扣减",
              created_at: "2026-07-28T00:00:00Z",
            },
          },
          request_id: `req_points_adjust_${attempts}`,
        }),
      });
    });

    await page.goto("/account/points");
    await expect(page.getByRole("heading", { name: "积分账本运营" })).toBeVisible();
    await page.getByLabel("完整邮箱").fill(targetEmail);
    await page.getByRole("button", { name: "查找账户" }).click();
    await expect(page.locator("[data-account-points-result]")).toContainText(targetName);
    await page.getByLabel("积分变更").fill("120");
    await page.getByLabel("操作原因").fill("人工核验后的补偿积分");
    await page.getByRole("button", { name: "提交积分调整" }).click();

    await expect(page.locator("[data-points-confirm-step]")).toContainText(targetName);
    await expect(page.locator("[data-points-confirm-step]")).toContainText("120");
    await page.getByRole("button", { name: "确认写入" }).click();

    await expect(page.getByText("积分调整已写入账本，并已为目标用户创建持久化通知。")).toBeVisible();
    await expect(page.locator("[data-account-points-result]")).toContainText("当前积分余额 120");
    await expect(page.locator("[data-account-points-result]")).toContainText("人工核验后的补偿积分");

    await page.getByLabel("积分变更").fill("-20");
    await page.getByLabel("操作原因").fill("重复记账复核扣减");
    await page.getByRole("button", { name: "提交积分调整" }).click();
    await expect(page.locator("[data-points-confirm-step]")).toContainText(targetName);
    await expect(page.locator("[data-points-confirm-step]")).toContainText("20");
    await page.getByRole("button", { name: "确认写入" }).click();
    await expect(page.locator("[data-account-points-result]")).toContainText("当前积分余额 100");
    await expect(page.locator("[data-account-points-result]")).toContainText("重复记账复核扣减");

    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Console preserves JavaScript-safe point boundaries without rounding a browser command", async ({ page }) => {
  const maxSafe = Number.MAX_SAFE_INTEGER;
  let attempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_points_session" }) }),
  );
  await routeAccountLookup(page, { id: targetUserID, display_name: targetName, email: targetEmail, status: "active" });
  await page.route("**/api/v1/account/points/adjustments", async (route) => {
    attempts += 1;
    const input = await route.request().postDataJSON();
    expect(input).toEqual(attempts === 1
      ? { user_id: targetUserID, amount: maxSafe, reason: "最大安全整数记账。" }
      : { user_id: targetUserID, amount: -maxSafe, reason: "最小安全整数扣减。" });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          balance: attempts === 1 ? maxSafe : 0,
          entry: {
            id: attempts === 1 ? "aaaaaaa4-aaaa-4aaa-8aaa-aaaaaaaaaaaa" : "aaaaaaa5-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            amount: attempts === 1 ? maxSafe : -maxSafe,
            reason: attempts === 1 ? "最大安全整数记账。" : "最小安全整数扣减。",
            created_at: "2026-07-28T00:00:00Z",
          },
        },
        request_id: `req_points_safe_${attempts}`,
      }),
    });
  });

  await page.goto("/account/points");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("积分变更").fill(String(maxSafe));
  await page.getByLabel("操作原因").fill("最大安全整数记账。");
  await page.getByRole("button", { name: "提交积分调整" }).click();
  await page.getByRole("button", { name: "确认写入" }).click();
  await expect(page.getByText("积分调整已写入账本，并已为目标用户创建持久化通知。")).toBeVisible();

  await page.getByLabel("积分变更").fill(String(-maxSafe));
  await page.getByLabel("操作原因").fill("最小安全整数扣减。");
  await page.getByRole("button", { name: "提交积分调整" }).click();
  await page.getByRole("button", { name: "确认写入" }).click();
  await expect(page.getByText("积分调整已写入账本，并已为目标用户创建持久化通知。")).toBeVisible();

  await page.getByLabel("积分变更").fill("9007199254740992");
  await page.getByLabel("操作原因").fill("越界值不得由浏览器截断后发送。");
  await page.getByRole("button", { name: "提交积分调整" }).click();
  await expect(page.getByText("请先查找并核对账户，再填写非零整数积分和不超过 1000 字的操作原因。")).toBeVisible();
  expect(attempts).toBe(2);
});

test("Console writes nothing when the operator cancels the point confirmation", async ({ page }) => {
  let adjustAttempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_points_session" }) }),
  );
  await routeAccountLookup(page, { id: targetUserID, display_name: targetName, email: targetEmail, status: "active" });
  await page.route("**/api/v1/account/points/adjustments", (route) => {
    adjustAttempts += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { balance: 120, entry: { id: "aaaaaaa6-aaaa-4aaa-8aaa-aaaaaaaaaaaa", amount: 120, reason: "canceled", created_at: "2026-07-28T00:00:00Z" } }, request_id: "req_points_canceled" }) });
  });

  await page.goto("/account/points");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("积分变更").fill("120");
  await page.getByLabel("操作原因").fill("取消后不得写入");
  await page.getByRole("button", { name: "提交积分调整" }).click();
  await expect(page.locator("[data-points-confirm-step]")).toContainText(targetName);
  await expect(page.locator("[data-points-confirm-step]")).toContainText("不可撤销");
  await page.getByRole("button", { name: "取消" }).click();
  await expect(page.getByText("已取消，未写入任何积分流水。")).toBeVisible();
  expect(adjustAttempts).toBe(0);
  await expect.poll(() => page.evaluate(() => sessionStorage.getItem("henukit.account-points.pending-command"))).toBeNull();
});

test("Console distinguishes an unknown email from a lookup outage", async ({ page }) => {
  let lookupAttempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_points_session" }) }),
  );
  await page.route("**/api/v1/operations/account-lookups", async (route) => {
    lookupAttempts += 1;
    const { email } = await route.request().postDataJSON();
    if (email === "absent@stu.henu.edu.cn") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { account: null }, request_id: "req_points_lookup_gateway" }) });
      return;
    }
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "unavailable", message: "retry later" }, request_id: "req_points_lookup_down" }) });
  });

  await page.goto("/account/points");
  await page.getByLabel("完整邮箱").fill("absent@stu.henu.edu.cn");
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("没有找到该邮箱对应的账户。请核对邮箱后重试；这不是服务不可用。")).toBeVisible();

  await page.getByLabel("完整邮箱").fill("service-down@stu.henu.edu.cn");
  await page.getByRole("button", { name: "查找账户" }).click();
  await expect(page.getByText("账户查找服务暂时不可用，请稍后再试。")).toBeVisible();
  expect(lookupAttempts).toBe(2);
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "390px", width: 390, height: 844 }]) {
test(`${viewport.name} Console retries an unknown point adjustment with its original identity and idempotency key`, async ({ page }) => {
  await page.setViewportSize(viewport);
  const keys: string[] = [];
  let attempts = 0;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_points_session" }) }),
  );
  await routeAccountLookup(page, { id: targetUserID, display_name: targetName, email: targetEmail, status: "active" });
  await page.route("**/api/v1/account/points/adjustments", async (route) => {
    attempts += 1;
    keys.push(route.request().headers()["idempotency-key"] ?? "");
    expect(await route.request().postDataJSON()).toEqual({ user_id: targetUserID, amount: 30, reason: "网关结果未确认时重试" });
    if (attempts === 1) {
      await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "unavailable", message: "retry" }, request_id: "req_points_unknown" }) });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { balance: 30, entry: { id: "aaaaaaa3-aaaa-4aaa-8aaa-aaaaaaaaaaaa", amount: 30, reason: "网关结果未确认时重试", created_at: "2026-07-28T00:00:00Z" } }, request_id: "req_points_replayed" }),
    });
  });

  await page.goto("/account/points");
  await page.getByLabel("完整邮箱").fill(targetEmail);
  await page.getByRole("button", { name: "查找账户" }).click();
  await page.getByLabel("积分变更").fill("30");
  await page.getByLabel("操作原因").fill("网关结果未确认时重试");
  await page.getByRole("button", { name: "提交积分调整" }).click();
  await page.getByRole("button", { name: "确认写入" }).click();
  await expect(page.getByRole("status")).toContainText("同一已确认目标");
  await expect(page.getByRole("status")).toContainText(`${targetName} · ${targetEmail}`);
  await expect(page.getByRole("status")).toContainText("沿用原操作请求，系统会避免重复执行");
  const stored = await page.evaluate(() => sessionStorage.getItem("henukit.account-points.pending-command"));
  expect(stored).toContain(targetUserID);
  expect(stored).toContain(targetName);
  expect(stored).toContain(targetEmail);
  await page.getByRole("button", { name: "按原请求重试" }).click();
  await expect(page.getByText("积分调整已写入账本，并已为目标用户创建持久化通知。")).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[1]).toBe(keys[0]);
});
}

test("Console fails closed without the exact point-adjustment permission", async ({ page }) => {
  let writeAttempted = false;
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ...session, access_context: { ...session.access_context, permissions: [] } }, request_id: "req_points_session" }) }),
  );
  await page.route("**/api/v1/account/points/adjustments", (route) => {
    writeAttempted = true;
    return route.fulfill({ status: 500 });
  });

  await page.goto("/account/points");
  await expect(page.getByText("当前账户没有积分调整权限，请联系管理员开通。")).toBeVisible();
  expect(writeAttempted).toBe(false);
});

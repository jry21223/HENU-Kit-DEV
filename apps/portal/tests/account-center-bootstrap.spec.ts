import { expect, test } from "@playwright/test";

test("Account Center requests a login code through the bounded status contract", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.localStorage.clear();
  });
  let bootstrapCalls = 0;
  let codeCalls = 0;
  await page.route("**/account-auth/account/bootstrap?flow=login", async (route) => {
    bootstrapCalls += 1;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          flow: "login",
          csrf_token: "csrf-token-with-at-least-thirty-two-characters",
        },
        request_id: "req_browser_account_bootstrap",
      }),
    });
  });
  await page.route("**/account-auth/login/code", async (route) => {
    codeCalls += 1;
    const request = route.request();
    expect(request.headers()["accept"]).toBe("application/json");
    expect(request.headers()["x-henukit-form-response"]).toBe("status");
    expect(request.postData()).toBe(
      "csrf_token=csrf-token-with-at-least-thirty-two-characters&email=student%40henu.edu.cn&return_to=%2Fapi%2Fv1%2Fauth%2Flogin%3Freturn_to%3D%252Faccount"
    );
    await route.fulfill({
      status: 204,
      headers: { "X-Verification-Expires": "2026-08-24T07:30:00Z" },
    });
  });

  await page.goto("/account/login", { waitUntil: "networkidle" });
  const email = page.getByLabel("学校邮箱");
  await email.fill("student");
  await expect(page.getByText("将发送至 student@henu.edu.cn")).toBeVisible();
  await page.getByRole("button", { name: "发送验证码" }).click();

  await expect(page.getByText("验证码已进入发送队列（student@henu.edu.cn），请查收学校邮箱。")).toBeVisible();
  expect(bootstrapCalls).toBe(1);
  expect(codeCalls).toBe(1);
});

test("Account Center shows actionable Bootstrap failures without backend details", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.localStorage.clear();
  });
  await page.route("**/account-auth/account/bootstrap?flow=login", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "redis unavailable" },
        request_id: "req_browser_account_unavailable",
      }),
    });
  });

  await page.goto("/account/login", { waitUntil: "networkidle" });
  const email = page.getByLabel("学校邮箱");
  await email.fill("student");
  await expect(page.getByText("将发送至 student@henu.edu.cn")).toBeVisible();
  await page.getByRole("button", { name: "发送验证码" }).click();

  await expect(page.getByText("登录服务暂时不可用，请稍后再试")).toBeVisible();
  await expect(page.getByText("redis unavailable")).toHaveCount(0);
});

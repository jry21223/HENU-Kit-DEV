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

test("Account Center resumes a trusted Portal OAuth continuation after password login", async ({
  page,
}) => {
  await page.addInitScript(() => window.localStorage.clear());
  const continuation = "opaque-browser-continuation";
  let resumeCalls = 0;
  await page.route(
    `**/account-auth/account/bootstrap?flow=login&continuation=${continuation}`,
    async (route) => {
      expect(route.request().headers()["referer"]).toBeUndefined();
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            flow: "login",
            csrf_token: "csrf-token-with-at-least-thirty-two-characters",
            continuation: { available: true, product_name: "HENU Kit" },
          },
          request_id: "req_browser_continuation",
        }),
      });
    }
  );
  await page.route("**/account-auth/login/password", async (route) => {
    expect(route.request().postData()).toBe(
      "csrf_token=csrf-token-with-at-least-thirty-two-characters&email=student%40henu.edu.cn&password=correct+horse+battery+staple&return_to=%2Fapi%2Fv1%2Fauth%2Flogin%3Freturn_to%3D%252Faccount"
    );
    await route.fulfill({ status: 204 });
  });
  await page.route("**/account-auth/account/continuation/resume", async (route) => {
    resumeCalls += 1;
    expect(route.request().method()).toBe("POST");
    expect(route.request().postData()).toBe(
      "continuation=opaque-browser-continuation&csrf_token=csrf-token-with-at-least-thirty-two-characters"
    );
    await route.fulfill({
      status: 200,
      contentType: "text/html",
      body: "<!doctype html><title>resumed</title><p>OAuth resumed</p>",
    });
  });

  const accountResponse = await page.goto(`/account/login?continuation=${continuation}&product_name=伪造产品`, {
    waitUntil: "networkidle",
  });
  expect(accountResponse?.headers()["referrer-policy"]).toBe("no-referrer");
  expect(accountResponse?.headers()["content-security-policy"]).toContain(
    "frame-ancestors 'none'"
  );
  await expect(page.getByText("登录后继续前往 HENU Kit")).toBeVisible();
  await expect(page.getByText("伪造产品")).toHaveCount(0);
  await page.getByRole("button", { name: "密码登录" }).click();
  await page.getByLabel("学校邮箱").fill("student");
  await page.getByLabel("密码 / PASSWORD").fill("correct horse battery staple");
  await page.getByRole("button", { name: "登 录" }).click();

  await expect(page.getByText("OAuth resumed")).toBeVisible();
  expect(resumeCalls).toBe(1);
});

test("Account Center renders an actionable continuation failure without internal detail", async ({
  page,
}) => {
  await page.addInitScript(() => window.localStorage.clear());
  await page.route(
    "**/account-auth/account/bootstrap?flow=login&continuation=expired-handle",
    async (route) => {
      await route.fulfill({
        status: 410,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "OAUTH_CONTINUATION_UNAVAILABLE",
            message: "redis key and callback detail",
          },
          request_id: "req_browser_continuation_expired",
        }),
      });
    }
  );

  await page.goto("/account/login?continuation=expired-handle", {
    waitUntil: "networkidle",
  });
  await expect(
    page.getByRole("heading", { name: "登录链接已过期或不可继续" })
  ).toBeVisible();
  await expect(page.getByText("请求编号：req_browser_continuation_expired")).toBeVisible();
  await expect(page.getByRole("link", { name: "重新开始登录" })).toHaveAttribute(
    "href",
    "/api/v1/auth/login?return_to=%2Faccount"
  );
  await expect(page.getByText("redis key and callback detail")).toHaveCount(0);
});

test("Account Center renders a retryable continuation service failure with request id", async ({
  page,
}) => {
  await page.addInitScript(() => window.localStorage.clear());
  await page.route(
    "**/account-auth/account/bootstrap?flow=login&continuation=service-handle",
    async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "DEPENDENCY_UNAVAILABLE",
            message: "redis unavailable",
          },
          request_id: "req_browser_continuation_service",
        }),
      });
    }
  );

  await page.goto("/account/login?continuation=service-handle", {
    waitUntil: "networkidle",
  });
  await expect(page.getByRole("heading", { name: "登录暂时不可用" })).toBeVisible();
  await expect(page.getByText("暂时无法验证这次登录，请稍后重试。")).toBeVisible();
  await expect(page.getByText("请求编号：req_browser_continuation_service")).toBeVisible();
  await expect(page.getByRole("button", { name: "重新尝试" })).toBeVisible();
  await expect(page.getByText("redis unavailable")).toHaveCount(0);
});

test("Core-side continuation failure offers a fresh safe OAuth start", async ({
  page,
}) => {
  await page.goto(
    "/account/login?continuation_error=service&request_id=req_core_continuation_service",
    { waitUntil: "networkidle" }
  );

  await expect(page.getByRole("heading", { name: "登录暂时不可用" })).toBeVisible();
  await expect(
    page.getByRole("link", { name: "重新开始登录" })
  ).toHaveAttribute("href", "/api/v1/auth/login?return_to=%2Faccount");
});

test("OAuth continuation Account Center remains operable at 360px with reduced motion", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.addInitScript(() => window.localStorage.clear());
  await page.route(
    "**/account-auth/account/bootstrap?flow=login&continuation=mobile-handle",
    async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            flow: "login",
            csrf_token: "csrf-token-with-at-least-thirty-two-characters",
            continuation: { available: true, product_name: "HENU Kit" },
          },
          request_id: "req_mobile_continuation",
        }),
      });
    }
  );

  await page.goto("/account/login?continuation=mobile-handle", {
    waitUntil: "networkidle",
  });
  await expect(page.getByText("登录后继续前往 HENU Kit")).toBeVisible();
  await expect(page.getByRole("button", { name: "登 录" })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)
  ).toBe(true);
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toBeVisible();
});

import { expect, test, type Page } from "@playwright/test";

const sessionUserID = "11111111-1111-4111-8111-111111111111";

async function mockSession(page: Page) {
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: sessionUserID,
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    });
  });
}

const profile = {
  user_id: sessionUserID,
  target_roles: "后端开发",
  tech_stack: "go,postgres",
  locations: "郑州",
  job_type: "daily_intern",
  graduation_year: 2027,
  resume_text: "校内项目经历",
  email_notification_enabled: true,
  updated_at: "2026-08-15T00:00:00Z",
};

test("account profile page renders the career profile and saves the full replace", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/career/profile", async (route) => {
    if (route.request().method() === "PUT") {
      const input = await route.request().postDataJSON();
      expect(input).toEqual({
        target_roles: "后端开发",
        tech_stack: "go,postgres",
        locations: "郑州",
        job_type: "daily_intern",
        graduation_year: 2027,
        resume_text: "校内项目经历",
        email_notification_enabled: true,
      });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ profile, request_id: "req_profile_put" }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile_get" }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "求职画像" })).toBeVisible();
  const navigation = page.getByRole("link", { name: /A-08.*求职画像/ });
  await expect(navigation).toBeVisible();
  await expect(navigation).toHaveCSS("min-height", "44px");
  await expect(page.getByLabel("目标岗位 / 方向（≤500 字）")).toHaveValue("后端开发");
  await expect(page.getByLabel("技术栈关键词（≤1000 字）")).toHaveValue("go,postgres");
  await expect(page.getByLabel("目标城市（≤500 字）")).toHaveValue("郑州");
  await expect(page.getByLabel("求职类型")).toHaveValue("daily_intern");
  await expect(page.getByLabel("毕业年份（可空）")).toHaveValue("2027");
  await expect(page.getByLabel("经历摘要（≤4000 字）")).toHaveValue("校内项目经历");
  await expect(page.getByLabel("扫描结果邮件通知")).toBeChecked();

  await page.getByRole("button", { name: "保存画像" }).click();
  await expect(page.locator('[data-account-career-profile-save="success"]')).toBeVisible();
  await expect(page.getByText("求职画像已保存，将用于下一次求职雷达匹配。")).toBeVisible();
});

test("save button never double-submits while a save is pending", async ({ page }) => {
  await mockSession(page);
  let putCount = 0;
  let releasePut!: () => void;
  const putReleased = new Promise<void>((resolve) => {
    releasePut = resolve;
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ profile, request_id: "req_profile_get" }),
      });
      return;
    }
    putCount += 1;
    await putReleased;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile_put" }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toBeVisible();
  const saveButton = page.getByRole("button", { name: "保存画像" });
  await saveButton.click();
  await expect(saveButton).toBeDisabled();
  await saveButton.click({ force: true });
  await expect(saveButton).toHaveText("保存中…");
  releasePut();
  await expect(page.locator('[data-account-career-profile-save="success"]')).toBeVisible();
  expect(putCount).toBe(1);
});

test("a free member sees the lifetime gate instead of the profile form", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({
        error: "lifetime_required",
        message: "求职雷达需要 Lifetime VIP 会员",
        request_id: "req_profile_gate",
      }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-career-profile-state="locked"]')).toBeVisible();
  await expect(page.getByText("求职雷达需要 Lifetime VIP 会员")).toBeVisible();
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toHaveCount(0);
  const purchaseEntry = page.getByRole("link", { name: "前往会员权益开通" });
  await expect(purchaseEntry).toHaveAttribute("href", "/account/membership");
  await expect(purchaseEntry).toHaveCSS("min-height", "44px");
});

test("invalid graduation year blocks saving with a readable message", async ({ page }) => {
  await mockSession(page);
  let putCount = 0;
  await page.route("**/api/v1/career/profile", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ profile, request_id: "req_profile_get" }),
      });
      return;
    }
    putCount += 1;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile_put" }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toBeVisible();
  await page.getByLabel("毕业年份（可空）").fill("202");
  await page.getByRole("button", { name: "保存画像" }).click();
  await expect(page.locator('[data-account-career-profile-save="error"]')).toContainText("毕业年份需为 4 位数字");
  expect(putCount).toBe(0);
});

test("a failed profile read is a recoverable error, never a local profile", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "career_unavailable", request_id: "req_profile_down" }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-career-profile-state="error"]')).toBeVisible();
  await expect(page.getByText("画像加载不出来时，不会以本地或会话数据替代真实画像。")).toBeVisible();
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重新加载" })).toHaveCSS("min-height", "44px");
});

test("/career branches on membership and profile state", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "lifetime", lifetime: true }, request_id: "req_membership" }),
    });
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        profile: { user_id: sessionUserID, updated_at: "2026-08-15T00:00:00Z" },
        request_id: "req_empty_profile",
      }),
    });
  });
  await page.route("**/api/v1/career/searches", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ searches: [], request_id: "req_searches" }),
    });
  });

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="lifetime-no-profile"]')).toBeVisible();
  const link = page.getByRole("link", { name: "去设置求职画像 →" });
  await expect(link).toHaveAttribute("href", "/account/profile");
  await expect(link).toHaveCSS("min-height", "44px");
});

test("/career keeps free members off the scan entry and points at ¥9.9 membership", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "free", lifetime: false }, request_id: "req_membership" }),
    });
  });

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="free"]')).toBeVisible();
  await expect(page.getByText("¥9.9 开通 Lifetime VIP →")).toBeVisible();
  const buy = page.getByRole("link", { name: "¥9.9 开通 Lifetime VIP →" });
  await expect(buy).toHaveAttribute("href", "/account/membership");
  await expect(page.getByRole("button", { name: /开始扫描/ })).toHaveCount(0);
});

test("/career renders the ready view for a lifetime member with a complete profile", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "lifetime", lifetime: true }, request_id: "req_membership" }),
    });
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile" }),
    });
  });
  await page.route("**/api/v1/career/searches", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ searches: [], request_id: "req_searches" }),
    });
  });

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="lifetime-ready"]')).toBeVisible();
  await expect(page.getByText("后端开发")).toBeVisible();
  await expect(page.getByRole("button", { name: "开始扫描 →" })).toBeVisible();
  const history = page.getByRole("link", { name: "全部历史 →" });
  await expect(history).toHaveAttribute("href", "/career/history");
});

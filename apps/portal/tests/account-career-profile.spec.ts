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

test("resume suification previews without overwriting and applies only after confirmation", async ({ page }) => {
  await mockSession(page);
  let profilePutCount = 0;
  let suifyCount = 0;
  await page.route("**/api/v1/career/profile", async (route) => {
    if (route.request().method() === "PUT") profilePutCount += 1;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile_get" }),
    });
  });
  await page.route("**/api/v1/career/profile/suifications", async (route) => {
    suifyCount += 1;
    expect(route.request().headers()["idempotency-key"]).toMatch(/^career:suify-/);
    expect(await route.request().postDataJSON()).toEqual({ resume_text: "校内项目经历" });
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        draft: { resume_text: "校内项目经历（重点表达版）" },
        request_id: "req_career_suify",
      }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  const original = page.getByLabel("经历摘要（≤4000 字）");
  await expect(original).toHaveValue("校内项目经历");

  await page.getByRole("button", { name: "酥化" }).click();
  const preview = page.locator('[data-account-career-suification="preview"]');
  await expect(preview).toBeVisible();
  await expect(preview.getByLabel("酥化预览")).toHaveValue("校内项目经历（重点表达版）");
  await expect(original).toHaveValue("校内项目经历");
  expect(profilePutCount).toBe(0);

  await preview.getByRole("button", { name: "撤销" }).click();
  await expect(preview).toHaveCount(0);
  await expect(original).toHaveValue("校内项目经历");

  await page.getByRole("button", { name: "酥化" }).click();
  await expect(preview).toBeVisible();
  await preview.getByRole("button", { name: "应用" }).click();
  await expect(preview).toHaveCount(0);
  await expect(original).toHaveValue("校内项目经历（重点表达版）");
  expect(suifyCount).toBe(2);
  expect(profilePutCount).toBe(0);

  await page.getByRole("button", { name: "恢复原文" }).click();
  await expect(original).toHaveValue("校内项目经历");
  await expect(page.getByRole("button", { name: "恢复原文" })).toHaveCount(0);
});

test("a newly extracted resume invalidates an older in-flight Suification", async ({ page }) => {
  await mockSession(page);
  let releaseSuification!: () => void;
  const suificationReleased = new Promise<void>((resolve) => {
    releaseSuification = resolve;
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile, request_id: "req_profile_get" }),
    });
  });
  await page.route("**/api/v1/career/profile/suifications", async (route) => {
    await suificationReleased;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        draft: { resume_text: "旧内容的迟到酥化结果" },
        request_id: "req_suify_old",
      }),
    });
  });
  await page.route("**/api/v1/career/profile/extractions", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        extraction: {
          id: "22222222-2222-4222-8222-222222222222",
          status: "completed",
          user_id: sessionUserID,
          file_name: "new-resume.txt",
          extracted: { resume_text: "新识别经历" },
          created_at: "2026-08-24T00:00:00Z",
        },
        request_id: "req_extract_new",
      }),
    });
  });

  await page.goto("/account/profile", { waitUntil: "domcontentloaded" });
  const resumeText = page.getByLabel("经历摘要（≤4000 字）");
  await page.getByRole("button", { name: "酥化" }).click();
  await expect(page.getByRole("button", { name: "酥化中" })).toHaveText("酥化中…");
  await expect(page.getByRole("button", { name: "酥化中" })).toHaveAttribute("aria-busy", "true");

  await page.locator("#career-resume-upload").setInputFiles({
    name: "new-resume.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("new resume"),
  });
  await page.getByRole("button", { name: "上传并识别" }).click();
  await expect(page.locator('[data-account-career-extraction="done"]')).toBeVisible();
  await expect(resumeText).toHaveValue("新识别经历");

  releaseSuification();
  await expect(page.locator('[data-account-career-suification="preview"]')).toHaveCount(0);
  await expect(resumeText).toHaveValue("新识别经历");
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
  const saveButton = page.locator('button[type="submit"]');
  await expect(saveButton).toHaveAccessibleName("保存画像");
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
  const layout = page.locator("[data-career-layout]");
  await expect(layout).toBeVisible();
  await expect(layout).not.toHaveCSS("background-image", "none");
  await expect(page.locator('[data-career-state="lifetime-no-profile"]')).toBeVisible();
  const link = page.getByRole("link", { name: "去设置求职画像 →" });
  await expect(link).toHaveAttribute("href", "/account/profile");
  await expect(link).toHaveCSS("min-height", "44px");
});

test("saving an extracted profile makes client navigation back to radar ready without a hard reload", async ({ page }) => {
  await mockSession(page);
  let currentProfile: Record<string, unknown> = {
    user_id: sessionUserID,
    updated_at: "2026-08-15T00:00:00Z",
  };
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "lifetime", lifetime: true }, request_id: "req_membership" }),
    });
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    if (route.request().method() === "PUT") {
      currentProfile = {
        ...currentProfile,
        ...(await route.request().postDataJSON()),
        updated_at: "2026-08-15T00:01:00Z",
      };
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile: currentProfile, request_id: "req_profile" }),
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
  await page.getByRole("link", { name: "去设置求职画像 →" }).click();
  await expect(page.locator('[data-account-career-profile-state="ready"]')).toBeVisible();
  await page.getByLabel("目标岗位 / 方向（≤500 字）").fill("后端开发");
  await page.getByRole("button", { name: "保存画像" }).click();
  await expect(page.locator('[data-account-career-profile-save="success"]')).toBeVisible();

  await page.goBack({ waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="lifetime-ready"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "开始扫描 →" })).toBeVisible();
});

test("an already open radar tab refreshes when it becomes visible after profile saving", async ({ page }) => {
  await mockSession(page);
  let currentProfile: Record<string, unknown> = {
    user_id: sessionUserID,
    updated_at: "2026-08-15T00:00:00Z",
  };
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "lifetime", lifetime: true }, request_id: "req_membership" }),
    });
  });
  await page.route("**/api/v1/career/profile", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ profile: currentProfile, request_id: "req_profile" }),
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
  currentProfile = {
    ...currentProfile,
    target_roles: "后端开发",
    updated_at: "2026-08-15T00:01:00Z",
  };

  const otherPage = await page.context().newPage();
  await otherPage.goto("about:blank");
  await otherPage.bringToFront();
  await page.bringToFront();
  // Chromium's headless bringToFront does not emit a window focus event even
  // though real tab activation does, so dispatch that browser signal here.
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));

  await expect(page.locator('[data-career-state="lifetime-ready"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "开始扫描 →" })).toBeVisible();
  await otherPage.close();
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

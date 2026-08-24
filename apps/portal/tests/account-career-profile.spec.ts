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

const completedSearch = {
  id: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
  status: "completed",
  user_id: sessionUserID,
  has_email: true,
  digest_status: "sent",
  created_at: "2026-08-20T09:30:00Z",
  result: {
    source_count: 1,
    job_count: 24,
    matched_count: 3,
    summary: "本次扫描覆盖 1 个受控官方来源，命中 3 个达到阈值的岗位。",
    jobs: [] as unknown[],
  },
};

const runningSearch = {
  id: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff",
  status: "running",
  stage: "crawling",
  user_id: sessionUserID,
  has_email: false,
  created_at: "2026-08-23T10:00:00Z",
};

async function mockLifetimeCareer(
  page: Page,
  searches: Array<Record<string, unknown>> = []
) {
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
  await page.route("**/api/v1/career/searches/*", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ search: searches[0], request_id: "req_search" }),
    });
  });
  await page.route("**/api/v1/career/searches", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ searches, request_id: "req_searches" }),
    });
  });
}

// 页头曾经重复：页面渲染一次 SectionHeading（首页的 05 编号），每个状态视图
// 又渲染一次自己的 R-0x 眉标 + 标题，且整页没有 h1。锁死「每页正好一个 h1」。
test("career pages carry exactly one h1 and their own document title", async ({ page }) => {
  await mockLifetimeCareer(page, [completedSearch]);

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="lifetime-ready"]')).toBeVisible();
  await expect(page.locator("h1")).toHaveCount(1);
  await expect(page.locator("h1")).toHaveText("求职画像已就绪");
  await expect(page).toHaveTitle("求职雷达 | HENU Kit");

  await page.goto("/career/history", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-history-state="ready"]')).toBeVisible();
  await expect(page.locator("h1")).toHaveCount(1);
  await expect(page.locator("h1")).toHaveText("扫描历史");
  await expect(page).toHaveTitle("扫描历史 — henukit 求职雷达");
});

// 表盘只反映服务端确认的状态与推荐数：completed 时点亮 matched_count 个目标，
// 进行中不按进度估算点亮任何一个，装饰示意图完全不渲染读数。
test("the work radar dial reflects only server-confirmed scan facts", async ({ page }) => {
  await mockLifetimeCareer(page, [completedSearch]);
  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-scan-status="completed"]')).toBeVisible();

  const dial = page.locator('svg[aria-label="求职雷达状态：已完成"]');
  await expect(dial).toBeVisible();
  // result.matched_count === 3 → 三个目标点亮，其余保持未命中。
  await expect(dial.locator("[data-radar-blip]")).toHaveCount(3);
  await expect(dial.locator("[data-radar-ping]")).toHaveCount(3);
});

test("a running scan lights no targets, because the backend confirms no counts yet", async ({ page }) => {
  await mockLifetimeCareer(page, [runningSearch]);
  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-scan-status="running"]')).toBeVisible();

  const dial = page.locator('svg[aria-label="求职雷达状态：扫描中"]');
  await expect(dial).toBeVisible();
  // 进行中后端只返回 stage，没有任何计数，表盘不得按进度估算点亮目标。
  await expect(dial.locator("[data-radar-blip]")).toHaveCount(0);
});

test("the marketing radar is a schematic: labelled, aria-hidden, and lights nothing real", async ({ page }) => {
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({}) });
  });

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-state="anonymous"]')).toBeVisible();
  await expect(page.getByText("SCHEMATIC")).toBeVisible();
  // 整块装饰面板对辅助技术隐藏——包括表头那行 WORK RADAR / WR-01 · SCHEMATIC，
  // 它是 <svg> 的兄弟节点，只给 <svg> 加 aria-hidden 盖不住它。
  const panel = page.locator("[data-career-state='anonymous'] [aria-hidden='true']");
  await expect(panel).toHaveCount(1);
  await expect(panel.getByText("SCHEMATIC")).toHaveCount(1);
  await expect(panel.locator("svg")).toHaveCount(1);
  // 装饰表盘不冒充一次带状态的真实扫描。
  await expect(page.locator("svg[aria-label^='求职雷达状态']")).toHaveCount(0);
});

// 恢复 effect 曾经依赖 searches[0] 的对象身份，而父页面在窗口重新获得焦点时会
// 重新拉取历史、每次返回新数组 —— 于是切回标签页就把已恢复的任务整个重新恢复
// 一遍。用 completed 任务做探针：它的恢复路径必定读一次单条状态接口
// (`/searches/{id}`)，且完成后轮询即停，没有后台轮询来污染计数。
test("refocusing the tab does not re-restore an unchanged scan", async ({ page }) => {
  let statusReads = 0;
  let listReads = 0;
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
  await page.route("**/api/v1/career/searches/*", async (route) => {
    statusReads += 1;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ search: completedSearch, request_id: "req_search" }),
    });
  });
  await page.route("**/api/v1/career/searches", async (route) => {
    listReads += 1;
    // 每次都返回结构相同但对象身份全新的列表，模拟真实网关响应。
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ searches: [{ ...completedSearch }], request_id: "req_searches" }),
    });
  });

  await page.goto("/career", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-career-scan-status="completed"]')).toBeVisible();
  await expect.poll(() => statusReads).toBeGreaterThan(0);
  const statusReadsAfterFirstRestore = statusReads;
  const listReadsBeforeRefocus = listReads;

  // 父页面在 visibilitychange / focus 时重新解析整页状态。逐轮触发并等到列表
  // 确实被重新拉取，用真实信号代替固定等待。
  //
  // 走满两轮是有意的：listReads 在请求「到达」时自增，早于它要守的那个恢复
  // effect；只观察一轮的话，断言可能赶在 React 重渲染之前就跑完了。等到第二
  // 轮的请求发出，第一轮的重渲染与 effect 必然已经结束。
  for (let round = 0; round < 2; round += 1) {
    const listReadsBeforeRound = listReads;
    await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect.poll(() => listReads).toBeGreaterThan(listReadsBeforeRound);
  }
  expect(listReads).toBeGreaterThan(listReadsBeforeRefocus);

  // 刷新落地之后，恢复 effect 不应重跑：同一条任务的完整状态不会被再读一次。
  expect(statusReads).toBe(statusReadsAfterFirstRestore);
  await expect(page.getByRole("button", { name: "正在恢复任务…" })).toHaveCount(0);
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

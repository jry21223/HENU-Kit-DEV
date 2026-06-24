import { expect, test, type APIRequestContext, type BrowserContext, type Page } from "@playwright/test";

test.use({ channel: "chrome" });

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
};

type LoginData = {
  accessToken: string;
  user: {
    id: string;
    email: string;
    role: string;
  };
};

type BlogPost = {
  id: string;
  authorId: string;
  title: string;
  slug: string;
  content: string;
  status?: string;
};

type Report = {
  id: string;
  reporterId: string;
  targetType: string;
  targetId: string;
  reason: string;
  description: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
};

type NotificationItem = {
  id: string;
  userId: string;
  type: string;
  title: string;
  body: string;
  data?: unknown;
  readAt?: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  adminBaseURL: string;
  apiBaseURL: string;
  authorEmail: string;
  authorCode: string;
  reporterEmail: string;
  reporterCode: string;
  adminEmail: string;
  adminCode: string;
};

const cfg = readConfig();

test.describe("admin report-review browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_REPORT_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin report-review smoke.");

  test("reported public content is handled through admin review and reporter notification", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const author = await loginByAPI(context.request, cfg.apiBaseURL, cfg.authorEmail, cfg.authorCode, "Smoke Report Author");
      const reporter = await loginByAPI(context.request, cfg.apiBaseURL, cfg.reporterEmail, cfg.reporterCode, "Smoke Reporter");
      const admin = await loginByAPI(context.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const stamp = Date.now();
      const title = `Smoke Report Target ${stamp}`;
      const slug = `smoke-report-target-${stamp}`;
      const content = `Smoke report target content ${stamp}. Report review should not mutate this public blog content.`;

      const pendingPost = await createBlogPost(context.request, cfg.apiBaseURL, author.accessToken, { title, slug, content });
      expect(pendingPost.status).toBe("pending");
      await approveBlogPostByAPI(context.request, cfg.apiBaseURL, admin.accessToken, pendingPost.id);

      const publicBeforeReport = await apiGET<{ post: BlogPost }>(context.request, cfg.apiBaseURL, `/blog/posts/${pendingPost.id}`);
      expect(publicBeforeReport.post.title).toBe(title);
      expect(publicBeforeReport.post.content).toBe(content);

      const reportReason = `policy concern ${stamp}`;
      const reportDescription = `Smoke report description ${stamp}. Admin handling should notify reporter but not rewrite target content.`;
      const report = await createReport(context.request, cfg.apiBaseURL, reporter.accessToken, {
        targetType: "blog_post",
        targetId: pendingPost.id,
        reason: reportReason,
        description: reportDescription,
      });
      expect(report.status).toBe("pending");
      expect(report.targetType).toBe("blog_post");
      expect(report.targetId).toBe(pendingPost.id);

      const adminPage = await context.newPage();
      await prepareAdminSession(context, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/reports"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(reportReason).first()).toBeVisible();
      await expect(adminPage.getByText(pendingPost.id).first()).toBeVisible();
      await adminPage.getByTestId(`report-review-resolve-${report.id}`).click();
      const reviewReason = `smoke report handled ${stamp}`;
      await adminPage.locator(".el-dialog textarea").fill(reviewReason);
      const [resolveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/reports/${report.id}/resolve`) && response.request().method() === "POST",
        ),
        adminPage.getByTestId("report-review-submit").click(),
      ]);
      expect(resolveResponse.status()).toBe(200);

      const reviewedReport = await findReport(context.request, cfg.apiBaseURL, admin.accessToken, report.id);
      expect(reviewedReport.status).toBe("approved");
      expect(reviewedReport.reviewReason).toBe(reviewReason);
      expect(reviewedReport.reviewedAt ?? "").not.toBe("");
      expect(reviewedReport.reviewerId ?? "").not.toBe("");

      const reporterNotifications = await apiGETAuth<{ notifications: NotificationItem[]; unreadCount: number }>(
        context.request,
        cfg.apiBaseURL,
        "/me/notifications",
        reporter.accessToken,
      );
      const notification = reporterNotifications.notifications.find((item) => item.type === "report_result" && JSON.stringify(item).includes(report.id));
      expect(notification, "Reporter should receive a report_result notification for the handled report.").toBeTruthy();
      expect(JSON.stringify(notification), "Reporter notification must not leak reviewer identity.").not.toMatch(/reviewerId/i);
      expect(JSON.stringify(notification)).toContain("approved");
      expect(JSON.stringify(notification)).toContain(reviewReason);

      const publicAfterResolve = await apiGET<{ post: BlogPost }>(context.request, cfg.apiBaseURL, `/blog/posts/${pendingPost.id}`);
      expect(publicAfterResolve.post.title).toBe(title);
      expect(publicAfterResolve.post.content).toBe(content);
      expect(publicAfterResolve.post.status).toBeUndefined();

      const webPage = await context.newPage();
      await webPage.goto(joinURL(cfg.webBaseURL, `/blog/${pendingPost.id}`), { waitUntil: "networkidle" });
      await expect(webPage.getByText(title).first()).toBeVisible();
      await expect(webPage.getByText(content).first()).toBeVisible();
    } finally {
      await context.close();
    }
  });
});

async function loginByAPI(request: APIRequestContext, apiBaseURL: string, email: string, configuredCode: string, name: string) {
  const sendResponse = await request.post(joinURL(apiBaseURL, "/auth/send-code"), {
    data: { email },
  });
  expect(sendResponse.status()).toBe(200);
  const sendPayload = (await sendResponse.json()) as Envelope<{ devCode?: string }>;
  expect(sendPayload.code).toBe(0);

  const code = configuredCode || sendPayload.data.devCode || "";
  expect(code, `No verification code is available for ${email}. Set an E2E_*_CODE value outside development.`).not.toBe("");

  const loginResponse = await request.post(joinURL(apiBaseURL, "/auth/login"), {
    data: { email, code, name, grade: "2023" },
  });
  expect(loginResponse.status()).toBe(200);
  const loginPayload = (await loginResponse.json()) as Envelope<LoginData>;
  expect(loginPayload.code).toBe(0);
  expect(loginPayload.data.accessToken).not.toBe("");
  return loginPayload.data;
}

async function createBlogPost(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: { title: string; slug: string; content: string },
) {
  const response = await request.post(joinURL(apiBaseURL, "/blog/posts"), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ post: BlogPost }>;
  expect(payload.code).toBe(0);
  return payload.data.post;
}

async function approveBlogPostByAPI(request: APIRequestContext, apiBaseURL: string, token: string, postId: string) {
  const response = await request.post(joinURL(apiBaseURL, `/admin/blog/posts/${postId}/approve`), {
    data: { reviewReason: "smoke report target approval" },
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ reviewed: boolean; status: string }>;
  expect(payload.code).toBe(0);
  expect(payload.data.status).toBe("published");
}

async function createReport(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: { targetType: string; targetId: string; reason: string; description: string },
) {
  const response = await request.post(joinURL(apiBaseURL, "/reports"), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ report: Report; created: boolean }>;
  expect(payload.code).toBe(0);
  expect(payload.data.created).toBe(true);
  return payload.data.report;
}

async function findReport(request: APIRequestContext, apiBaseURL: string, token: string, reportId: string) {
  const response = await request.get(joinURL(apiBaseURL, "/admin/reports?status=all"), {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ reports: Report[] }>;
  expect(payload.code).toBe(0);
  const report = payload.data.reports.find((item) => item.id === reportId);
  expect(report, `Expected report ${reportId} to exist in admin report list.`).toBeTruthy();
  return report as Report;
}

async function prepareAdminSession(context: BrowserContext, page: Page, token: string) {
  await context.addInitScript((storedToken) => {
    window.localStorage.setItem("final-review-admin-token", storedToken);
  }, token);
  await page.addInitScript((storedToken) => {
    window.localStorage.setItem("final-review-admin-token", storedToken);
  }, token);
}

async function apiGET<T>(request: APIRequestContext, apiBaseURL: string, path: string): Promise<T> {
  const response = await request.get(joinURL(apiBaseURL, path));
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<T>;
  expect(payload.code).toBe(0);
  return payload.data;
}

async function apiGETAuth<T>(request: APIRequestContext, apiBaseURL: string, path: string, token: string): Promise<T> {
  const response = await request.get(joinURL(apiBaseURL, path), {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<T>;
  expect(payload.code).toBe(0);
  return payload.data;
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_REPORT_REVIEW_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    authorEmail: process.env.E2E_REPORT_REVIEW_AUTHOR_EMAIL ?? "smoke-report-author@stu.henu.edu.cn",
    authorCode: process.env.E2E_REPORT_REVIEW_AUTHOR_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
    reporterEmail: process.env.E2E_REPORT_REVIEW_REPORTER_EMAIL ?? "smoke-report-reporter@stu.henu.edu.cn",
    reporterCode: process.env.E2E_REPORT_REVIEW_REPORTER_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
    adminEmail: process.env.E2E_ADMIN_EMAIL ?? "admin@example.com",
    adminCode: process.env.E2E_ADMIN_CODE ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

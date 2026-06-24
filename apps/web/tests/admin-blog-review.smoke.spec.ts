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
  status: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  adminBaseURL: string;
  apiBaseURL: string;
  authorEmail: string;
  authorCode: string;
  adminEmail: string;
  adminCode: string;
};

const cfg = readConfig();

test.describe("admin blog-review browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin blog-review smoke.");

  test("pending blog post stays hidden until admin approval", async ({ browser }) => {
    const adminContext = await browser.newContext();
    try {
      const author = await loginByAPI(adminContext.request, cfg.apiBaseURL, cfg.authorEmail, cfg.authorCode, "Smoke Blog Author");
      const stamp = Date.now();
      const title = `Smoke Blog Review ${stamp}`;
      const slug = `smoke-blog-review-${stamp}`;
      const content = `Smoke review content ${stamp}. This post should become public only after reviewer approval.`;
      const pendingPost = await createBlogPost(adminContext.request, cfg.apiBaseURL, author.accessToken, { title, slug, content });
      expect(pendingPost.status).toBe("pending");

      const hiddenBefore = await adminContext.request.get(joinURL(cfg.apiBaseURL, `/blog/posts/${pendingPost.id}`));
      expect(hiddenBefore.status(), "Pending blog post must not be public before approval.").toBe(404);

      const admin = await loginByAPI(adminContext.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const adminPage = await adminContext.newPage();
      await prepareAdminSession(adminContext, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/blog-reviews"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(title).first()).toBeVisible();
      await adminPage.getByTestId(`blog-review-approve-${pendingPost.id}`).click();
      await expect(adminPage.getByText(title).first()).toBeVisible();
      const [approveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/blog/posts/${pendingPost.id}/approve`) && response.request().method() === "POST",
        ),
        adminPage.getByTestId("blog-review-submit").click(),
      ]);
      expect(approveResponse.status()).toBe(200);

      const published = await apiGET<{ post: BlogPost }>(adminContext.request, cfg.apiBaseURL, `/blog/posts/${pendingPost.id}`);
      expect(published.post.status).toBeUndefined();
      expect(published.post.title).toBe(title);

      const webPage = await adminContext.newPage();
      await webPage.goto(joinURL(cfg.webBaseURL, `/blog/${pendingPost.id}`), { waitUntil: "networkidle" });
      await expect(webPage.getByText(title).first()).toBeVisible();
      await expect(webPage.getByText(content).first()).toBeVisible();
    } finally {
      await adminContext.close();
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

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_REVIEW_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    authorEmail: process.env.E2E_REVIEW_AUTHOR_EMAIL ?? "smoke-review-author@stu.henu.edu.cn",
    authorCode: process.env.E2E_REVIEW_AUTHOR_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
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

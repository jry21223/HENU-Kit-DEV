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

type ForumBoard = {
  id: string;
  name: string;
  status: string;
};

type ForumPost = {
  id: string;
  authorId: string;
  boardId: string;
  title: string;
  content: string;
  type: string;
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
  boardID: string;
};

const cfg = readConfig();

test.describe("admin forum-review browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_FORUM_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin forum-review smoke.");

  test("pending forum post stays hidden until admin approval", async ({ browser }) => {
    const adminContext = await browser.newContext();
    try {
      const board = await selectForumBoard(adminContext.request, cfg);
      const author = await loginByAPI(adminContext.request, cfg.apiBaseURL, cfg.authorEmail, cfg.authorCode, "Smoke Forum Author");
      const stamp = Date.now();
      const title = `Smoke Forum Review ${stamp}`;
      const content = `Smoke forum review content ${stamp}. This post should become public only after reviewer approval.`;
      const pendingPost = await createForumPost(adminContext.request, cfg.apiBaseURL, author.accessToken, {
        boardId: board.id,
        title,
        content,
        type: "question",
      });
      expect(pendingPost.status).toBe("pending");

      const hiddenBefore = await adminContext.request.get(joinURL(cfg.apiBaseURL, `/forum/posts/${pendingPost.id}`));
      expect(hiddenBefore.status(), "Pending forum post must not be public before approval.").toBe(404);

      const admin = await loginByAPI(adminContext.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const adminPage = await adminContext.newPage();
      await prepareAdminSession(adminContext, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/forum-reviews"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(title).first()).toBeVisible();
      await adminPage.getByTestId(`forum-review-approve-${pendingPost.id}`).click();
      await expect(adminPage.getByText(title).first()).toBeVisible();
      const [approveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/forum/posts/${pendingPost.id}/approve`) && response.request().method() === "POST",
        ),
        adminPage.getByTestId("forum-review-submit").click(),
      ]);
      expect(approveResponse.status()).toBe(200);

      const published = await apiGET<{ post: ForumPost }>(adminContext.request, cfg.apiBaseURL, `/forum/posts/${pendingPost.id}`);
      expect(published.post.status).toBe("published");
      expect(published.post.title).toBe(title);
      expect(JSON.stringify(published), "Public forum detail must hide review metadata.").not.toMatch(/reviewerId|reviewReason/i);

      const publicList = await apiGET<{ posts: ForumPost[] }>(adminContext.request, cfg.apiBaseURL, `/forum/posts?boardId=${board.id}`);
      expect(publicList.posts.some((post) => post.id === pendingPost.id)).toBe(true);

      const webPage = await adminContext.newPage();
      await webPage.goto(joinURL(cfg.webBaseURL, `/forum/${pendingPost.id}`), { waitUntil: "networkidle" });
      await expect(webPage.getByText(title).first()).toBeVisible();
      await expect(webPage.getByText(content).first()).toBeVisible();
    } finally {
      await adminContext.close();
    }
  });
});

async function selectForumBoard(request: APIRequestContext, config: SmokeConfig): Promise<ForumBoard> {
  const data = await apiGET<{ boards: ForumBoard[] }>(request, config.apiBaseURL, "/forum/boards");
  const board = config.boardID ? data.boards.find((item) => item.id === config.boardID) : data.boards[0];
  if (!board) {
    throw new Error("No published forum board was found. Seed or create a board before running forum review smoke.");
  }
  return board;
}

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

async function createForumPost(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: { boardId: string; title: string; content: string; type: string },
) {
  const response = await request.post(joinURL(apiBaseURL, "/forum/posts"), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ post: ForumPost }>;
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
    enabled: process.env.E2E_FORUM_REVIEW_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    authorEmail: process.env.E2E_FORUM_REVIEW_AUTHOR_EMAIL ?? process.env.E2E_REVIEW_AUTHOR_EMAIL ?? "smoke-forum-author@stu.henu.edu.cn",
    authorCode: process.env.E2E_FORUM_REVIEW_AUTHOR_CODE ?? process.env.E2E_REVIEW_AUTHOR_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
    adminEmail: process.env.E2E_ADMIN_EMAIL ?? "admin@example.com",
    adminCode: process.env.E2E_ADMIN_CODE ?? "",
    boardID: process.env.E2E_FORUM_BOARD_ID ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

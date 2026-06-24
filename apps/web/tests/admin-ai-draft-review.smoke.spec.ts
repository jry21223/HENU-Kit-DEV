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

type AITask = {
  id: string;
  userId?: string;
  type: string;
  status: string;
  input?: unknown;
  result?: unknown;
};

type AIDraft = {
  id: string;
  taskId: string;
  outputType: string;
  status: string;
  draftContent?: unknown;
  publishedId?: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
};

type SmokeConfig = {
  enabled: boolean;
  adminBaseURL: string;
  apiBaseURL: string;
  studentEmail: string;
  studentCode: string;
  adminEmail: string;
  adminCode: string;
  timeoutMs: number;
};

const cfg = readConfig();

test.describe("admin AI draft-review browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_AI_DRAFT_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin AI draft-review smoke.");

  test("worker-generated AI draft can be reviewed without auto-publishing", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const student = await loginByAPI(context.request, cfg.apiBaseURL, cfg.studentEmail, cfg.studentCode, "Smoke AI Student");
      const admin = await loginByAPI(context.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const stamp = Date.now();
      const task = await createAITask(context.request, cfg.apiBaseURL, student.accessToken, {
        type: "paper_generation",
        input: {
          smoke: "ai-draft-review",
          marker: `smoke-ai-draft-review-${stamp}`,
          requirement: "Generate a draft that must be reviewed before publication.",
        },
      });
      expect(task.status).toBe("pending");

      const draft = await waitForDraft(context.request, cfg.apiBaseURL, admin.accessToken, task.id, cfg.timeoutMs);
      expect(draft.status).toBe("pending");
      expect(draft.outputType).toBe("paper_generation");
      expect(draft.publishedId ?? "").toBe("");
      expect(JSON.stringify(draft.draftContent ?? {})).toContain(task.id);

      const adminPage = await context.newPage();
      await prepareAdminSession(context, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/ai/drafts"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(draft.id).first()).toBeVisible();
      await expect(adminPage.getByText(task.id).first()).toBeVisible();
      await adminPage.getByTestId(`ai-draft-review-approve-${draft.id}`).click();
      const [approveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/ai/drafts/${draft.id}/approve`) && response.request().method() === "POST",
        ),
        adminPage.getByTestId("ai-draft-review-submit").click(),
      ]);
      expect(approveResponse.status()).toBe(200);

      const approved = await findDraft(context.request, cfg.apiBaseURL, admin.accessToken, draft.id);
      expect(approved.status).toBe("approved");
      expect(approved.reviewedAt ?? "").not.toBe("");
      expect(approved.reviewerId ?? "").not.toBe("");
      expect(approved.publishedId ?? "").toBe("");
      expect(approved.status).not.toBe("published");
      await expect(adminPage.getByTestId(`ai-draft-review-reject-${draft.id}`)).toBeDisabled();
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

async function createAITask(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: { type: string; input: Record<string, unknown> },
) {
  const response = await request.post(joinURL(apiBaseURL, "/ai/tasks"), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ task: AITask; enqueued: boolean }>;
  expect(payload.code).toBe(0);
  return payload.data.task;
}

async function waitForDraft(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  taskId: string,
  timeoutMs: number,
) {
  const startedAt = Date.now();
  let lastDrafts: AIDraft[] = [];
  while (Date.now() - startedAt < timeoutMs) {
    const drafts = await listDrafts(request, apiBaseURL, token);
    lastDrafts = drafts;
    const draft = drafts.find((item) => item.taskId === taskId);
    if (draft) return draft;
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(
    `Timed out waiting for AI draft for task ${taskId}. Confirm the worker is running, Redis is reachable, and LLM_MODE=mock is configured. Last draft count: ${lastDrafts.length}`,
  );
}

async function findDraft(request: APIRequestContext, apiBaseURL: string, token: string, draftId: string) {
  const drafts = await listDrafts(request, apiBaseURL, token);
  const draft = drafts.find((item) => item.id === draftId);
  expect(draft, `Expected AI draft ${draftId} to exist in admin draft list.`).toBeTruthy();
  return draft as AIDraft;
}

async function listDrafts(request: APIRequestContext, apiBaseURL: string, token: string) {
  const response = await request.get(joinURL(apiBaseURL, "/admin/ai/drafts"), {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ drafts: AIDraft[] }>;
  expect(payload.code).toBe(0);
  return payload.data.drafts;
}

async function prepareAdminSession(context: BrowserContext, page: Page, token: string) {
  await context.addInitScript((storedToken) => {
    window.localStorage.setItem("final-review-admin-token", storedToken);
  }, token);
  await page.addInitScript((storedToken) => {
    window.localStorage.setItem("final-review-admin-token", storedToken);
  }, token);
}

function readConfig(): SmokeConfig {
  const timeoutSeconds = Number.parseInt(process.env.E2E_AI_DRAFT_REVIEW_TIMEOUT_SECONDS ?? "60", 10);
  return {
    enabled: process.env.E2E_AI_DRAFT_REVIEW_SMOKE === "1",
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    studentEmail: process.env.E2E_AI_DRAFT_REVIEW_STUDENT_EMAIL ?? process.env.E2E_STUDENT_EMAIL ?? "smoke-ai@stu.henu.edu.cn",
    studentCode: process.env.E2E_AI_DRAFT_REVIEW_STUDENT_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
    adminEmail: process.env.E2E_ADMIN_EMAIL ?? "admin@example.com",
    adminCode: process.env.E2E_ADMIN_CODE ?? "",
    timeoutMs: (Number.isFinite(timeoutSeconds) && timeoutSeconds > 0 ? timeoutSeconds : 60) * 1000,
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

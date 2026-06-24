import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

test.use({ channel: "chrome" });

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
};

type WikiCreatorApplication = {
  id: string;
  reason: string;
  sampleTitle: string;
  sampleBody: string;
  status: string;
  reviewReason?: string;
  createdAt: string;
  updatedAt: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  apiBaseURL: string;
  studentEmail: string;
  studentCode: string;
};

const cfg = readConfig();

test.describe("wiki creator application browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_WIKI_CREATOR_APPLICATION_SMOKE=1 plus E2E_* URLs/account to run.");

  test("student submits creator application and sees own pending status", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const page = await context.newPage();
      await loginStudent(page, cfg.studentEmail, cfg.studentCode);

      await page.goto(joinURL(cfg.webBaseURL, "/wiki"), { waitUntil: "networkidle" });
      await expect(page.getByTestId("wiki-creator-application-panel")).toBeVisible();

      const stamp = Date.now();
      const reason = `Smoke creator application reason ${stamp}`;
      const sampleTitle = `Smoke Creator Sample ${stamp}`;
      const sampleBody = `Smoke creator sample body ${stamp}. This draft is used only for application review smoke testing.`;

      await page.getByTestId("wiki-creator-application-reason").fill(reason);
      await page.getByTestId("wiki-creator-application-sample-title").fill(sampleTitle);
      await page.getByTestId("wiki-creator-application-sample-body").fill(sampleBody);

      const [submitResponse] = await Promise.all([
        page.waitForResponse((response) => response.url().includes("/wiki/creator-applications") && response.request().method() === "POST"),
        page.getByTestId("wiki-creator-application-submit").click(),
      ]);
      expect(submitResponse.status()).toBe(200);
      const submitPayload = (await submitResponse.json()) as Envelope<{ application: WikiCreatorApplication }>;
      expect(submitPayload.code).toBe(0);
      expect(submitPayload.data.application.status).toBe("pending");
      expect(submitPayload.data.application.sampleTitle).toBe(sampleTitle);

      await expect(page.getByText(sampleTitle).first()).toBeVisible();

      const mine = await apiGET<{ applications: WikiCreatorApplication[] }>(
        context.request,
        cfg.apiBaseURL,
        "/wiki/creator-applications/me",
      );
      expect(mine.applications.some((application) => application.id === submitPayload.data.application.id)).toBe(true);
      expect(JSON.stringify(mine), "Self creator application list must not expose reviewer IDs.").not.toMatch(/reviewerId/i);
    } finally {
      await context.close();
    }
  });
});

async function loginStudent(page: Page, email: string, code: string) {
  await page.goto(joinURL(cfg.webBaseURL, "/login"), { waitUntil: "networkidle" });
  await page.locator('input[type="email"]').first().fill(email);
  await page.getByRole("button", { name: "鍙戦€侀獙璇佺爜" }).click();
  const codeInput = page.getByPlaceholder("123456").first();
  if (code) {
    await codeInput.fill(code);
  } else {
    await expect(codeInput, "No E2E_STUDENT_CODE was provided and the API did not return a development code.").not.toHaveValue("");
  }
  await page.getByRole("button", { name: "鐧诲綍" }).click();
  await expect(page.getByText(email)).toBeVisible();
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
    enabled: process.env.E2E_WIKI_CREATOR_APPLICATION_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    studentEmail: process.env.E2E_WIKI_CREATOR_APPLICATION_EMAIL ?? `smoke-wiki-creator-${Date.now()}@stu.henu.edu.cn`,
    studentCode: process.env.E2E_WIKI_CREATOR_APPLICATION_CODE ?? process.env.E2E_STUDENT_CODE ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

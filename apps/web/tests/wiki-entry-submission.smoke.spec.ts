import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

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

type WikiEntry = {
  id: string;
  title: string;
  slug: string;
  content: string;
  status: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  apiBaseURL: string;
  creatorEmail: string;
  creatorCode: string;
};

const cfg = readConfig();

test.describe("wiki entry submission browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_WIKI_ENTRY_SUBMISSION_SMOKE=1 plus E2E_* URLs/creator account to run.");

  test("creator submits a pending wiki entry that stays hidden publicly", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      await loginByAPI(context.request, cfg.apiBaseURL, cfg.creatorEmail, cfg.creatorCode, "Smoke Wiki Creator");
      const page = await context.newPage();
      await loginWeb(page, cfg.creatorEmail, cfg.creatorCode);

      const stamp = Date.now();
      const title = `Smoke Wiki Entry ${stamp}`;
      const slug = `smoke-wiki-entry-${stamp}`;
      const content = `Smoke wiki entry content ${stamp}. This entry must stay hidden until reviewer approval.`;

      await page.goto(joinURL(cfg.webBaseURL, "/wiki/new"), { waitUntil: "networkidle" });
      await expect(page.getByTestId("wiki-entry-submission-form")).toBeVisible();
      await page.getByTestId("wiki-entry-title").fill(title);
      await page.getByTestId("wiki-entry-slug").fill(slug);
      await page.getByTestId("wiki-entry-content").fill(content);
      await page.getByTestId("wiki-entry-summary").fill("smoke initial wiki submission");

      const [submitResponse] = await Promise.all([
        page.waitForResponse((response) => response.url().includes("/wiki/entries") && response.request().method() === "POST"),
        page.getByTestId("wiki-entry-submit").click(),
      ]);
      expect(submitResponse.status()).toBe(200);
      const submitPayload = (await submitResponse.json()) as Envelope<{ entry: WikiEntry }>;
      expect(submitPayload.code).toBe(0);
      expect(submitPayload.data.entry.title).toBe(title);
      expect(submitPayload.data.entry.slug).toBe(slug);
      expect(submitPayload.data.entry.status).toBe("pending");

      const publicDetail = await context.request.get(joinURL(cfg.apiBaseURL, `/wiki/entries/${submitPayload.data.entry.id}`));
      expect(publicDetail.status(), "Pending wiki entries must not be readable through public detail API.").toBe(404);
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
  expect(code, `No verification code is available for ${email}. Set an E2E_CREATOR_CODE value outside development.`).not.toBe("");

  const loginResponse = await request.post(joinURL(apiBaseURL, "/auth/login"), {
    data: { email, code, name, grade: "2023" },
  });
  expect(loginResponse.status()).toBe(200);
  const loginPayload = (await loginResponse.json()) as Envelope<LoginData>;
  expect(loginPayload.code).toBe(0);
  return loginPayload.data;
}

async function loginWeb(page: Page, email: string, code: string) {
  await page.goto(joinURL(cfg.webBaseURL, "/login"), { waitUntil: "networkidle" });
  await page.locator('input[type="email"]').first().fill(email);
  await page.getByRole("button", { name: "鍙戦€侀獙璇佺爜" }).click();
  const codeInput = page.getByPlaceholder("123456").first();
  if (code) {
    await codeInput.fill(code);
  } else {
    await expect(codeInput, "No E2E_CREATOR_CODE was provided and the API did not return a development code.").not.toHaveValue("");
  }
  await page.getByRole("button", { name: "鐧诲綍" }).click();
  await expect(page.getByText(email)).toBeVisible();
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_WIKI_ENTRY_SUBMISSION_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    creatorEmail: process.env.E2E_WIKI_ENTRY_CREATOR_EMAIL ?? "creator@example.com",
    creatorCode: process.env.E2E_WIKI_ENTRY_CREATOR_CODE ?? process.env.E2E_CREATOR_CODE ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

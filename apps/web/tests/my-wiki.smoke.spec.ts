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

test.describe("my wiki browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_MY_WIKI_SMOKE=1 plus E2E_* URLs/creator account to run.");

  test("creator can see own pending wiki entry in /me/wiki", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const creator = await loginByAPI(context.request, cfg.apiBaseURL, cfg.creatorEmail, cfg.creatorCode, "Smoke Wiki Creator");
      const stamp = Date.now();
      const title = `Smoke My Wiki ${stamp}`;
      const entry = await createWikiEntry(context.request, cfg.apiBaseURL, creator.accessToken, {
        title,
        slug: `smoke-my-wiki-${stamp}`,
        content: `Smoke my wiki content ${stamp}. This should be visible only in the owner's submission tracker until approved.`,
        summary: "my wiki smoke",
      });
      expect(entry.status).toBe("pending");

      const page = await context.newPage();
      await loginWeb(page, cfg.creatorEmail, cfg.creatorCode);
      await page.goto(joinURL(cfg.webBaseURL, "/me/wiki"), { waitUntil: "networkidle" });
      await expect(page.getByText(title).first()).toBeVisible();
      await expect(page.getByText("pending").first()).not.toBeVisible();
      await expect(page.getByText("待审").first()).toBeVisible();
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

async function createWikiEntry(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: { title: string; slug: string; content: string; summary: string },
) {
  const response = await request.post(joinURL(apiBaseURL, "/wiki/entries"), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ entry: WikiEntry }>;
  expect(payload.code).toBe(0);
  return payload.data.entry;
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
    enabled: process.env.E2E_MY_WIKI_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    creatorEmail: process.env.E2E_MY_WIKI_CREATOR_EMAIL ?? "creator@example.com",
    creatorCode: process.env.E2E_MY_WIKI_CREATOR_CODE ?? process.env.E2E_CREATOR_CODE ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

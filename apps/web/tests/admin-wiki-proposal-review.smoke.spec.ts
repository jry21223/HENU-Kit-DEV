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

type WikiEntry = {
  id: string;
  authorId: string;
  courseId?: string;
  title: string;
  slug: string;
  content: string;
  status?: string;
  version: number;
};

type WikiEditProposal = {
  id: string;
  entryId: string;
  editorId: string;
  baseVersion: number;
  proposedTitle: string;
  proposedContent: string;
  summary: string;
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

test.describe("admin wiki-proposal-review browser smoke", () => {
  test.skip(
    !cfg.enabled,
    "Set E2E_WIKI_PROPOSAL_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin wiki proposal-review smoke.",
  );

  test("published wiki content changes only after proposal approval", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const author = await loginByAPI(context.request, cfg.apiBaseURL, cfg.authorEmail, cfg.authorCode, "Smoke Wiki Proposal Author");
      const admin = await loginByAPI(context.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const stamp = Date.now();
      const baseTitle = `Smoke Wiki Proposal Base ${stamp}`;
      const baseSlug = `smoke-wiki-proposal-base-${stamp}`;
      const baseContent = `Smoke wiki proposal base content ${stamp}. This is the approved public baseline.`;
      const proposedTitle = `Smoke Wiki Proposal Approved ${stamp}`;
      const proposedContent = `Smoke wiki proposal approved content ${stamp}. This content should appear only after proposal approval.`;

      const pendingEntry = await createWikiEntry(context.request, cfg.apiBaseURL, author.accessToken, {
        title: baseTitle,
        slug: baseSlug,
        content: baseContent,
        summary: "smoke proposal baseline",
      });
      expect(pendingEntry.status).toBe("pending");

      await approveWikiEntryByAPI(context.request, cfg.apiBaseURL, admin.accessToken, pendingEntry.id);
      const baseline = await apiGET<{ entry: WikiEntry }>(context.request, cfg.apiBaseURL, `/wiki/entries/${pendingEntry.id}`);
      expect(baseline.entry.title).toBe(baseTitle);
      expect(baseline.entry.content).toBe(baseContent);
      expect(baseline.entry.version).toBe(1);

      const proposal = await createWikiProposal(context.request, cfg.apiBaseURL, author.accessToken, pendingEntry.id, {
        title: proposedTitle,
        content: proposedContent,
        summary: "smoke approved proposal",
      });
      expect(proposal.status).toBe("pending");
      expect(proposal.baseVersion).toBe(1);

      const publicBeforeApproval = await apiGET<{ entry: WikiEntry }>(
        context.request,
        cfg.apiBaseURL,
        `/wiki/entries/${pendingEntry.id}`,
      );
      expect(publicBeforeApproval.entry.title).toBe(baseTitle);
      expect(publicBeforeApproval.entry.content).toBe(baseContent);
      expect(publicBeforeApproval.entry.title).not.toBe(proposedTitle);
      expect(publicBeforeApproval.entry.content).not.toContain(proposedContent);

      const adminPage = await context.newPage();
      await prepareAdminSession(context, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/wiki-proposal-reviews"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(proposedTitle).first()).toBeVisible();
      await adminPage.getByTestId(`wiki-proposal-review-approve-${proposal.id}`).click();
      await expect(adminPage.getByText(proposedTitle).first()).toBeVisible();
      const [approveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/wiki/proposals/${proposal.id}/approve`) &&
            response.request().method() === "POST",
        ),
        adminPage.getByTestId("wiki-proposal-review-submit").click(),
      ]);
      expect(approveResponse.status()).toBe(200);

      const published = await apiGET<{ entry: WikiEntry }>(context.request, cfg.apiBaseURL, `/wiki/entries/${pendingEntry.id}`);
      expect(published.entry.title).toBe(proposedTitle);
      expect(published.entry.content).toBe(proposedContent);
      expect(published.entry.version).toBe(2);
      expect(published.entry.status).toBeUndefined();
      expect(JSON.stringify(published), "Public wiki detail must hide review metadata.").not.toMatch(
        /reviewerId|reviewedAt|reviewReason/i,
      );

      const webPage = await context.newPage();
      await webPage.goto(joinURL(cfg.webBaseURL, `/wiki/${pendingEntry.id}`), { waitUntil: "networkidle" });
      await expect(webPage.getByText(proposedTitle).first()).toBeVisible();
      await expect(webPage.getByText(proposedContent).first()).toBeVisible();
      await expect(webPage.getByText(baseTitle).first()).toHaveCount(0);
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

async function approveWikiEntryByAPI(request: APIRequestContext, apiBaseURL: string, token: string, entryId: string) {
  const response = await request.post(joinURL(apiBaseURL, `/admin/wiki/entries/${entryId}/approve`), {
    data: { reviewReason: "smoke baseline approval" },
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ reviewed: boolean; status: string }>;
  expect(payload.code).toBe(0);
  expect(payload.data.status).toBe("published");
}

async function createWikiProposal(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  entryId: string,
  input: { title: string; content: string; summary: string },
) {
  const response = await request.post(joinURL(apiBaseURL, `/wiki/entries/${entryId}/proposals`), {
    data: input,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ proposal: WikiEditProposal }>;
  expect(payload.code).toBe(0);
  return payload.data.proposal;
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
    enabled: process.env.E2E_WIKI_PROPOSAL_REVIEW_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    authorEmail: process.env.E2E_WIKI_PROPOSAL_REVIEW_AUTHOR_EMAIL ?? "creator@example.com",
    authorCode:
      process.env.E2E_WIKI_PROPOSAL_REVIEW_AUTHOR_CODE ??
      process.env.E2E_WIKI_REVIEW_AUTHOR_CODE ??
      process.env.E2E_REVIEW_AUTHOR_CODE ??
      process.env.E2E_STUDENT_CODE ??
      "",
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

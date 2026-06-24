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

type Course = {
  id: string;
  name: string;
  status: string;
};

type Material = {
  id: string;
  courseId: string;
  title: string;
  type: string;
  description: string;
  fileName: string;
  fileSize: number;
  previewContent: string;
  accessLevel: string;
  status?: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  adminBaseURL: string;
  apiBaseURL: string;
  adminEmail: string;
  adminCode: string;
  courseID: string;
};

const cfg = readConfig();

test.describe("admin material-review browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_MATERIAL_REVIEW_SMOKE=1 plus E2E_* URLs/accounts to run the admin material-review smoke.");

  test("pending material stays hidden until admin approval", async ({ browser }) => {
    const adminContext = await browser.newContext();
    try {
      const course = await selectCourse(adminContext.request, cfg);
      const admin = await loginByAPI(adminContext.request, cfg.apiBaseURL, cfg.adminEmail, cfg.adminCode, "Smoke Admin");
      const stamp = Date.now();
      const title = `Smoke Material Review ${stamp}`;
      const previewContent = `Smoke material review preview ${stamp}. This material should become public only after reviewer approval.`;
      const pendingMaterial = await createPendingMaterial(adminContext.request, cfg.apiBaseURL, admin.accessToken, {
        courseId: course.id,
        title,
        description: "Created by material review smoke; safe to delete.",
        fileName: `smoke-material-review-${stamp}.txt`,
        fileSize: 128,
        previewContent,
        storageKey: `materials/smoke/smoke-material-review-${stamp}.txt`,
      });
      expect(pendingMaterial.status).toBe("pending");

      const hiddenBefore = await adminContext.request.get(joinURL(cfg.apiBaseURL, `/materials/${pendingMaterial.id}`));
      expect(hiddenBefore.status(), "Pending material must not be public before approval.").toBe(404);

      const adminPage = await adminContext.newPage();
      await prepareAdminSession(adminContext, adminPage, admin.accessToken);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/material-reviews"), { waitUntil: "networkidle" });

      await expect(adminPage.getByText(title).first()).toBeVisible();
      await adminPage.getByTestId(`material-review-approve-${pendingMaterial.id}`).click();
      await expect(adminPage.getByText(title).first()).toBeVisible();
      const [approveResponse] = await Promise.all([
        adminPage.waitForResponse(
          (response) =>
            response.url().includes(`/admin/materials/${pendingMaterial.id}/approve`) && response.request().method() === "POST",
        ),
        adminPage.getByTestId("material-review-submit").click(),
      ]);
      expect(approveResponse.status()).toBe(200);

      const published = await apiGET<{ material: Material }>(adminContext.request, cfg.apiBaseURL, `/materials/${pendingMaterial.id}`);
      expect(published.material.title).toBe(title);
      expect(published.material.status).toBe("published");
      expect(published.material.previewContent).toBe(previewContent);
      expect(JSON.stringify(published), "Public material detail must hide file storage and review metadata.").not.toMatch(
        /storageKey|createdBy|reviewerId|reviewedAt|reviewReason/i,
      );

      const webPage = await adminContext.newPage();
      await webPage.goto(joinURL(cfg.webBaseURL, `/materials/${pendingMaterial.id}`), { waitUntil: "networkidle" });
      await expect(webPage.getByText(title).first()).toBeVisible();
      await expect(webPage.getByText(previewContent).first()).toBeVisible();
    } finally {
      await adminContext.close();
    }
  });
});

async function selectCourse(request: APIRequestContext, config: SmokeConfig): Promise<Course> {
  const data = await apiGET<{ courses: Course[] }>(request, config.apiBaseURL, "/courses");
  const course = config.courseID ? data.courses.find((item) => item.id === config.courseID) : data.courses[0];
  if (!course) {
    throw new Error("No published course was found. Seed or create a published course before running material review smoke.");
  }
  return course;
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

async function createPendingMaterial(
  request: APIRequestContext,
  apiBaseURL: string,
  token: string,
  input: {
    courseId: string;
    title: string;
    description: string;
    fileName: string;
    fileSize: number;
    previewContent: string;
    storageKey: string;
  },
) {
  const response = await request.post(joinURL(apiBaseURL, "/admin/materials"), {
    data: {
      ...input,
      type: "knowledge_note",
      accessLevel: "free",
      status: "pending",
    },
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ material: Material }>;
  expect(payload.code).toBe(0);
  return payload.data.material;
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
    enabled: process.env.E2E_MATERIAL_REVIEW_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    adminEmail: process.env.E2E_ADMIN_EMAIL ?? "admin@example.com",
    adminCode: process.env.E2E_ADMIN_CODE ?? "",
    courseID: process.env.E2E_MATERIAL_REVIEW_COURSE_ID ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

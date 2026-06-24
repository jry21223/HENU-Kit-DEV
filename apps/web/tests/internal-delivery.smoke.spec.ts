import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

test.use({ channel: "chrome" });

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
};

type CoursePackage = {
  id: string;
  title: string;
  status: string;
  priceFen: number;
  currency?: string;
};

type Material = {
  id: string;
  title: string;
  accessLevel: string;
  status: string;
};

type PackageListData = {
  packages: CoursePackage[];
};

type PackageDetailData = {
  package: CoursePackage;
  materials: Material[];
};

type User = {
  id: string;
  email: string;
  role: string;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  adminBaseURL: string;
  apiBaseURL: string;
  studentEmail: string;
  studentCode: string;
  adminEmail: string;
  adminCode: string;
  packageID: string;
};

type SelectedPackage = {
  coursePackage: CoursePackage;
  paidMaterial: Material;
};

const cfg = readConfig();

test.describe("internal delivery browser smoke", () => {
  test.skip(
    !cfg.enabled,
    "Set E2E_DELIVERY_SMOKE=1 plus E2E_* URLs/accounts to run the target-environment delivery smoke.",
  );

  test("paid package material unlocks only after admin package grant", async ({ browser }) => {
    const student = await browser.newContext();
    const admin = await browser.newContext();

    try {
      const selected = await selectPackageWithPaidMaterial(student.request, cfg);
      const studentPage = await student.newPage();

      await studentPage.goto(joinURL(cfg.webBaseURL, `/packages/${selected.coursePackage.id}`), { waitUntil: "networkidle" });
      await expect(studentPage.getByText(selected.coursePackage.title).first()).toBeVisible();

      await studentPage.goto(joinURL(cfg.webBaseURL, `/materials/${selected.paidMaterial.id}`), { waitUntil: "networkidle" });
      await expect(studentPage.getByText(selected.paidMaterial.title).first()).toBeVisible();

      await loginStudent(studentPage, cfg.studentEmail, cfg.studentCode);
      const studentUser = await apiGET<User>(student.request, cfg.apiBaseURL, "/auth/me");
      expect(studentUser.email).toBe(cfg.studentEmail);

      const denied = await student.request.get(joinURL(cfg.apiBaseURL, `/materials/${selected.paidMaterial.id}/download`));
      expect(denied.status(), "Use a fresh student account; paid download must be denied before entitlement.").toBe(403);
      await expectJSONMessage(denied, /entitlement_required|forbidden/i);

      const adminPage = await admin.newPage();
      await loginAdmin(adminPage, cfg.adminBaseURL, cfg.adminEmail, cfg.adminCode);
      await adminPage.goto(joinURL(cfg.adminBaseURL, "/access-grants"), { waitUntil: "networkidle" });
      await expect(adminPage.getByRole("heading", { name: "权益授权" })).toBeVisible();

      const grantResponse = await admin.request.post(joinURL(cfg.apiBaseURL, "/admin/access-grants"), {
        data: {
          packageId: selected.coursePackage.id,
          userId: studentUser.id,
        },
      });
      expect(grantResponse.status()).toBe(200);
      const grantPayload = (await grantResponse.json()) as Envelope<{ alreadyGranted: boolean }>;
      expect(grantPayload.code).toBe(0);

      const allowed = await student.request.get(joinURL(cfg.apiBaseURL, `/materials/${selected.paidMaterial.id}/download`));
      expect(allowed.status()).toBe(200);
      expect((await allowed.body()).length).toBeGreaterThan(0);

      await studentPage.goto(joinURL(cfg.webBaseURL, `/packages/${selected.coursePackage.id}`), { waitUntil: "networkidle" });
      await expect(studentPage.getByRole("heading", { name: "已解锁" })).toBeVisible();
      await expect(studentPage.getByText(selected.paidMaterial.title).first()).toBeVisible();
    } finally {
      await admin.close();
      await student.close();
    }
  });
});

async function selectPackageWithPaidMaterial(request: APIRequestContext, config: SmokeConfig): Promise<SelectedPackage> {
  if (config.packageID) {
    const detail = await packageDetail(request, config.apiBaseURL, config.packageID);
    return selectedFromDetail(detail);
  }

  const list = await apiGET<PackageListData>(request, config.apiBaseURL, "/packages");
  for (const coursePackage of list.packages) {
    const detail = await packageDetail(request, config.apiBaseURL, coursePackage.id);
    const paidMaterial = detail.materials.find(isPaidMaterial);
    if (paidMaterial) {
      return { coursePackage: detail.package, paidMaterial };
    }
  }

  throw new Error("No published package with a paid/member_only material was found.");
}

async function packageDetail(request: APIRequestContext, apiBaseURL: string, packageID: string) {
  const detail = await apiGET<PackageDetailData>(request, apiBaseURL, `/packages/${packageID}`);
  expect(JSON.stringify(detail), "Public package detail must not expose storage keys.").not.toContain("storageKey");
  return detail;
}

function selectedFromDetail(detail: PackageDetailData): SelectedPackage {
  const paidMaterial = detail.materials.find(isPaidMaterial);
  if (!paidMaterial) {
    throw new Error(`Package ${detail.package.id} does not include a paid/member_only material.`);
  }
  return { coursePackage: detail.package, paidMaterial };
}

function isPaidMaterial(material: Material) {
  return material.status === "published" && (material.accessLevel === "paid" || material.accessLevel === "member_only");
}

async function loginStudent(page: Page, email: string, code: string) {
  await page.goto(joinURL(cfg.webBaseURL, "/login"), { waitUntil: "networkidle" });
  await page.locator('input[type="email"]').first().fill(email);
  await page.getByRole("button", { name: "发送验证码" }).click();
  const codeInput = page.getByPlaceholder("123456").first();
  if (code) {
    await codeInput.fill(code);
  } else {
    await expect(codeInput, "No E2E_STUDENT_CODE was provided and the API did not return a development code.").not.toHaveValue("");
  }
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText(email)).toBeVisible();
}

async function loginAdmin(page: Page, adminBaseURL: string, email: string, code: string) {
  await page.goto(joinURL(adminBaseURL, "/login"), { waitUntil: "networkidle" });
  await page.getByPlaceholder("admin@example.com").fill(email);
  await page.getByRole("button", { name: "发送验证码" }).click();
  const codeInput = page.getByPlaceholder("123456").first();
  if (code) {
    await codeInput.fill(code);
  } else {
    await expect(codeInput, "No E2E_ADMIN_CODE was provided and the API did not return a development code.").not.toHaveValue("");
  }
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText(email)).toBeVisible();
}

async function apiGET<T>(request: APIRequestContext, apiBaseURL: string, path: string): Promise<T> {
  const response = await request.get(joinURL(apiBaseURL, path));
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<T>;
  expect(payload.code).toBe(0);
  return payload.data;
}

async function expectJSONMessage(response: { json: () => Promise<unknown> }, pattern: RegExp) {
  const payload = (await response.json().catch(() => ({}))) as { message?: string };
  expect(payload.message ?? "").toMatch(pattern);
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_DELIVERY_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    adminBaseURL: cleanBaseURL(process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:5173"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    studentEmail: process.env.E2E_STUDENT_EMAIL ?? "smoke-browser@stu.henu.edu.cn",
    studentCode: process.env.E2E_STUDENT_CODE ?? "",
    adminEmail: process.env.E2E_ADMIN_EMAIL ?? "admin@example.com",
    adminCode: process.env.E2E_ADMIN_CODE ?? "",
    packageID: process.env.E2E_PACKAGE_ID ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

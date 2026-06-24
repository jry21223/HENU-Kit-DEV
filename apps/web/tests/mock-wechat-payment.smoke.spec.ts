import { createHmac } from "node:crypto";
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

type Order = {
  id: string;
  outTradeNo: string;
  status: string;
  amountTotal: number;
  currency: string;
};

type OrderCreateResult = {
  order?: Order;
  alreadyOwned: boolean;
  alreadyPending: boolean;
  entitlementGranted: boolean;
};

type NativePayment = {
  orderId: string;
  codeUrl: string;
  status: string;
  amountTotal: number;
  mock: boolean;
};

type OrderStatus = {
  orderId: string;
  status: string;
  entitlementGranted: boolean;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  apiBaseURL: string;
  studentEmail: string;
  studentCode: string;
  packageID: string;
  mockSecret: string;
};

type SelectedPackage = {
  coursePackage: CoursePackage;
  paidMaterial: Material;
};

const cfg = readConfig();

test.describe("mock WeChat payment browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_MOCK_PAYMENT_SMOKE=1 plus E2E_* URLs/account and E2E_MOCK_PAYMENT_SECRET to run.");

  test("Web QR flow unlocks paid material only after signed backend mock notify", async ({ browser }) => {
    expect(cfg.mockSecret, "Set E2E_MOCK_PAYMENT_SECRET to the same fake secret as API WECHAT_PAY_API_V3_KEY.").not.toBe("");

    const context = await browser.newContext();
    try {
      const selected = await selectPackageWithPaidMaterial(context.request, cfg);
      const page = await context.newPage();

      await loginStudent(page, cfg.studentEmail, cfg.studentCode);

      const denied = await context.request.get(joinURL(cfg.apiBaseURL, `/materials/${selected.paidMaterial.id}/download`));
      expect(denied.status(), "Use a fresh student account; paid download must be denied before payment entitlement.").toBe(403);
      await expectJSONMessage(denied, /entitlement_required|forbidden/i);

      await page.goto(joinURL(cfg.webBaseURL, `/packages/${selected.coursePackage.id}`), { waitUntil: "networkidle" });
      await expect(page.getByText(selected.coursePackage.title).first()).toBeVisible();
      await expect(page.getByTestId("package-payment-panel")).toBeVisible();

      const [orderResponse, nativeResponse] = await Promise.all([
        page.waitForResponse((response) => response.url().includes("/orders") && response.request().method() === "POST"),
        page.waitForResponse((response) => response.url().includes("/payments/wechat/native") && response.request().method() === "POST"),
        page.getByTestId("package-create-order").click(),
      ]);
      expect(orderResponse.status()).toBe(200);
      expect(nativeResponse.status()).toBe(200);

      const orderPayload = (await orderResponse.json()) as Envelope<OrderCreateResult>;
      expect(orderPayload.code).toBe(0);
      expect(orderPayload.data.alreadyOwned, "Use a fresh student account; already-owned packages cannot prove payment unlock.").toBeFalsy();
      const order = orderPayload.data.order;
      expect(order?.id).toBeTruthy();
      expect(order?.outTradeNo).toBeTruthy();
      expect(order?.amountTotal).toBeGreaterThan(0);

      const nativePayload = (await nativeResponse.json()) as Envelope<NativePayment>;
      expect(nativePayload.code).toBe(0);
      expect(nativePayload.data.orderId).toBe(order?.id);
      expect(nativePayload.data.status).toBe("paying");
      expect(nativePayload.data.mock, "This smoke must run only against mock WeChat mode.").toBe(true);
      expect(nativePayload.data.codeUrl).toContain("weixin://wxpay/mock/");
      await expect(page.getByTestId("package-wechat-qr")).toBeVisible();
      await expect(page.getByTestId("package-native-status")).toContainText("paying");

      const body = JSON.stringify({
        outTradeNo: order?.outTradeNo,
        transactionId: `E2E_MOCK_${Date.now()}`,
        tradeState: "SUCCESS",
        amountTotal: order?.amountTotal,
      });
      const notifyResponse = await context.request.post(joinURL(cfg.apiBaseURL, "/payments/wechat/notify"), {
        data: body,
        headers: {
          "Content-Type": "application/json",
          "X-WeChat-Mock-Signature": mockNotifySignature(body, cfg.mockSecret),
        },
      });
      expect(notifyResponse.status()).toBe(200);
      const notifyPayload = (await notifyResponse.json()) as { code: string; message: string };
      expect(notifyPayload.code).toBe("SUCCESS");

      const status = await apiGET<OrderStatus>(context.request, cfg.apiBaseURL, `/orders/${order?.id}/status`);
      expect(status.status).toBe("paid");
      expect(status.entitlementGranted).toBe(true);

      const [statusResponse] = await Promise.all([
        page.waitForResponse((response) => response.url().includes(`/orders/${order?.id}/status`) && response.request().method() === "GET"),
        page.getByTestId("package-refresh-order").click(),
      ]);
      expect(statusResponse.status()).toBe(200);
      await expect(page.getByTestId("package-unlocked-panel")).toBeVisible();
      await expect(page.getByText(selected.paidMaterial.title).first()).toBeVisible();

      const allowed = await context.request.get(joinURL(cfg.apiBaseURL, `/materials/${selected.paidMaterial.id}/download`));
      expect(allowed.status()).toBe(200);
      expect((await allowed.body()).length).toBeGreaterThan(0);
    } finally {
      await context.close();
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

function mockNotifySignature(body: string, secret: string) {
  return createHmac("sha256", secret).update(body).digest("hex");
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_MOCK_PAYMENT_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    studentEmail: process.env.E2E_STUDENT_EMAIL ?? "smoke-mock-pay@stu.henu.edu.cn",
    studentCode: process.env.E2E_STUDENT_CODE ?? "",
    packageID: process.env.E2E_PACKAGE_ID ?? "",
    mockSecret: process.env.E2E_MOCK_PAYMENT_SECRET ?? process.env.SMOKE_MOCK_WECHAT_SECRET ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}

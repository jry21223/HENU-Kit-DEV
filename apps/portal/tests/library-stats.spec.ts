import { expect, test } from "@playwright/test";

const MATERIALS = [
  {
    id: "calculus-note",
    type: "note",
    subject: "高等数学A（二）",
    title: "极限复习笔记",
    author: "学生编辑部",
    intro: "公开复习资料",
    toc: [],
    pages: [],
    price: 0,
    previewPages: 0,
    rating: 4.8,
    downloads: 1200,
    favs: 12,
  },
  {
    id: "network-exam",
    type: "exam",
    subject: "计算机网络",
    title: "2025 年期末试卷",
    author: "学生编辑部",
    intro: "公开往年真题",
    toc: [],
    pages: [],
    price: 0,
    previewPages: 0,
    rating: 4.6,
    downloads: 34,
    favs: 3,
  },
];

function counter(page: import("@playwright/test").Page, label: string) {
  return page
    .getByText(label, { exact: true })
    .locator("..")
    .locator('span[aria-hidden="true"]');
}

function counterGroup(page: import("@playwright/test").Page, label: string) {
  return page.getByText(label, { exact: true }).locator("..");
}

test("library statistics stay unknown until the complete catalog loads", async ({
  page,
}) => {
  let releaseResponse!: () => void;
  const responseGate = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });

  await page.route("**/api/v1/library/materials", async (route) => {
    await responseGate;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: MATERIALS,
        request_id: "req_library_stats",
      }),
    });
  });

  await page.goto("/library", { waitUntil: "domcontentloaded" });

  await expect(counter(page, "收录资料")).toHaveText("—");
  await expect(counter(page, "累计下载")).toHaveText("—");
  await expect(counterGroup(page, "收录资料")).toHaveAttribute("aria-busy", "true");

  releaseResponse();

  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /2025 年期末试卷/ })).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("1,234");

  await page.getByPlaceholder("搜索：真题 / 高数 / 实验报告").fill("计算机网络");
  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toHaveCount(0);
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("1,234");
});

test("library statistics recover together after a failed catalog request", async ({
  page,
}) => {
  let shouldFail = true;
  await page.route("**/api/v1/library/materials", async (route) => {
    if (shouldFail) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: { code: "library_unavailable", message: "资料库暂时不可用" },
          request_id: "req_library_error",
        }),
      });
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: MATERIALS.slice(0, 1),
        request_id: "req_library_retry",
      }),
    });
  });

  await page.goto("/library", { waitUntil: "domcontentloaded" });

  await expect(
    page.getByRole("alert").filter({ hasText: "ERROR / 数据源不可用" })
  ).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("—");
  await expect(counter(page, "累计下载")).toHaveText("—");
  await expect(counterGroup(page, "收录资料")).toHaveAttribute("aria-busy", "false");
  await expect(counterGroup(page, "收录资料").getByText("暂不可用")).toBeAttached();

  shouldFail = false;
  await page.getByRole("button", { name: "重试" }).click();

  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("1");
  await expect(counter(page, "累计下载")).toHaveText("1,200");
});

test("a failed refresh does not present a warmed catalog as current statistics", async ({
  page,
}) => {
  let shouldFail = false;
  await page.route("**/api/v1/library/materials", async (route) => {
    if (shouldFail) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: { code: "library_unavailable", message: "资料库暂时不可用" },
          request_id: "req_library_refresh_error",
        }),
      });
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: MATERIALS,
        request_id: "req_library_warm_cache",
      }),
    });
  });

  await page.goto("/library", { waitUntil: "domcontentloaded" });
  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("1,234");

  await page.getByRole("link", { name: /极限复习笔记/ }).click();
  await expect(page.getByRole("heading", { name: "极限复习笔记" })).toBeVisible();
  shouldFail = true;
  await page.goBack({ waitUntil: "domcontentloaded" });

  await expect(
    page.getByRole("alert").filter({ hasText: "ERROR / 数据源不可用" })
  ).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("—");
  await expect(counter(page, "累计下载")).toHaveText("—");
});

test("an empty catalog reports confirmed zero statistics", async ({ page }) => {
  await page.route("**/api/v1/library/materials", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: [],
        request_id: "req_library_empty",
      }),
    });
  });

  await page.goto("/library", { waitUntil: "domcontentloaded" });

  await expect(page.getByText("无匹配资料 / EMPTY", { exact: true })).toBeVisible();
  await expect(counter(page, "收录资料")).toHaveText("0");
  await expect(counter(page, "累计下载")).toHaveText("0");
});

test("reduced motion reaches the same final library statistics", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route("**/api/v1/library/materials", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: MATERIALS,
        request_id: "req_library_reduced_motion",
      }),
    });
  });

  await page.goto("/library", { waitUntil: "domcontentloaded" });

  await expect(counter(page, "收录资料")).toHaveText("2");
  await expect(counter(page, "累计下载")).toHaveText("1,234");
});

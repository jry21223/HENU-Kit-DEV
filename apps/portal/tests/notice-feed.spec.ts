import { expect, test } from "@playwright/test";

const publishedNotice = {
  id: "22222222-2222-4222-8222-222222222222",
  source: {
    id: "11111111-1111-4111-8111-111111111111",
    code: "registrar",
    name: "教务处",
  },
  version: 1,
  title: "暑期安排",
  body: "暑期服务时间以本公告为准。",
  source_url: "https://example.edu/notices/summer",
  content_hash: "0".repeat(64),
  state: "distributed",
  revision: 1,
  created_at: "2026-08-07T00:00:00Z",
  distribution_count: 1,
  distribution_status: "delivered",
};

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} renders and expands a contract-valid Notice-owner item`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/notices", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          request_id: "req_notice_success",
          data: { items: [publishedNotice], generated_at: "2026-08-07T00:01:00Z" },
        }),
      });
    });

    await page.goto("/notice", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: "通知公告" })).toBeVisible();
    await page.getByRole("button", { name: /暑期安排/ }).click();
    await expect(page.getByText("暑期服务时间以本公告为准。")).toBeVisible();
    await expect(page.getByText(/来源 \/ 教务处/)).toBeVisible();
    expect(await page.locator("html").evaluate(
      (element) => element.scrollWidth <= element.clientWidth
    )).toBeTruthy();
  });
}

test("empty, dependency failure, and signed-out Notice states stay explicit", async ({ page }) => {
  let state: "empty" | "failure" | "signed-out" = "empty";
  await page.route("**/api/v1/notices", async (route) => {
    if (state === "failure") {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "notice_unavailable", request_id: "req_notice_failure" }),
      });
      return;
    }
    if (state === "signed-out") {
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ error: "not authenticated", request_id: "req_notice_signed_out" }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        request_id: "req_notice_empty",
        data: { items: [], generated_at: "2026-08-07T00:01:00Z" },
      }),
    });
  });

  await page.goto("/notice", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("暂无已发布公告")).toBeVisible();

  state = "failure";
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByText("服务暂时不可用，请稍后再试。")).toBeVisible();
  await expect(page.getByText("暑期安排")).toHaveCount(0);

  state = "signed-out";
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByText("需要登录后才能查看通知", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "登录 / 注册" }))
    .toHaveAttribute("href", "/account/login?next=/notice");
});

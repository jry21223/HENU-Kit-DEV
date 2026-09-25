import { expect, test, type Page } from "@playwright/test";

/**
 * 资料库接口返回 HTML 错误页（反向代理 404、WAF 挑战页）时，首页 01 与 /library
 * 都只给中文、可恢复的提示（#533）：一条主信息加「重试」，有请求编号时显示为错误编号；
 * 不出现 Not Found、接口路径、HTTP 状态文本或内部组件名。
 */

const HTML_404 =
  "<!DOCTYPE html><html><head><title>404 Not Found</title></head><body><h1>Not Found</h1><hr><center>nginx</center></body></html>";
const LEAKS = /Not Found|nginx|\/api\/|Gateway|HTTP|Invalid JSON|DATA SOURCE|OFFLINE|数据源不可用/;

const MATERIAL = {
  id: "11111111-1111-4111-8111-111111111111", type: "note", subject: "高等数学",
  title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
  price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
};

const CATALOG = {
  materials: [MATERIAL],
  statistics: {
    releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
    materialCount: 1,
    downloadStarts: 12,
    countingSince: "2026-08-11T00:00:00Z",
    asOf: "2026-08-11T01:00:00Z",
  },
  request_id: "req_library_ok",
};

/** Fails the catalog read with an HTML page until recover() is called. */
async function breakLibraryCatalog(page: Page, status: number) {
  let broken = true;
  await page.route("**/api/v1/library/materials", (route) =>
    broken
      ? route.fulfill({
          status,
          headers: { "Content-Type": "text/html", "X-Request-Id": "req_edge404" },
          body: HTML_404,
        })
      : route.fulfill({ json: CATALOG })
  );
  return () => {
    broken = false;
  };
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

for (const [name, status] of [
  ["an HTML 404", 404],
  ["an HTML page served as 200", 200],
] as const) {
  test(`home library block turns ${name} into a Chinese message it can retry`, async ({ page }) => {
    const recover = await breakLibraryCatalog(page, status);
    await page.goto("/");
    await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");

    const section = page.locator("section").filter({ has: page.getByRole("heading", { name: "资料库", level: 2 }) });
    const alert = section.getByRole("alert");
    await expect(alert).toContainText("服务暂时不可用，请稍后再试。");
    await expect(alert).toContainText("错误编号：req_edge404");
    await expect(section).not.toContainText(LEAKS);
    await expect(page.locator("body")).not.toContainText("Not Found");

    recover();
    await alert.getByRole("button", { name: "重试" }).click();
    await expect(section.getByRole("alert")).toHaveCount(0);
    await expect(section.getByRole("heading", { name: "笔记总结" })).toBeVisible();
  });
}

test("/library turns an HTML 404 into one Chinese message with retry", async ({ page }) => {
  const recover = await breakLibraryCatalog(page, 404);
  await page.goto("/library");
  await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");

  const alert = page.locator("main").getByRole("alert");
  await expect(alert).toContainText("资料库暂时无法加载，请稍后重试。");
  await expect(alert).toContainText("错误编号：req_edge404");
  await expect(alert).not.toContainText("服务暂时不可用，请稍后再来");
  await expect(page.locator("main")).not.toContainText(LEAKS);
  // 失败只说一次：书架区不再叠一句空状态。
  await expect(page.locator("main")).not.toContainText("加载不出来");
  await expect(page.getByText(/资料库当前暂无公开资料|无匹配资料/)).toHaveCount(0);

  recover();
  await alert.getByRole("button", { name: "重试" }).click();
  await expect(page.locator("main").getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("link", { name: /极限复习笔记/ })).toBeVisible();
});

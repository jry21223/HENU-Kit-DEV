import { expect, test, type Page, type Route } from "@playwright/test";

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

test("/food says a failed ranking load once, in the error banner only (#549)", async ({ page }) => {
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_food_down" },
    })
  );
  await page.goto("/food");
  await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");

  const alert = page.locator("main").getByRole("alert");
  await expect(alert).toHaveCount(1);
  await expect(alert.getByRole("button", { name: "重试" })).toBeVisible();
  // 与 /library、/campus 一致：失败只由提示条说明，列表区不再叠一句空状态。
  await expect(page.getByText(/榜单暂时加载不出来/)).toHaveCount(0);
  await expect(page.locator("main")).not.toContainText(LEAKS);
  // 筛选行右侧的英文状态也不再说还在同步：请求已经失败了。
  await expect(page.locator("main")).not.toContainText("SYNCING");
});

/**
 * 互助单详情分清两种失败（与资料详情、美食详情一致）：单子不存在时只显示 404 页；
 * 服务不可用、返回 HTML 或断网时说明暂时读不到，并给「重试」。
 */
const CAMPUS_ITEM = {
  id: "campus-bookcase", type: "sell", category: "flea", title: "九成新书架", desc: "宿舍搬家出",
  price: 20, seller: "同学乙", credit: 0, dealsDone: 0, wants: 0, place: "金明校区", status: "open", time: "2026-09-21",
};

test("a campus item that does not exist says so once, without an error line or retry", async ({ page }) => {
  await page.route(`**/api/v1/campus/items/${CAMPUS_ITEM.id}`, (route) =>
    route.fulfill({ status: 404, json: { error: "not_found", request_id: "req_campus_gone" } })
  );
  await page.goto(`/campus/item/${CAMPUS_ITEM.id}`);
  await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");

  const main = page.locator("main");
  await expect(main.getByText("单子不存在或已下架", { exact: true })).toBeVisible();
  await expect(main).toContainText("404 / NOT FOUND");
  // 不存在就只说不存在：不叠一句“稍后再试”，也不给帮不上忙的重试。
  await expect(main.getByRole("alert")).toHaveCount(0);
  await expect(main).not.toContainText(/暂时不可用|稍后|重试/);
  await expect(main.locator("[data-back-link]")).toHaveAttribute("href", "/campus");
});

const CAMPUS_FAILURES = [
  {
    name: "is unavailable",
    message: "服务暂时不可用，请稍后再试。",
    fail: (route: Route) =>
      route.fulfill({ status: 503, json: { error: "upstream_unavailable", request_id: "req_campus_down" } }),
  },
  {
    name: "comes back as an HTML page",
    message: "服务暂时不可用，请稍后再试。",
    fail: (route: Route) => route.fulfill({ status: 200, contentType: "text/html", body: HTML_404 }),
  },
  {
    name: "cannot be reached",
    message: "网络连接失败，请检查网络后重试。",
    fail: (route: Route) => route.abort("internetdisconnected"),
  },
];

for (const failure of CAMPUS_FAILURES) {
  test(`a campus item that ${failure.name} is not reported as missing and can be retried`, async ({ page }) => {
    let broken = true;
    await page.route(`**/api/v1/campus/items/${CAMPUS_ITEM.id}`, (route) =>
      broken
        ? failure.fail(route)
        : route.fulfill({ json: { item: CAMPUS_ITEM, messages: [], request_id: "req_campus_item" } })
    );
    await page.goto(`/campus/item/${CAMPUS_ITEM.id}`);
    await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");

    const main = page.locator("main");
    const alert = main.getByRole("alert");
    await expect(alert).toContainText(failure.message);
    // 暂时读不到不等于单子不存在。
    await expect(main).not.toContainText(/404|NOT FOUND|不存在或已下架/);
    await expect(main).not.toContainText(LEAKS);
    await expect(main.locator("[data-back-link]")).toHaveAttribute("href", "/campus");

    broken = false;
    await alert.getByRole("button", { name: "重试" }).click();
    await expect(main.getByRole("heading", { level: 1, name: CAMPUS_ITEM.title })).toBeVisible();
    await expect(main.getByRole("alert")).toHaveCount(0);
  });
}

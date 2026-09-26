import { expect, test, type Page } from "@playwright/test";

/**
 * 空状态给出可执行的下一步（#545）：没有内容时说明原因，并提供一个动作——
 * 去别处（去题库、去刷题）或就地改条件（清除筛选）。清除筛选要让列表回来，
 * 筛选控件也回到「全部」。
 */

async function waitForHydration(page: Page) {
  await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");
}

test("an empty favorites overview links to the bank catalog", async ({ page }) => {
  await page.route("**/api/v1/practice/favorites", (route) =>
    route.fulfill({ json: { request_id: "req_favorites_empty", data: [] } })
  );
  await page.goto("/practice/favorites");
  await waitForHydration(page);

  const overview = page.getByTestId("practice-favorites-overview");
  await expect(overview).toContainText("还没有收藏任何题目，刷题时点击「收藏」即可加入");
  await expect(overview).not.toContainText("EMPTY");
  const toCatalog = overview.getByRole("link", { name: "去题库", exact: true });
  await expect(toCatalog).toHaveAttribute("href", "/practice");

  await toCatalog.click();
  await expect(page).toHaveURL(/\/practice$/);
});

test("practice data that has not launched yet points to practice", async ({ page }) => {
  await page.goto("/practice/stats");
  await waitForHydration(page);

  const disabled = page.getByTestId("practice-stats-disabled");
  await expect(disabled).toContainText("学习数据即将上线，敬请期待");
  await expect(disabled).not.toContainText("EMPTY");
  await expect(disabled.getByRole("link", { name: "去刷题", exact: true })).toHaveAttribute("href", "/practice");
});

test("a leaderboard that has not opened yet points to practice", async ({ page }) => {
  await page.goto("/practice/leaderboard");
  await waitForHydration(page);

  const board = page.getByTestId("practice-leaderboard");
  await expect(board).toContainText("排行榜数据暂未开放");
  await expect(board).not.toContainText("EMPTY");
  await expect(board.getByRole("link", { name: "去刷题", exact: true })).toHaveAttribute("href", "/practice");
});

const CAMPUS_ITEMS = [
  {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  },
  {
    id: "campus-bookcase", type: "sell", category: "flea", title: "九成新书架", desc: "宿舍搬家出",
    price: 20, seller: "同学乙", credit: 0, dealsDone: 0, wants: 0, place: "金明校区", status: "open", time: "2026-09-21",
  },
];

test("campus filters with no match offer to clear them and bring the list back", async ({ page }) => {
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_campus_items" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_campus_categories" } })
  );
  await page.goto("/campus");
  await waitForHydration(page);

  const express = page.getByRole("heading", { name: "代取快递到南门", exact: true });
  const bookcase = page.getByRole("heading", { name: "九成新书架", exact: true });
  await expect(express).toBeVisible();
  await expect(bookcase).toBeVisible();
  // 真实有内容时不出现清除筛选。
  await expect(page.getByRole("button", { name: "清除筛选" })).toHaveCount(0);

  const search = page.getByPlaceholder("搜索：快递 / 键盘 / 占座");
  await search.fill("快递");
  await page.getByRole("button", { name: "闲置单", exact: true }).click();
  await page.getByRole("button", { name: "代取快递", exact: true }).click();
  await expect(page.getByText("无匹配单子", { exact: true })).toBeVisible();
  await expect(express).toHaveCount(0);
  await expect(bookcase).toHaveCount(0);

  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  await expect(express).toBeVisible();
  await expect(bookcase).toBeVisible();
  // 按钮随空状态一起消失，焦点交给刚复位的搜索与筛选区，而不是落回页面开头。
  await expect(page.getByRole("search", { name: "互助搜索与筛选" })).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(page.getByRole("button", { name: "闲置单", exact: true })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByRole("button", { name: "代取快递", exact: true })).toHaveAttribute("aria-pressed", "false");
  // 类型和分类各有一个「全部」，两个都要回到选中。
  const allFilters = page.getByRole("button", { name: "全部", exact: true });
  await expect(allFilters).toHaveCount(2);
  for (const all of await allFilters.all()) {
    await expect(all).toHaveAttribute("aria-pressed", "true");
  }
  await expect(page.getByRole("button", { name: "清除筛选" })).toHaveCount(0);
});

const LIBRARY_MATERIALS = [
  {
    id: "library-limits", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
  {
    id: "library-network", type: "exam", subject: "计算机网络",
    title: "期末试卷", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 34, downloadAvailable: true, fileSize: 8192,
  },
];

test("library filters with no match offer to clear them and bring the shelf back", async ({ page }) => {
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      json: {
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 46,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_library_empty_actions",
      },
    })
  );
  await page.goto("/library");
  await waitForHydration(page);

  const limits = page.getByRole("link", { name: /极限复习笔记/ });
  const exam = page.getByRole("link", { name: /期末试卷/ });
  await expect(limits).toBeVisible();
  await expect(exam).toBeVisible();
  await expect(page.getByRole("button", { name: "清除筛选" })).toHaveCount(0);

  const search = page.getByPlaceholder("搜索：真题 / 高数 / 课件");
  const subject = page.getByRole("combobox", { name: "按科目筛选" });
  const types = page.getByRole("group", { name: "资料类型" });
  await subject.selectOption("高等数学");
  await types.getByRole("button", { name: "往年真题", exact: true }).click();
  await search.fill("极限");
  await expect(page.getByText("无匹配资料", { exact: true })).toBeVisible();
  await expect(limits).toHaveCount(0);
  await expect(exam).toHaveCount(0);

  // 用键盘清除：焦点交给刚复位的搜索与筛选区，再按 Tab 就回到搜索框。
  await page.getByRole("button", { name: "清除筛选", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(limits).toBeVisible();
  await expect(exam).toBeVisible();
  await expect(page.getByRole("search", { name: "资料搜索与筛选" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(subject).toHaveValue("all");
  await expect(types.getByRole("button", { name: "全部", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "清除筛选" })).toHaveCount(0);
});

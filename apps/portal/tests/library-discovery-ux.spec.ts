import { expect, test, type Page } from "@playwright/test";

const MATERIALS = [
  {
    id: "library-cpp", type: "textbook", subject: "C++",
    title: "C++_教材_C++PrimerPlus第6版", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
  {
    id: "library-python", type: "exercise", subject: "Python程序设计",
    title: "Python程序设计_题库练习_期末复习题_文字版", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 8, downloadAvailable: true, fileSize: 8192,
  },
  {
    id: "library-spring", type: "note", subject: "Spring与后端",
    title: "Spring与后端_笔记_AOP面向切面编程", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 5, downloadAvailable: true, fileSize: 2048,
  },
  {
    id: "library-plain", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 1, downloadAvailable: true, fileSize: 1024,
  },
  {
    id: "library-identifier", type: "note", subject: "数据库",
    title: "数据库_笔记_primary_key 与 user_id", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 3, downloadAvailable: true, fileSize: 1024,
  },
  {
    id: "library-unknown", type: "note", subject: "数据库",
    title: "数据库_实验_primary_key_v2", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 2, downloadAvailable: true, fileSize: 1024,
  },
];

async function mockCatalog(page: Page) {
  await page.route("**/api/v1/library/materials", (route) => route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({
      materials: MATERIALS,
      statistics: {
        releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
        materialCount: MATERIALS.length,
        downloadStarts: 99,
        countingSince: "2026-08-11T00:00:00Z",
        asOf: "2026-08-11T01:00:00Z",
      },
      request_id: "req_library_discovery",
    }),
  }));
}

test("readable cards preserve subject, type, source-title search and the owner download", async ({ page }) => {
  await mockCatalog(page);
  const source = MATERIALS[0];
  await page.route(`**/api/v1/library/materials/${source.id}`, (route) => route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ material: source, request_id: "req_library_detail" }),
  }));
  await page.goto("/library");

  const cppCard = page.getByRole("link", { name: /C\+\+PrimerPlus第6版/ });
  await expect(cppCard.getByRole("heading", { name: "C++PrimerPlus第6版", exact: true })).toBeVisible();
  await expect(cppCard.getByText("C++", { exact: true })).toBeVisible();
  await expect(cppCard.getByText("电子版教材", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "期末复习题_文字版", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "AOP面向切面编程", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "极限复习笔记", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "primary_key 与 user_id", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "数据库_实验_primary_key_v2", exact: true })).toBeVisible();

  const search = page.getByPlaceholder("搜索：真题 / 高数 / 课件");
  await search.fill(source.title);
  await expect(cppCard).toBeVisible();
  await expect(page.getByRole("link", { name: /AOP面向切面编程/ })).toHaveCount(0);
  await search.fill("Spring与后端");
  await expect(page.getByRole("link", { name: /AOP面向切面编程/ })).toBeVisible();
  await search.fill("C++PrimerPlus第6版");
  await cppCard.click();

  await expect(page.getByRole("heading", { name: "C++PrimerPlus第6版", exact: true })).toBeVisible();
  await expect(page.getByText("原始标题", { exact: true }).locator("..")).toContainText(source.title);
  await expect(page.getByRole("link", { name: /下载资料/ })).toHaveAttribute("href", `/api/v1/library/materials/${source.id}/download`);
});

test("390px Library search and filters are reachable on arrival and preserve owner totals", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockCatalog(page);
  await page.goto("/library");

  const search = page.getByPlaceholder("搜索：真题 / 高数 / 课件");
  await expect(search).toBeInViewport({ ratio: 1 });
  const subject = page.getByRole("combobox", { name: "按科目筛选" });
  await expect(subject).toBeInViewport({ ratio: 1 });
  const textbooks = page.getByRole("button", { name: "电子版教材", exact: true });
  await expect(textbooks).toBeInViewport({ ratio: 1 });
  await expect(page.getByText("搜索资料", { exact: true })).toBeVisible();
  const controls = [search, subject, ...await page.getByRole("search", { name: "资料搜索与筛选" }).getByRole("button").all()];
  for (const control of controls) {
    const bounds = await control.boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    expect(bounds?.width).toBeGreaterThanOrEqual(44);
  }

  await search.focus();
  await page.keyboard.press("Tab");
  await expect(subject).toBeFocused();
  await subject.selectOption("C++");
  await textbooks.click();
  await expect(textbooks).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("heading", { name: "C++PrimerPlus第6版", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "极限复习笔记", exact: true })).toHaveCount(0);
  await expect(page.getByText("收录资料", { exact: true }).locator("..").locator('span[aria-hidden="true"]')).toHaveText("6");
  await expect(page.getByText("累计下载", { exact: true }).locator("..").locator('span[aria-hidden="true"]')).toHaveText("99");
  const sizes = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
  expect(sizes.scroll).toBeLessThanOrEqual(sizes.client);
});

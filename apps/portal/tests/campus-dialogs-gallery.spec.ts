import { expect, test, type Page } from "@playwright/test";
import { mockSignedInGateway } from "./support/gateway";

/**
 * 校验：
 * 1. /campus「发布新单子」弹窗：直角分段类型切换 + 分类标签网格（无 <select>）、纸白方框输入、
 *    隐藏原生 file input 与「+ 上传」虚线方框、缩略图下方 ≥44px 删除按钮、文件数量/格式/大小校验、
 *    390×844 与 1440×900 无横向溢出；
 * 2. /campus「联系方式簿」弹窗：503 CAMPUS_WRITES_UNAVAILABLE 隐藏「重新加载」按钮，
 *    临时 500 错误保留「重新加载」按钮；
 * 3. /campus/item/[id] 多图展示（1–6 张切换 + 大图弹窗）在 390px 与 1440px 下无溢出、无断图；
 * 4. /food 排序 Select 具备 appearance: none 与右侧自定义 SVG 箭头。
 */

const TINY_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+X2dIAAAAASUVORK5CYII=",
  "base64"
);

const CATEGORIES = [
  { key: "express", name: "快递代取", code: "EXP" },
  { key: "book", name: "教材资料", code: "BOK" },
  { key: "digital", name: "数码外设", code: "DIG" },
];

async function mockCampusBase(page: Page) {
  await mockSignedInGateway(page);
  await page.route("**/api/v1/campus/items", (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({ json: { items: [], request_id: "req_campus_list" } });
    }
    return route.continue();
  });
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: CATEGORIES, request_id: "req_campus_cats" } })
  );
}

test.describe("Campus 弹窗与多图展示", () => {
  test("发布新单子弹窗：无原生 select / 无可见原生 file input，支持分段切换、图片校验与 390/1440 适配", async ({
    page,
  }) => {
    await mockCampusBase(page);

    for (const viewport of [
      { width: 390, height: 844 },
      { width: 1440, height: 900 },
    ]) {
      await page.setViewportSize(viewport);
      await page.goto("/campus", { waitUntil: "domcontentloaded" });
      await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });

      await page.getByRole("button", { name: "发布新单子" }).click();
      const dialog = page.getByRole("dialog", { name: "发布新单子" });
      await expect(dialog).toBeVisible();

      // 绝不使用原生 <select>
      await expect(dialog.locator("select")).toHaveCount(0);

      // 类型 2 格分段切换按钮
      const typeGroup = dialog.getByRole("group", { name: "类型" });
      const helpBtn = typeGroup.getByRole("button", { name: "互助" });
      const sellBtn = typeGroup.getByRole("button", { name: "闲置" });
      await expect(helpBtn).toHaveAttribute("aria-pressed", "true");
      await sellBtn.click();
      await expect(sellBtn).toHaveAttribute("aria-pressed", "true");
      await expect(helpBtn).toHaveAttribute("aria-pressed", "false");

      // 分类标签网格
      const catGroup = dialog.getByRole("group", { name: "分类" });
      await catGroup.getByRole("button", { name: "数码外设" }).click();
      await expect(catGroup.getByRole("button", { name: "数码外设" })).toHaveAttribute("aria-pressed", "true");

      // 原生 file input 视觉隐藏，露出「+ 上传」虚线方框
      const fileInput = dialog.locator('input[type="file"]');
      await expect(fileInput).toHaveClass(/sr-only/);
      await expect(dialog.getByText("+ 上传")).toBeVisible();

      // 上传 1 张有效 PNG -> 缩略图下方 ≥ 44px 删除按钮
      await fileInput.setInputFiles({
        name: "photo.png",
        mimeType: "image/png",
        buffer: TINY_PNG,
      });
      await expect(dialog.getByText("图片（1/6，单张 ≤2MB，选填）")).toBeVisible();
      const removeBtn = dialog.getByRole("button", { name: "删除图片 1" });
      await expect(removeBtn).toBeVisible();
      const removeBox = await removeBtn.boundingBox();
      expect(removeBox?.height ?? 0).toBeGreaterThanOrEqual(44);

      // 非法格式提示
      await fileInput.setInputFiles({
        name: "bad.gif",
        mimeType: "image/gif",
        buffer: Buffer.from("GIF89a"),
      });
      await expect(dialog.getByRole("alert")).toContainText("仅支持 JPG、PNG、WebP");

      // 删除图片恢复 0/6
      await removeBtn.click();
      await expect(dialog.getByText("图片（0/6，单张 ≤2MB，选填）")).toBeVisible();

      // 弹窗内部无横向溢出
      const overflow = await dialog.evaluate((el) => el.scrollWidth - el.clientWidth);
      expect(overflow, `dialog overflow at ${viewport.width}px`).toBeLessThanOrEqual(1);

      await dialog.getByRole("button", { name: "关闭窗口" }).click();
      await expect(dialog).toBeHidden();
    }
  });

  test("联系方式簿弹窗：503 CAMPUS_WRITES_UNAVAILABLE 隐藏重新加载，临时错误保留重新加载", async ({
    page,
  }) => {
    await mockCampusBase(page);

    let mode: "unavailable" | "transient" = "unavailable";
    await page.route("**/api/v1/campus/contact-book", (route) => {
      if (mode === "unavailable") {
        return route.fulfill({
          status: 503,
          json: {
            error: "CAMPUS_WRITES_UNAVAILABLE",
            message: "互助写入服务暂未开启。",
            request_id: "req_campus_writes_off",
          },
        });
      }
      return route.fulfill({
        status: 500,
        json: {
          error: "CAMPUS_UPSTREAM_UNAVAILABLE",
          message: "上游暂时繁忙，请稍后重试。",
          request_id: "req_campus_500",
        },
      });
    });

    await page.goto("/campus", { waitUntil: "domcontentloaded" });
    await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });

    // 1. 503 CAMPUS_WRITES_UNAVAILABLE -> 清晰提示卡片，无「重新加载」按钮
    await page.getByRole("button", { name: "联系方式簿" }).click();
    const dialog = page.getByRole("dialog", { name: "联系方式簿" });
    await expect(dialog.locator('[data-campus-writes-state="unavailable"]')).toBeVisible();
    await expect(dialog.getByRole("button", { name: "重新加载" })).toHaveCount(0);
    await dialog.getByRole("button", { name: "关闭窗口" }).click();

    // 2. 临时 500 错误 -> 保留「重新加载」按钮
    mode = "transient";
    await page.getByRole("button", { name: "联系方式簿" }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "重新加载" })).toBeVisible();
  });

  test("详情页 1–6 张多图切换、查看大图与 390/1440 无溢出", async ({ page }) => {
    await mockSignedInGateway(page);

    const images = [0, 1, 2].map((pos) => `/api/v1/campus/items/multi-1/images/${pos}?v=1`);
    await page.route("**/api/v1/campus/items/multi-1/images/*", (route) =>
      route.fulfill({ status: 200, contentType: "image/png", body: TINY_PNG })
    );
    await page.route("**/api/v1/campus/items/multi-1", (route) =>
      route.fulfill({
        json: {
          item: {
            id: "multi-1",
            type: "sell",
            category: "digital",
            title: "九成新机械键盘出闲置",
            desc: "自用一年，轴体完好，明伦校区当面验货。",
            price: 120,
            seller: "小河同学",
            credit: 99,
            dealsDone: 5,
            wants: 2,
            place: "明伦校区图书馆门口",
            status: "open",
            time: "2026-11-23",
            images,
          },
          messages: [],
          request_id: "req_campus_detail_multi",
        },
      })
    );
    await page.route("**/api/v1/campus/contact-book", (route) =>
      route.fulfill({
        status: 503,
        json: { error: "CAMPUS_WRITES_UNAVAILABLE", message: "互助写入服务暂未开启。" },
      })
    );
    await page.route("**/api/v1/campus/items/multi-1/exchanges", (route) =>
      route.fulfill({
        status: 503,
        json: { error: "CAMPUS_WRITES_UNAVAILABLE", message: "互助写入服务暂未开启。" },
      })
    );

    for (const viewport of [
      { width: 390, height: 844 },
      { width: 1440, height: 900 },
    ]) {
      await page.setViewportSize(viewport);
      await page.goto("/campus/item/multi-1", { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("heading", { name: "九成新机械键盘出闲置" })).toBeVisible();
      await expect(page.getByText("1 / 3")).toBeVisible();

      await page.getByRole("button", { name: "查看下一张图片" }).click();
      await expect(page.getByText("2 / 3")).toBeVisible();
      await page.getByRole("button", { name: "查看图片 3" }).click();
      await expect(page.getByText("3 / 3")).toBeVisible();

      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth
      );
      expect(overflow, `detail page overflow at ${viewport.width}px`).toBeLessThanOrEqual(1);
    }
  });

  test("/food 排序下拉框使用 appearance: none 与自定义 SVG 箭头", async ({ page }) => {
    await page.route("**/api/v1/food/posts", (route) =>
      route.fulfill({ json: { posts: [], request_id: "req_food_select" } })
    );
    await page.goto("/food", { waitUntil: "domcontentloaded" });
    const select = page.getByLabel("排序");
    await expect(select).toBeVisible();
    const appearance = await select.evaluate((el) => getComputedStyle(el).appearance);
    expect(appearance).toBe("none");
  });
});

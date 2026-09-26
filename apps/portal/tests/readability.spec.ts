import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/**
 * 可读性基线（#536）：Portal 的颜色来自 packages/design-tokens，文字与底色的对比度
 * 达到 WCAG AA。期望色值直接读 tokens.json，页面渲染出来的就应该是 token。
 */

const TOKENS = JSON.parse(
  readFileSync(path.resolve(__dirname, "../../../packages/design-tokens/tokens.json"), "utf8")
);

/** tokens.json 的 #RRGGBB 换成 getComputedStyle 返回的写法。 */
function computed(hex: string) {
  const [r, g, b] = (hex.slice(1).match(/../g) ?? []).map((pair) => Number.parseInt(pair, 16));
  return `rgb(${r}, ${g}, ${b})`;
}

async function mockGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_readability" },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGateway(page);
});

test("home marquee sets ink text on the orange band", async ({ page }) => {
  await page.goto("/");

  const item = page.getByText("往年试卷", { exact: true }).first();
  await expect(item).toBeVisible();
  const { text, band } = await item.evaluate((element) => {
    // 条带底色：从文字往上找第一个不透明的背景。
    let node: Element | null = element;
    let background = "";
    while (node && !background) {
      const colour = getComputedStyle(node).backgroundColor;
      if (colour !== "rgba(0, 0, 0, 0)" && colour !== "transparent") background = colour;
      node = node.parentElement;
    }
    return { text: getComputedStyle(element).color, band: background };
  });
  // 纸白字压在强调橙上只有 2.92:1，墨色字是 5.49:1。
  expect(band).toBe(computed(TOKENS.color.brand.accent.$value));
  expect(text).toBe(computed(TOKENS.color.text.primary.$value));
});

import { expect, test } from "@playwright/test";

/**
 * The hero's 3D scene is absolutely positioned across the whole hero and sits
 * behind the headline and copy. On a desktop it only covers the right 55%, so
 * the text stays clear; on a phone it covered everything and the dense
 * wireframe rendered the copy unreadable.
 *
 * It must not merely be hidden with CSS either — that still creates the canvas
 * and renders it every frame, on the devices least able to afford three.js.
 */

test("a phone never creates the hero's 3D canvas", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { name: "HENUKIT" })).toBeVisible();
  // Give the client-only scene the chance it would need to mount.
  await page.waitForTimeout(1500);

  await expect(page.locator("canvas")).toHaveCount(0);
});

test("the headline and copy stay legible on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/", { waitUntil: "domcontentloaded" });

  const headline = page.getByRole("heading", { name: "HENUKIT" });
  await expect(headline).toBeVisible();

  // Nothing decorative may sit on top of the copy.
  const copy = page.getByText("henukit 是面向校园的综合性学生平台", { exact: false });
  await expect(copy).toBeVisible();

  const box = await copy.boundingBox();
  expect(box).not.toBeNull();
  const topmost = await page.evaluate(
    ({ x, y }) => {
      const el = document.elementFromPoint(x, y);
      return el?.tagName.toLowerCase() ?? "";
    },
    { x: box!.x + 8, y: box!.y + box!.height / 2 }
  );
  expect(topmost).not.toBe("canvas");

  // The hero must not scroll sideways on a phone.
  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

test("a desktop still gets the 3D scene", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { name: "HENUKIT" })).toBeVisible();
  await expect(page.locator("canvas")).toHaveCount(1, { timeout: 10000 });
});

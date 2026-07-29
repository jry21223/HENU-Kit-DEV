import { expect, test } from "@playwright/test";

test("default Portal flags keep the V2 catalog dark and fail closed", async ({ page }) => {
  let catalogRequests = 0;
  await page.route("**/api/v1/practice/catalog", async (route) => {
    catalogRequests += 1;
    await route.abort();
  });
  for (const legacyPath of ["**/api/v1/practice/schools", "**/api/v1/practice/banks"]) {
    await page.route(legacyPath, async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "legacy_practice_unavailable" }),
      });
    });
  }

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  await expect(page.getByText("接口不可用")).toBeVisible();
  await expect(page.getByTestId("quizcraft-catalog")).toHaveCount(0);
  await expect(page.getByTestId("quizcraft-catalog-start")).toHaveCount(0);
  expect(catalogRequests).toBe(0);
});

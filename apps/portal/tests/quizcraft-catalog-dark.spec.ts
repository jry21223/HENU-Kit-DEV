import { expect, test } from "@playwright/test";

test("default Portal flags keep the V2 catalog dark and fail closed", async ({ page }) => {
  let catalogRequests = 0;
  let legacyPracticeRequests = 0;
  page.on("request", (request) => {
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/v1/practice/catalog") catalogRequests += 1;
    if (
      pathname === "/api/v1/practice/schools" ||
      pathname === "/api/v1/practice/banks" ||
      pathname === "/api/v1/practice/leaderboard"
    ) {
      legacyPracticeRequests += 1;
    }
  });
  await page.route("**/api/v1/practice/catalog", async (route) => {
    await route.abort();
  });

  await page.goto("/practice", { waitUntil: "domcontentloaded" });

  // The legacy schools/banks → gateway cache → mock fallback chain was
  // removed with ADR-0036: with the browser flag off the page renders an
  // empty non-probing state instead of a fabricated catalog.
  await expect(page.getByText("暂无题库")).toBeVisible();
  await expect(page.getByTestId("quizcraft-catalog")).toHaveCount(0);
  await expect(page.getByTestId("quizcraft-catalog-start")).toHaveCount(0);
  expect(catalogRequests).toBe(0);
  expect(legacyPracticeRequests).toBe(0);
});

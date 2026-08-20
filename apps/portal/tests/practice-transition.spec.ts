import { expect, test } from "@playwright/test";

// TransitionLink plays an exit tween before pushing the route. When the current
// page has no [data-block] elements it animates the [data-transition-page]
// wrapper instead — and that wrapper lives in practice/layout, so it survives
// the navigation. Nothing used to restore it, which left the next page mounted
// inside an opacity:0/visibility:hidden shell: every practice route rendered
// blank after coming back from a block-less page.
//
// The quiz route without a bank selection renders PracticeState, which carries
// only [data-enter] and is therefore exactly that block-less case.
test("Practice keeps the shell visible after leaving a page without animated blocks", async ({ page }) => {
  await page.goto("/practice/quiz");

  const shell = page.locator("[data-transition-page]");
  const backLink = page.getByRole("link", { name: /返回题库目录/ });
  await expect(backLink).toBeVisible();
  // Guard the premise: this state must not contain animated blocks.
  await expect(shell.locator("[data-block]")).toHaveCount(0);

  await backLink.click();

  // The push must happen even though the exit tween drives the shell, and the
  // shell must be repainted rather than inheriting the faded-out styles.
  await expect(page).toHaveURL(/\/practice$/);
  await expect(shell).toBeVisible();
  await expect(shell).toHaveCSS("opacity", "1");
  await expect(shell).toHaveCSS("visibility", "visible");
});

// The exit tween is driven by requestAnimationFrame, so a backgrounded tab
// freezes it mid-flight and onComplete never runs. Navigation must not be lost
// with it. Stalling rAF reproduces that without backgrounding the tab.
test("Practice still navigates when the exit animation never completes", async ({ page }) => {
  // GSAP captures requestAnimationFrame when its module initialises, so the
  // stub has to be installed before any page script runs.
  await page.addInitScript(() => {
    window.requestAnimationFrame = (() => 0) as typeof window.requestAnimationFrame;
  });

  await page.goto("/practice/quiz");
  const backLink = page.getByRole("link", { name: /返回题库目录/ });
  await expect(backLink).toBeVisible();

  await backLink.click();

  await expect(page).toHaveURL(/\/practice$/, { timeout: 15_000 });
});

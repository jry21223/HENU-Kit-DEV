import { expect, test } from "@playwright/test";

const wrongFact = {
  bank_id: "33333333-3333-4333-8333-333333333333",
  question_id: "55555555-5555-4555-8555-555555555555",
  question_version_id: "66666666-6666-4666-8666-666666666666",
  wrong: true,
  attempt_count: 3,
  correct_count: 1,
  updated_at: "2026-08-06T08:00:00Z",
};

function learningStateEnvelope(
  items: Array<typeof wrongFact>,
  page = 1,
  total = items.length,
) {
  return {
    request_id: "req_learning_state_state",
    data: {
      items,
      pagination: {
        page,
        page_size: 20,
        total,
        total_pages: total === 0 ? 0 : Math.ceil(total / 20),
      },
    },
  };
}

test("mistakes page renders synchronized Core wrong-question facts at 390px", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(/\/api\/v1\/learning-state(?:\?.*)?$/, async (route) => {
    expect(new URL(route.request().url()).searchParams.get("wrong")).toBe("true");
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(learningStateEnvelope([wrongFact])),
    });
  });

  await page.goto("/practice/mistakes", { waitUntil: "domcontentloaded" });
  const facts = page.getByTestId("learning-state-success");
  await expect(facts).toBeVisible();
  await expect(facts).toContainText(wrongFact.bank_id);
  await expect(facts).toContainText(wrongFact.question_id);
  await expect(facts).toContainText("作答 3 次");
  await expect(facts).toContainText("答对 1 次");
  await expect(page.locator("main")).not.toContainText("题干");
  await expect(page.locator("main")).not.toContainText("答案");
  expect(await page.locator("html").evaluate((element) => element.scrollWidth <= element.clientWidth)).toBeTruthy();
});

test("mistakes page reaches its second page and recovers when that page disappears", async ({ page }) => {
  const facts = Array.from({ length: 21 }, (_, index) => ({
    ...wrongFact,
    question_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
    question_version_id: `10000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  }));
  const requestedPages: string[] = [];
  let shrunk = false;
  await page.route(/\/api\/v1\/learning-state(?:\?.*)?$/, async (route) => {
    const requestURL = new URL(route.request().url());
    expect(requestURL.searchParams.get("wrong")).toBe("true");
    const requestedPage = requestURL.searchParams.get("page") ?? "1";
    requestedPages.push(requestedPage);
    const pageNumber = Number(requestedPage);
    const items = pageNumber === 1 ? facts.slice(0, 20) : shrunk ? [] : facts.slice(20);
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(learningStateEnvelope(items, pageNumber, shrunk ? 20 : facts.length)),
    });
  });

  await page.goto("/practice/mistakes", { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("learning-state-success")).toContainText(facts[0].question_id);
  await expect(page.getByTestId("learning-state-success")).not.toContainText(facts[20].question_id);
  await page.getByRole("button", { name: "下一页" }).click();
  await expect(page.getByTestId("learning-state-success")).toContainText(facts[20].question_id);
  expect(requestedPages).toEqual(["1", "2"]);

  shrunk = true;
  await page.getByRole("button", { name: "刷新同步" }).click();
  await expect(page.getByTestId("learning-state-success")).toContainText(facts[0].question_id);
  await expect(page.getByTestId("learning-state-empty")).toHaveCount(0);
  expect(requestedPages.slice(-2)).toEqual(["2", "1"]);
});

test("mistakes page keeps loading, empty, failure, retry, and refresh explicit", async ({ page }) => {
  let phase: "loading" | "empty" | "failure" | "success" | "refreshing" | "refreshed" = "loading";
  const pendingReleases = new Set<() => void>();
  await page.route(/\/api\/v1\/learning-state(?:\?.*)?$/, async (route) => {
    if (phase === "loading" || phase === "refreshing") {
      await new Promise<void>((resolve) => {
        const release = () => {
          pendingReleases.delete(release);
          resolve();
        };
        pendingReleases.add(release);
      });
    }
    if (phase === "failure") {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "learning state unavailable", request_id: "req_down" }),
      });
      return;
    }
    const item = phase === "refreshed" ? { ...wrongFact, attempt_count: 4 } : wrongFact;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(learningStateEnvelope(phase === "empty" ? [] : [item])),
    });
  });

  await page.goto("/practice/mistakes", { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("learning-state-loading")).toBeVisible();
  phase = "empty";
  for (const release of [...pendingReleases]) release();
  await expect(page.getByTestId("learning-state-empty")).toBeVisible();
  await expect(page.getByTestId("learning-state-empty")).toContainText("还没有同步的错题记录");

  phase = "failure";
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("learning-state-error")).toBeVisible();
  await expect(page.getByTestId("learning-state-success")).toHaveCount(0);

  phase = "success";
  await page.getByRole("button", { name: "重试" }).click();
  await expect(page.getByTestId("learning-state-success")).toContainText("作答 3 次");

  phase = "refreshing";
  await page.getByRole("button", { name: "刷新同步" }).click();
  await expect(page.getByTestId("learning-state-loading")).toBeVisible();
  phase = "refreshed";
  for (const release of [...pendingReleases]) release();
  await expect(page.getByTestId("learning-state-success")).toContainText("作答 4 次");
});

test("mistakes page gives signed-out users an explicit sign-in action", async ({ page }) => {
  await page.route(/\/api\/v1\/learning-state(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "not authenticated", request_id: "req_signed_out" }),
    });
  });

  await page.goto("/practice/mistakes", { waitUntil: "domcontentloaded" });
  const signedOut = page.getByTestId("learning-state-signed-out");
  await expect(signedOut).toBeVisible();
  await expect(signedOut).toContainText("登录后查看跨设备同步的错题");
  await expect(page.getByRole("button", { name: "登录查看" })).toBeVisible();
  await expect(page.getByTestId("learning-state-success")).toHaveCount(0);
});

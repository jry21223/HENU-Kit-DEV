import { expect, test } from "@playwright/test";

test("Console browser returns to the authenticated Food operations path", async ({
  page,
}) => {
  await page.goto("/food");
  const login = page.getByRole("link", { name: "登录 Console" });
  await expect(login).toHaveAttribute(
    "href",
    "/api/v1/auth/login?return_to=%2Ffood"
  );
  await login.click();

  await expect(page).toHaveURL(
    /http:\/\/127\.0\.0\.1:3112\/account\/login\?continuation=[A-Za-z0-9_-]+$/
  );
  const accountCenterURL = new URL(page.url());
  expect([...accountCenterURL.searchParams.keys()]).toEqual(["continuation"]);
  await expect(
    page.getByText("登录后继续前往 HENUKit Console")
  ).toBeVisible();

  await page.getByRole("button", { name: "密码登录" }).click();
  await page.getByLabel("学校邮箱").fill("operator");
  await page
    .getByLabel("密码 / PASSWORD")
    .fill("correct horse battery staple");
  await page.getByLabel("密码 / PASSWORD").press("Enter");

  await expect(page).toHaveURL("http://127.0.0.1:4175/food");
  await expect(page.getByText("权限已验证")).toBeVisible();
  const session = await page.evaluate(async () => {
    const response = await fetch("/api/v1/session", {
      credentials: "include",
      cache: "no-store",
    });
    return { status: response.status, body: await response.json() };
  });
  expect(session.status).toBe(200);
  expect(session.body).toMatchObject({
    data: { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" } },
  });
});

test("Console destination remains usable at 360px with keyboard navigation", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/food");
  await page.getByRole("link", { name: "登录 Console" }).click();

  await expect(
    page.getByText("登录后继续前往 HENUKit Console")
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true);
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toBeVisible();
});

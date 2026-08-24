import { expect, test } from "@playwright/test";

test("Portal browser completes Account Center continuation through the real Gateway callback", async ({
  page,
}) => {
  await page.addInitScript(() => window.localStorage.clear());

  await page.goto(
    "http://127.0.0.1:3111/api/v1/auth/login?return_to=%2Faccount"
  );
  await expect(page).toHaveURL(
    /http:\/\/127\.0\.0\.1:3111\/account\/login\?continuation=[A-Za-z0-9_-]+$/
  );
  const accountCenterURL = new URL(page.url());
  expect([...accountCenterURL.searchParams.keys()]).toEqual(["continuation"]);
  await expect(page.getByText("登录后继续前往 HENU Kit")).toBeVisible();

  await page.getByRole("button", { name: "密码登录" }).click();
  await page.getByLabel("学校邮箱").fill("student");
  await page.getByLabel("密码 / PASSWORD").fill("correct horse battery staple");
  await page.getByRole("button", { name: "登 录" }).click();

  await expect(page).toHaveURL("http://127.0.0.1:3111/account");
  const session = await page.evaluate(async () => {
    const response = await fetch("/api/v1/session", {
      credentials: "include",
      cache: "no-store",
    });
    return { status: response.status, body: await response.json() };
  });
  expect(session.status).toBe(200);
  expect(session.body).toMatchObject({
    user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
    display_name: "小河同学",
  });
});

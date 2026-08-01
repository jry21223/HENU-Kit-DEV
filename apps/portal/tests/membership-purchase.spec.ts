import { expect, test, type BrowserContext } from "@playwright/test";

const sessionUserID = "11111111-1111-4111-8111-111111111111";
const orderID = "9f1c2a44-6f3d-4c2b-9a71-2b6d5e8c4a10";
const merchantOrderID = "HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234";
const checkoutURL = "weixin://wxpay/bizpayurl?pr=A1b2C3d";

type PurchaseState = {
  paid: boolean;
  checkoutURL?: string;
  providerEnabled: boolean;
  dependencyFailure?: boolean;
};

function order(state: PurchaseState) {
  return {
    id: orderID,
    plan: "lifetime",
    amount_cents: 990,
    status: state.paid ? "paid" : "pending_payment",
    version: 2,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
  };
}

async function installGateway(context: BrowserContext, state: PurchaseState) {
  await context.route("**/api/v1/session", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: sessionUserID,
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    })
  );

  await context.route("**/api/v1/account/membership", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: { plan: state.paid ? "lifetime" : "free", lifetime: state.paid },
        request_id: "req_membership",
      }),
    })
  );

  await context.route("**/api/v1/account/membership-orders", async (route) => {
    if (route.request().method() === "POST") {
      if (!state.providerEnabled) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({
            error: state.dependencyFailure
              ? "account_portfolio_unavailable"
              : "membership_payment_unavailable",
            request_id: "req_order",
          }),
        });
        return;
      }
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            order: order(state),
            ...(state.checkoutURL ? { checkout_url: state.checkoutURL } : {}),
          },
          request_id: "req_order",
        }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { orders: [order(state)] }, request_id: "req_orders" }),
    });
  });
}

test("a user scans the payment QR without ever seeing the merchant order number", async ({
  context,
  page,
}) => {
  const state: PurchaseState = { paid: false, checkoutURL, providerEnabled: true };
  await installGateway(context, state);

  await page.goto("/account/membership");
  await page.getByRole("button", { name: "购买终身会员" }).click();

  const qr = page.locator('[data-membership-checkout-qr="ready"] img');
  await expect(qr).toBeVisible();

  // The rendered code is a locally encoded image, and nothing on the page may
  // carry the private merchant order number.
  await expect(qr).toHaveAttribute("src", /^data:image\//);
  await expect(page.locator("body")).not.toContainText(merchantOrderID);
  expect(page.url()).not.toContain(merchantOrderID);
});

test("payment is reported only after the server confirms it", async ({ context, page }) => {
  const state: PurchaseState = { paid: false, checkoutURL, providerEnabled: true };
  await installGateway(context, state);

  await page.goto("/account/membership");
  await page.getByRole("button", { name: "购买终身会员" }).click();
  await expect(page.locator('[data-membership-purchase="awaiting"]')).toBeVisible();

  // Showing a QR must never be read as a completed payment.
  await expect(page.locator('[data-membership-purchase="paid"]')).toHaveCount(0);

  // The durable order flips only when the server says so. Once it does, the
  // entitlement the user actually cares about is what updates: the purchase
  // panel gives way to the confirmed lifetime membership.
  state.paid = true;
  await expect(page.getByRole("heading", { name: "终身会员" })).toBeVisible({ timeout: 20000 });
  await expect(page.locator('[data-membership-purchase="awaiting"]')).toHaveCount(0);
});

test("a disabled payment provider is an honest unavailable state", async ({ context, page }) => {
  const state: PurchaseState = { paid: false, providerEnabled: false };
  await installGateway(context, state);

  await page.goto("/account/membership");
  await page.getByRole("button", { name: "购买终身会员" }).click();

  await expect(page.locator('[data-membership-purchase="unavailable"]')).toBeVisible();
  await expect(page.locator('[data-membership-purchase="paid"]')).toHaveCount(0);
});

test("a dependency failure does not claim that no order was created", async ({ context, page }) => {
  const state: PurchaseState = {
    paid: false,
    providerEnabled: false,
    dependencyFailure: true,
  };
  await installGateway(context, state);

  await page.goto("/account/membership");
  await page.getByRole("button", { name: "购买终身会员" }).click();

  await expect(page.locator('[data-membership-purchase="error"]')).toBeVisible();
  await expect(page.locator('[data-membership-purchase="unavailable"]')).toHaveCount(0);
});

test("an order with no checkout handle tells the user to start again", async ({
  context,
  page,
}) => {
  const state: PurchaseState = { paid: false, providerEnabled: true };
  await installGateway(context, state);

  await page.goto("/account/membership");
  await page.getByRole("button", { name: "购买终身会员" }).click();

  await expect(page.locator('[data-membership-purchase="awaiting"]')).toContainText("重新发起支付");
  await expect(page.locator('[data-membership-checkout-qr="ready"]')).toHaveCount(0);
});

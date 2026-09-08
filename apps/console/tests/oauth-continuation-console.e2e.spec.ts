import { expect, test } from "@playwright/test";
import { defineOAuthContinuationJourney } from "@henukit/oauth-continuation-e2e";

defineOAuthContinuationJourney({
  name: "Console",
  platformOrigin: "http://127.0.0.1:3231",
  accountCenterOrigin: "http://127.0.0.1:3112",
  productOrigin: "http://127.0.0.1:4175",
  clientID: "console-gateway",
  redirectURI: "http://127.0.0.1:4175/api/v1/auth/callback",
  productName: "HENUKit Console",
  emailLocalPart: "operator",
  password: "correct horse battery staple",
  finalURL: "http://127.0.0.1:4175/food",
  expectedSession: {
    data: { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" } },
  },
  // App.vue owns this complete Console route inventory.
  publicRoutes: [
    { path: "/", readySelector: "#overview-heading" },
    { path: "/operations", readySelector: "#operations-heading" },
    { path: "/notices", readySelector: "#notice-heading" },
    { path: "/library", readySelector: "#library-heading" },
    { path: "/food", readySelector: "#food-heading" },
    { path: "/account", readySelector: "#account-tickets-heading" },
    {
      path: "/account/memberships",
      readySelector: "#account-membership-heading",
    },
    { path: "/account/points", readySelector: "#account-points-heading" },
  ],
  start: async (page) => {
    await page.goto("/food");
    const login = page.getByRole("link", { name: "登录 Console" });
    await expect(login).toHaveAttribute(
      "href",
      "/api/v1/auth/login?return_to=%2Ffood",
    );
    await login.click();
  },
  restart: async (page) => {
    await page.goto("/api/v1/auth/login?return_to=%2Ffood");
  },
}, { expect, test });

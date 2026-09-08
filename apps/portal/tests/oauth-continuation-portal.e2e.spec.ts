import { expect, test } from "@playwright/test";
import { defineOAuthContinuationJourney } from "@henukit/oauth-continuation-e2e";

defineOAuthContinuationJourney({
  name: "Portal",
  platformOrigin: "http://127.0.0.1:3211",
  accountCenterOrigin: "http://127.0.0.1:3111",
  productOrigin: "http://127.0.0.1:3111",
  clientID: "portal-gateway",
  redirectURI: "http://127.0.0.1:3111/api/v1/auth/callback",
  productName: "HENU Kit",
  emailLocalPart: "student",
  password: "correct horse battery staple",
  finalURL: "http://127.0.0.1:3111/account",
  expectedSession: {
    user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
    display_name: "小河同学",
  },
  // Canonical representative inventory for every Portal page route, including
  // one concrete path for each dynamic family.
  publicRoutes: [
    { path: "/", readySelector: "main" },
    { path: "/account", readySelector: "[data-account-summary-state]" },
    {
      path: "/account/login",
      readySelector: "main",
      accountCenter: true,
      isolatedSignedOut: true,
    },
    {
      path: "/account/membership",
      readySelector: "[data-account-membership-state]",
    },
    {
      path: "/account/notifications",
      readySelector: "[data-account-notifications-state]",
    },
    { path: "/account/posts", readySelector: "[data-account-food-posts-state]" },
    {
      path: "/account/profile",
      readySelector: "[data-account-career-profile-state]",
    },
    { path: "/account/recover", readySelector: "main" },
    { path: "/account/security", readySelector: "h1" },
    { path: "/account/tickets", readySelector: "[data-account-tickets-state]" },
    { path: "/account/wallet", readySelector: "[data-account-points-state]" },
    { path: "/campus", readySelector: "main" },
    { path: "/campus/deals", readySelector: "main" },
    { path: "/campus/item/h-01", readySelector: "main" },
    { path: "/campus/publish", readySelector: "main" },
    { path: "/career", readySelector: "main" },
    { path: "/career/history", readySelector: "main" },
    { path: "/food", readySelector: "main" },
    { path: "/food/post/ml-01", readySelector: "main" },
    { path: "/food/publish", readySelector: "main" },
    { path: "/library", readySelector: "main" },
    { path: "/library/item/free-limit-note", readySelector: "main" },
    {
      path: "/library/read/free-limit-note",
      expectedPath: "/library/item/free-limit-note",
      readySelector: "main",
    },
    { path: "/library/shelf", readySelector: "main" },
    {
      path: "/library/slides/free-limit-note",
      expectedPath: "/library/item/free-limit-note",
      readySelector: "main",
    },
    { path: "/practice", readySelector: "main" },
    { path: "/practice/favorites", readySelector: "main" },
    { path: "/practice/favorites/sixiu", readySelector: "main" },
    { path: "/practice/leaderboard", readySelector: "main" },
    { path: "/practice/lists/ds-final", readySelector: "main" },
    { path: "/practice/quiz", readySelector: "main" },
    { path: "/practice/stats", readySelector: "main" },
  ],
  start: async (page) => {
    await page.goto("/api/v1/auth/login?return_to=%2Faccount");
  },
  restart: async (page) => {
    await page.goto("/api/v1/auth/login?return_to=%2Faccount");
  },
}, { expect, test });

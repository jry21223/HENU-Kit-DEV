import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "oauth-continuation-portal.e2e.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:3111",
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "node ../../scripts/oauth-continuation-platform-fixture.mjs",
      url: "http://127.0.0.1:3211/healthz",
      env: {
        ...process.env,
        OAUTH_FIXTURE_ADDRESS: "127.0.0.1:3211",
        OAUTH_FIXTURE_PORTAL_ORIGIN: "http://127.0.0.1:3111",
        OAUTH_FIXTURE_CLIENT_ID: "portal-gateway",
        OAUTH_FIXTURE_CLIENT_SECRET:
          "portal-e2e-client-secret-with-enough-entropy",
        OAUTH_FIXTURE_KEY_ID: "primary",
        OAUTH_FIXTURE_REDIRECT_URI:
          "http://127.0.0.1:3111/api/v1/auth/callback",
        OAUTH_FIXTURE_PRODUCT_NAME: "HENU Kit",
        OAUTH_FIXTURE_COOKIE_NAMESPACE: "e2e",
        OAUTH_FIXTURE_TEST_EMAIL: "student@henu.edu.cn",
        OAUTH_FIXTURE_TEST_PASSWORD: "correct horse battery staple",
        OAUTH_FIXTURE_EXCHANGE_TOKEN:
          "portal_e2e_exchange_token_with_32_characters",
        OAUTH_FIXTURE_USER_ID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
        OAUTH_FIXTURE_DISPLAY_NAME: "小河同学",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command:
        "cd ../../services/portal-gateway && go run ./test/oauth-continuation-fixture",
      url: "http://127.0.0.1:3210/api/v1/healthz",
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "pnpm --filter @henukit/portal exec next dev -p 3111",
      url: "http://127.0.0.1:3111",
      env: {
        ...process.env,
        NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
        NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
        PLAYWRIGHT_ACCOUNT_AUTH_URL: "http://127.0.0.1:3211",
        PLAYWRIGHT_PORTAL_GATEWAY_URL: "http://127.0.0.1:3210",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});

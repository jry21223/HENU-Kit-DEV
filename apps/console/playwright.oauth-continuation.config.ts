import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "oauth-continuation-console.e2e.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4175",
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "node ../../scripts/oauth-continuation-platform-fixture.mjs",
      url: "http://127.0.0.1:3231/healthz",
      env: {
        OAUTH_FIXTURE_ADDRESS: "127.0.0.1:3231",
        OAUTH_FIXTURE_PORTAL_ORIGIN: "http://127.0.0.1:3112",
        OAUTH_FIXTURE_CLIENT_ID: "console-gateway",
        OAUTH_FIXTURE_CLIENT_SECRET:
          "console-e2e-client-secret-with-enough-entropy",
        OAUTH_FIXTURE_KEY_ID: "primary",
        OAUTH_FIXTURE_REDIRECT_URI:
          "http://127.0.0.1:4175/api/v1/auth/callback",
        OAUTH_FIXTURE_PRODUCT_NAME: "HENUKit Console",
        OAUTH_FIXTURE_COOKIE_NAMESPACE: "console_e2e",
        OAUTH_FIXTURE_TEST_EMAIL: "operator@henu.edu.cn",
        OAUTH_FIXTURE_TEST_PASSWORD: "correct horse battery staple",
        OAUTH_FIXTURE_EXCHANGE_TOKEN:
          "console_e2e_exchange_token_with_32_characters",
        OAUTH_FIXTURE_USER_ID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
        OAUTH_FIXTURE_AUTHORIZATION_MODE: "allow",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command:
        "cd ../../services/console-gateway && go run ./test/oauth-continuation-fixture",
      url: "http://127.0.0.1:3230/api/v1/healthz",
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "pnpm --filter @henukit/portal exec next dev -p 3112",
      url: "http://127.0.0.1:3112",
      env: {
        NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
        NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
        PLAYWRIGHT_ACCOUNT_AUTH_URL: "http://127.0.0.1:3231",
        PLAYWRIGHT_CONSOLE_ORIGIN: "http://127.0.0.1:4175",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command:
        "pnpm --filter @henukit/console exec vite --host 127.0.0.1 --port 4175",
      url: "http://127.0.0.1:4175",
      env: {
        PLAYWRIGHT_CONSOLE_GATEWAY_URL: "http://127.0.0.1:3230",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});

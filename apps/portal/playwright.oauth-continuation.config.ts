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

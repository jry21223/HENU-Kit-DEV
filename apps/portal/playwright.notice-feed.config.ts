import { defineConfig, devices } from "@playwright/test";

// The notice-feed spec needs only the Portal process. Keeping it separate
// avoids making a local Portal verification depend on the Console checkout.
export default defineConfig({
  testDir: "./tests",
  testMatch: "notice-feed.spec.ts",
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://127.0.0.1:3004",
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: {
    command: "./node_modules/.bin/next dev -p 3004",
    url: "http://127.0.0.1:3004",
    env: {
      ...process.env,
      NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
      NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
    },
    reuseExistingServer: false,
    timeout: 120_000,
  },
});

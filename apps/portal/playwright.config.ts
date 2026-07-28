import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://127.0.0.1:3001",
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "pnpm --filter @henukit/portal dev",
      url: "http://127.0.0.1:3001",
      env: {
        ...process.env,
        NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
        NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
      },
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
    {
      command: "pnpm --filter @henukit/console exec vite --host 127.0.0.1 --port 4174",
      url: "http://127.0.0.1:4174",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
  ],
});

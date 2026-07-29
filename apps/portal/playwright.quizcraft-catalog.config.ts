import { defineConfig, devices } from "@playwright/test";

// A separate dev server makes the cutover-only public UI flag explicit and
// prevents a locally reused default server from accidentally hiding this test.
export default defineConfig({
  testDir: "./tests",
  testMatch: "quizcraft-catalog.spec.ts",
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://127.0.0.1:3002",
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: {
    command: "pnpm --filter @henukit/portal exec next dev -p 3002",
    url: "http://127.0.0.1:3002",
    env: {
      ...process.env,
      NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
      NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
      NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG: "1",
    },
    reuseExistingServer: false,
    timeout: 120_000,
  },
});

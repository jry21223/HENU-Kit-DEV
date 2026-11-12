import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.PLAYWRIGHT_TRAINING_PORT || 3164);
export default defineConfig({
  testDir: "./tests",
  testMatch: process.env.PLAYWRIGHT_TRAINING_STACK === "1" ? "training-stack.spec.ts" : "training-listening.spec.ts",
  timeout: 45_000,
  expect: { timeout: 10_000 },
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    ...devices["Desktop Chrome"],
    channel: process.env.PLAYWRIGHT_CHROME_CHANNEL || undefined,
    trace: "retain-on-failure",
  },
  webServer: {
    command: `pnpm --config.verify-deps-before-run=false exec next dev --webpack -p ${port}`,
    url: `http://127.0.0.1:${port}`,
    env: { ...process.env, NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1", NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0", NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG: "1", NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS: "1" },
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});

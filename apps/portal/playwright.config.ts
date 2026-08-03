import { defineConfig, devices } from "@playwright/test";

const quizCraftV2Reads =
  process.env.PLAYWRIGHT_ENABLE_QUIZCRAFT_V2_READS === "1" ? "1" : "0";
const port = quizCraftV2Reads === "1" ? 3101 : 3001;

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
    baseURL: `http://localhost:${port}`,
    ...devices["Desktop Chrome"],
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: `pnpm --filter @henukit/portal exec next dev -p ${port}`,
      url: `http://127.0.0.1:${port}`,
      env: {
        ...process.env,
        NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1",
        NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0",
        NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS: quizCraftV2Reads,
        NEXT_PUBLIC_FOOD_DESK_URL:
          "https://henu-campus-guide.cocoa-brush-7952.chatgpt.site/#food-submit",
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

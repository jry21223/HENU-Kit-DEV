import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests", testMatch: "qq-binding.spec.ts", workers: 1,
  // 与另外两个配置一样，给冷启动的 dev server 编译留出时间。
  timeout: 45_000, expect: { timeout: 10_000 },
  forbidOnly: !!process.env.CI, retries: process.env.CI ? 2 : 0, reporter: process.env.CI ? "github" : "list",
  use: { ...devices["Desktop Chrome"], baseURL: "http://127.0.0.1:3197", trace: "retain-on-failure" },
  webServer: {
    command: "pnpm exec next dev -p 3197 --hostname 127.0.0.1",
    url: "http://127.0.0.1:3197", reuseExistingServer: false, timeout: 120_000,
    env: { NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY: "1", NEXT_PUBLIC_PORTAL_ALLOW_MOCK: "0", NEXT_PUBLIC_LANGBOT_WIDGET_URL: "https://widget.example.test/widget.js" },
  },
});

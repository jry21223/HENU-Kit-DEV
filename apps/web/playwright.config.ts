import { defineConfig } from "@playwright/test";

const testOrigin = "http://127.0.0.1:3102";

process.env.HOME_URL ??= `${testOrigin}/`;
process.env.WORKSPACE_URL ??= `${testOrigin}/workspace`;
process.env.E2E_WEB_BASE_URL ??= testOrigin;

export default defineConfig({
  testDir: "./tests",
  webServer: {
    command: "pnpm exec next dev -p 3102",
    url: testOrigin,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});

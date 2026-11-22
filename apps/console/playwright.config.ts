import { defineConfig } from "@playwright/test";

const env = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env;
const channel = env?.PLAYWRIGHT_CHANNEL || (env?.CI ? "chrome" : undefined);

export default defineConfig({
  testDir: "./tests",
  testIgnore: "oauth-continuation-console.e2e.spec.ts",
  use: {
    baseURL: "http://127.0.0.1:4174",
    ...(channel ? { channel } : {}),
  },
  webServer: {
    command: "npm run build && npm run preview",
    url: "http://127.0.0.1:4174",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});

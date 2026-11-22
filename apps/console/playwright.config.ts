import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testIgnore: "oauth-continuation-console.e2e.spec.ts",
  use: { baseURL: "http://127.0.0.1:4174" },
  webServer: {
    command: "npx vite preview --host 127.0.0.1 --port 4174",
    url: "http://127.0.0.1:4174",
    reuseExistingServer: false,
    timeout: 60_000,
  },
});

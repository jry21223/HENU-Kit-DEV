// Local-only override: run e2e against a preview server on another host
// (e.g. the WSL box that did the build) instead of starting a local one.
// Not committed. Usage:
//   pnpm -C apps/console test:e2e --config playwright.remote.config.ts
//   E2E_BASE_URL=http://192.168.1.194:4174 pnpm -C apps/console test:e2e --config playwright.remote.config.ts
import { defineConfig } from "@playwright/test";
import baseConfig from "./playwright.config";

const { webServer: _omit, ...base } = baseConfig;

export default defineConfig({
  ...base,
  webServer: undefined,
  use: {
    ...base.use,
    baseURL: process.env.E2E_BASE_URL ?? "http://192.168.1.194:4174",
  },
});

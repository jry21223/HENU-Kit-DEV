import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  test: {
    environment: "node",
    // Globbed rather than enumerated in package.json so a new unit test can never
    // be silently excluded from CI. Scoped to src/ .test.ts because tests/ holds
    // Playwright specs (run by test:e2e:*) and src/ also holds node:test .mjs
    // files (run by test:unit) — neither can execute under Vitest.
    include: ["src/**/*.test.ts"],
    coverage: {
      // Scoped to the pure-logic surface this suite can actually reach: the
      // Vitest environment is "node", so components and pages cannot render
      // here. Without an explicit include, untested files are omitted from the
      // report entirely and the percentage flatters itself.
      include: ["src/lib/**/*.ts"],
      exclude: ["**/*.test.ts", "**/mock.ts", "**/*.generated.ts"],
      reporter: ["text", "json-summary", "html"],
    },
  },
});

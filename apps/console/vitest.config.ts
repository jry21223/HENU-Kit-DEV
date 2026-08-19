import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  test: {
    environment: "jsdom",
    include: ["src/**/*.spec.ts"],
    coverage: {
      // The environment is jsdom, so components are reachable as well as the
      // lib modules. An explicit include is required or files no test imports
      // are left out of the report entirely.
      include: ["src/**/*.ts", "src/**/*.vue"],
      exclude: [
        "**/*.spec.ts",
        "src/main.ts",
        "src/vite-env.d.ts",
        // Generated from console-gateway.yaml. Its correctness is guaranteed by
        // codegen plus the regenerate-and-diff gate in console-gateway.yml, not
        // by hand-written tests, and 1,300 lines of generated type guards would
        // otherwise dominate the percentage.
        "src/lib/console-gateway.ts",
      ],
      reporter: ["text", "json-summary", "html"],
    },
  },
});
